package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// LimitWindow is one allowance window of an account, as its provider reports
// it: how much of the window is used and when it starts over.
type LimitWindow struct {
	// ID names the window in a stable, provider-neutral way ("five_hour",
	// "week"), so the app can lay out the same window the same way every time.
	ID string `json:"id"`
	// Label is how the window is named to the operator ("5 hours", "Week").
	Label string `json:"label"`
	// UsedPercent is how much of the window is used, 0–100.
	UsedPercent float64 `json:"used_percent"`
	// ResetsAt is when the window starts over, when the provider says.
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// LimitReader is implemented by adapters whose harness can say how much of the
// account's allowance is left. Reading it must cost no model usage: it is asked
// on a timer, whenever the app opens the dashboard, and after every run.
//
// It is a separate interface rather than part of [Adapter] because not every
// harness has an allowance to report: opencode runs whatever backend it is
// pointed at, a local model included.
type LimitReader interface {
	ReadLimits(ctx context.Context, inst Instance) ([]LimitWindow, error)
}

// LimitsState says what the latest attempt to read an instance's limits found.
type LimitsState string

const (
	// LimitsOK means the windows were read just now.
	LimitsOK LimitsState = "ok"
	// LimitsUnavailable means the read failed; the windows, if any, are the
	// last ones that were read and Reason says why they are not newer.
	LimitsUnavailable LimitsState = "unavailable"
	// LimitsUnsupported means the harness has no allowance it can report.
	LimitsUnsupported LimitsState = "unsupported"
	// LimitsDisabled means the instance is switched off and is not asked.
	LimitsDisabled LimitsState = "disabled"
)

// Limits is what claudeq knows about one instance's allowance.
type Limits struct {
	State LimitsState `json:"state"`
	// Windows are the allowance windows from the last successful read. A failed
	// read keeps them: a figure that is twenty minutes old, labelled as such,
	// is more use than none.
	Windows []LimitWindow `json:"windows,omitempty"`
	// Reason says, in a sentence, why the latest read did not succeed.
	Reason string `json:"reason,omitempty"`
	// UpdatedAt is when Windows were read. Zero when they never were.
	UpdatedAt time.Time `json:"updated_at,omitzero"`
	// CheckedAt is when the latest attempt was made, successful or not.
	CheckedAt time.Time `json:"checked_at"`
}

// DefaultLimitsInterval is how often the daemon reads every instance's limits
// on its own. The windows move with use, and claudeq's own runs trigger a read
// when they end, so a quarter of an hour is enough for the rest: what other
// clients on the same account consumed.
const DefaultLimitsInterval = 15 * time.Minute

// DefaultLimitsMinAge is how recent a reading has to be for a "fresh" request
// to reuse it. Switching views and pressing Cmd+R in quick succession must not
// turn into a burst of calls to an endpoint that limits its callers too.
const DefaultLimitsMinAge = 30 * time.Second

// LimitsTimeout bounds one read. Nothing waits on it but the dashboard.
const LimitsTimeout = 20 * time.Second

// LimitMonitor reads and remembers each instance's allowance. It is safe for
// concurrent use; the daemon shares one between its timer, the engine (which
// asks for a read after every run) and the API.
type LimitMonitor struct {
	// Registry resolves an instance's kind to its adapter.
	Registry *Registry
	// List returns the configured instances, for the reads nobody asked for
	// with an instance in hand: the timer, and a run that just ended.
	List func() ([]Instance, error)
	// Now reads the clock. Nil means time.Now.
	Now func() time.Time
	// MinAge overrides [DefaultLimitsMinAge].
	MinAge time.Duration

	mu      sync.Mutex
	entries map[string]limitsEntry
	// reading serialises reads per instance, so the timer, a finished run and
	// the dashboard asking at the same moment cost one call, not three.
	reading map[string]*sync.Mutex
}

type limitsEntry struct {
	fingerprint string
	limits      Limits
}

// NewLimitMonitor returns a monitor for the adapters in reg.
func NewLimitMonitor(reg *Registry, list func() ([]Instance, error)) *LimitMonitor {
	return &LimitMonitor{Registry: reg, List: list}
}

func (m *LimitMonitor) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *LimitMonitor) minAge() time.Duration {
	if m.MinAge > 0 {
		return m.MinAge
	}
	return DefaultLimitsMinAge
}

// Get returns the limits of every instance in insts, positionally. Without
// fresh it answers from memory and only reads an instance it has never read;
// with fresh it reads every instance whose last attempt is older than MinAge.
// The reads run at the same time, so the slowest provider sets the wait.
func (m *LimitMonitor) Get(ctx context.Context, insts []Instance, fresh bool) []Limits {
	out := make([]Limits, len(insts))
	var wg sync.WaitGroup
	for i, inst := range insts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = m.get(ctx, inst, fresh, false)
		}()
	}
	wg.Wait()
	return out
}

// RefreshID reads the limits of the instance with this id now, regardless of
// how recent the last reading is. The engine calls it when a run on that
// instance ends, which is exactly when the figures have moved.
func (m *LimitMonitor) RefreshID(ctx context.Context, id string) error {
	insts, err := m.List()
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}
	for _, inst := range insts {
		if inst.ID == id {
			m.get(ctx, inst, true, true)
			return nil
		}
	}
	return nil
}

// RefreshAll reads every configured instance whose reading is older than
// MinAge.
func (m *LimitMonitor) RefreshAll(ctx context.Context) error {
	insts, err := m.List()
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}
	m.Get(ctx, insts, true)
	return nil
}

// Run reads every instance at once and then every interval until ctx ends.
// Errors listing the providers are reported to logf and retried on the next
// tick: the timer is the one reader nobody watches.
func (m *LimitMonitor) Run(ctx context.Context, interval time.Duration, logf func(string, ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	m.loop(ctx, t.C, logf)
}

func (m *LimitMonitor) loop(ctx context.Context, tick <-chan time.Time, logf func(string, ...any)) {
	for {
		if err := m.RefreshAll(ctx); err != nil && ctx.Err() == nil {
			logf("provider limits: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

func (m *LimitMonitor) get(ctx context.Context, inst Instance, fresh, force bool) Limits {
	lock := m.readLock(inst.ID)
	lock.Lock()
	defer lock.Unlock()

	// Checked under the instance's lock: a read that finished while this one
	// waited is as fresh as the one it would make.
	prev, ok := m.cached(inst)
	switch {
	case ok && !fresh:
		return prev
	case ok && !force && m.now().Sub(prev.CheckedAt) < m.minAge():
		return prev
	}
	l := m.read(ctx, inst, prev)
	if ctx.Err() != nil && l.State == LimitsUnavailable {
		// A read cut short by the caller going away says nothing about the
		// account, and remembering it would show a passing blip as a problem
		// until the next tick.
		return l
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]limitsEntry{}
	}
	m.entries[inst.ID] = limitsEntry{fingerprint: fingerprint(inst), limits: l}
	return l
}

func (m *LimitMonitor) readLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reading == nil {
		m.reading = map[string]*sync.Mutex{}
	}
	l, ok := m.reading[id]
	if !ok {
		l = &sync.Mutex{}
		m.reading[id] = l
	}
	return l
}

// cached returns the remembered limits of inst, provided they belong to its
// present configuration: a new configuration directory is another account.
func (m *LimitMonitor) cached(inst Instance) (Limits, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[inst.ID]
	if !ok || e.fingerprint != fingerprint(inst) {
		return Limits{}, false
	}
	return e.limits, true
}

func (m *LimitMonitor) read(ctx context.Context, inst Instance, prev Limits) Limits {
	now := m.now()
	if !inst.Enabled {
		return Limits{State: LimitsDisabled, CheckedAt: now}
	}
	ad, err := m.Registry.Lookup(inst.Kind)
	if err != nil {
		return Limits{State: LimitsUnsupported, Reason: err.Error(), CheckedAt: now}
	}
	r, ok := ad.(LimitReader)
	if !ok {
		return Limits{
			State:     LimitsUnsupported,
			Reason:    ad.Describe().Name + " does not report an allowance.",
			CheckedAt: now,
		}
	}
	ctx, cancel := context.WithTimeout(ctx, LimitsTimeout)
	defer cancel()
	windows, err := r.ReadLimits(ctx, inst)
	if err != nil {
		reason := sentence(err.Error())
		if errors.Is(err, context.DeadlineExceeded) {
			reason = inst.Label() + " did not report its limits in time."
		}
		return Limits{
			State:     LimitsUnavailable,
			Windows:   prev.Windows,
			UpdatedAt: prev.UpdatedAt,
			Reason:    reason,
			CheckedAt: now,
		}
	}
	return Limits{State: LimitsOK, Windows: windows, UpdatedAt: now, CheckedAt: now}
}

// sentence turns an error's text into the sentence the app shows: capitalised,
// with a full stop.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[n:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
