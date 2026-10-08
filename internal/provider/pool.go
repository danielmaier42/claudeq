package provider

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

// ErrUnknownPool means a selection names a pool nothing is configured under.
var ErrUnknownPool = errors.New("unknown pool")

// ErrInvalidPool is the base error for a pool configuration claudeq refuses to
// store.
var ErrInvalidPool = errors.New("invalid pool")

// Pool is a set of providers a task can be given as a whole: each run goes to
// the member whose allowance is most at risk of expiring unused (see
// [RankPool]). Its members are of one kind, so a model a task names means the
// same to every one of them.
type Pool struct {
	ID      string
	Name    string
	Members []PoolMember
}

// PoolMember is one provider of a pool and the weight it is given.
type PoolMember struct {
	ProviderID string
	// Weight is the member's allowance relative to the others'; zero takes the
	// plan size its provider reports (see LimitReading.Capacity), or 1.
	Weight float64
}

// Label is how a pool is named in text the operator reads.
func (p Pool) Label() string {
	if strings.TrimSpace(p.Name) != "" {
		return p.Name
	}
	return p.ID
}

// PoolOf reads a stored pool.
func PoolOf(p store.Pool) Pool {
	out := Pool{ID: p.ID, Name: p.Name, Members: make([]PoolMember, len(p.Members))}
	for i, m := range p.Members {
		out.Members[i] = PoolMember{ProviderID: m.Provider, Weight: m.Weight}
	}
	return out
}

// Stored returns the pool in the shape config.toml keeps.
func (p Pool) Stored() store.Pool {
	out := store.Pool{ID: p.ID, Name: p.Name, Members: make([]store.PoolMember, len(p.Members))}
	for i, m := range p.Members {
		out.Members[i] = store.PoolMember{Provider: m.ProviderID, Weight: m.Weight}
	}
	return out
}

// ValidatePool checks a pool against the configured providers: a usable id
// that no provider has too, at least one member, every member configured and
// named once, all members of one kind, and no negative weight.
func ValidatePool(set Set, p Pool) error {
	if err := CheckID(p.ID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPool, err)
	}
	if _, clash := set.Lookup(p.ID); clash {
		return fmt.Errorf("%w %q: a provider already has this id", ErrInvalidPool, p.ID)
	}
	if len(p.Members) == 0 {
		return fmt.Errorf("%w %q: a pool needs at least one member", ErrInvalidPool, p.ID)
	}
	seen := map[string]bool{}
	var kind Kind
	for _, m := range p.Members {
		inst, ok := set.Lookup(m.ProviderID)
		if !ok {
			return fmt.Errorf("%w %q: %w %q", ErrInvalidPool, p.ID, ErrUnknownProvider, m.ProviderID)
		}
		if seen[m.ProviderID] {
			return fmt.Errorf("%w %q: provider %q is a member twice", ErrInvalidPool, p.ID, m.ProviderID)
		}
		seen[m.ProviderID] = true
		if kind == "" {
			kind = inst.Kind
		} else if inst.Kind != kind {
			return fmt.Errorf("%w %q: every member must be of one type, and %q is not a %s provider",
				ErrInvalidPool, p.ID, m.ProviderID, kind)
		}
		if m.Weight < 0 || math.IsNaN(m.Weight) || math.IsInf(m.Weight, 0) {
			return fmt.Errorf("%w %q: the weight of %q must be zero (automatic) or more", ErrInvalidPool, p.ID, m.ProviderID)
		}
	}
	return nil
}

// MemberScore is where one member stands in its pool right now, and why. It is
// what the run log and the dashboard show, so the choice can be followed.
type MemberScore struct {
	ProviderID string `json:"provider"`
	Name       string `json:"name"`
	// Tier orders the members before their urgency is compared:
	// 0 has room in both windows, 1 has no reading to judge by, 2 is nearly out
	// of its 5-hour or weekly window, 3 cannot take work at all.
	Tier int `json:"tier"`
	// Urgency is the provider's own (see [ScoreProvider]): how far behind it
	// is on spending its long window, shared with the runs already on it.
	Urgency float64 `json:"urgency"`
	// Score is what a pool ranks its members by: urgency times weight, so a
	// larger plan with the same slack has more to lose. Higher goes first.
	Score float64 `json:"score"`
	// Weight is the plan size used for this member.
	Weight float64 `json:"weight"`
	// WeekFree is how much of the long window is left, in percent, and
	// ResetsAt when it starts over. Zero values when nothing was read.
	WeekFree float64    `json:"week_free"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Running is how many runs are on the member already.
	Running int `json:"running"`
	// Note says, in a few words, why the member is ranked where it is.
	Note string `json:"note"`
}

// RankInputs is what ranking a pool needs to know about its members.
type RankInputs struct {
	Now time.Time
	// Limits returns a member's last allowance reading.
	Limits func(inst Instance) (Limits, bool)
	// Running counts the runs a member has in flight (plus any the caller has
	// already planned onto it this pass).
	Running func(id string) int
	// Usable reports whether a member can take work at all: switched on, ready,
	// and not waiting out a rate limit. Nil accepts every enabled member.
	Usable func(inst Instance) (bool, string)
}

// DisplayInputs is how the Dashboard and `claudeq pool show` rank a pool:
// from the readings and readiness verdicts they have, and the rate-limit gates
// where the caller knows them (limited may be nil). It is one definition, so
// both show the same order the scheduler would take.
func DisplayInputs(now time.Time, limits func(Instance) (Limits, bool), health func(Instance) Health, limited func(id string) bool) RankInputs {
	return RankInputs{
		Now:    now,
		Limits: limits,
		Usable: func(inst Instance) (bool, string) {
			if limited != nil && limited(inst.ID) {
				return false, "waiting for its rate limit"
			}
			if h := health(inst); h.KnownUnready() {
				return false, h.ReasonOr("cannot run right now")
			}
			return true, ""
		},
	}
}

// Thresholds that put a member behind the ones with room. A window this full
// is about to stop the member anyway, and its share of a long run would start
// on an account that will pause it.
const (
	poolShortWindowFull = 90.0
	poolLongWindowFull  = 99.5
	// poolMinHours stops a window that resets in a few minutes from dwarfing
	// every other member: by the time a run on it gets going it has reset.
	poolMinHours = 0.25
)

// RankPool orders a pool's members, best first.
//
// The members that can take work and have room in both windows come first,
// ordered by their urgency (see [ScoreProvider]) times their weight. That puts
// work where the most allowance would otherwise expire unused soonest: an
// account with 10% left that resets in four hours goes before one with 60%
// left for five more days.
//
// Members with no reading follow, then members whose 5-hour or weekly window
// is all but full, then the ones that cannot take work. Within a tier, ties go
// to fewer running jobs and then to configuration order.
func RankPool(set Set, p Pool, in RankInputs) []MemberScore {
	out := make([]MemberScore, 0, len(p.Members))
	for _, m := range p.Members {
		inst, ok := set.Lookup(m.ProviderID)
		if !ok {
			out = append(out, MemberScore{ProviderID: m.ProviderID, Name: m.ProviderID, Tier: 3, Note: "not configured"})
			continue
		}
		out = append(out, scoreMember(inst, m, in))
	}
	order := make(map[string]int, len(out))
	for i, s := range out {
		order[s.ProviderID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Running != b.Running {
			return a.Running < b.Running
		}
		return order[a.ProviderID] < order[b.ProviderID]
	})
	return out
}

// ScoreProvider is where one provider stands on its own, outside any pool: the
// same tiers and notes a pool ranks by, and its urgency.
//
// Urgency is how far behind the provider is on spending its long (weekly)
// window: the free share of it, times the window's length, over the hours left
// until it resets, shared with the runs already on it. 1 means an even pace
// from now on uses up exactly what is left; 2 means it would take twice that
// pace, so half of what is left is at risk of expiring unused. A window that
// has just started over is at 1.
func ScoreProvider(inst Instance, in RankInputs) MemberScore {
	return scoreMember(inst, PoolMember{ProviderID: inst.ID, Weight: 1}, in)
}

// Spare reports whether a provider's allowance is at risk of going unused, as
// a backfill task asks it: it has room in both windows and its urgency is
// above threshold.
func (s MemberScore) Spare(threshold float64) bool {
	return s.Tier == 0 && s.Urgency > threshold
}

func scoreMember(inst Instance, m PoolMember, in RankInputs) MemberScore {
	s := MemberScore{ProviderID: inst.ID, Name: inst.Label(), Weight: 1}
	if in.Running != nil {
		s.Running = in.Running(inst.ID)
	}
	usable, why := inst.Enabled, "switched off"
	if usable && in.Usable != nil {
		usable, why = in.Usable(inst)
	}
	var l Limits
	var read bool
	if in.Limits != nil {
		l, read = in.Limits(inst)
	}
	switch {
	case m.Weight > 0:
		s.Weight = m.Weight
	case l.Capacity > 0:
		s.Weight = l.Capacity
	}
	if !usable {
		s.Tier, s.Note = 3, why
		return s
	}
	long, short := windowsOf(l.Windows)
	if !read || long == nil {
		s.Tier, s.Note = 1, "no limit reading yet"
		return s
	}
	// A window whose reset has passed since it was read has started over: its
	// figures say nothing any more, and the account is as fresh as it gets.
	week := *long
	freshWeek := week.ResetsAt != nil && !week.ResetsAt.After(in.Now)
	if freshWeek {
		week.UsedPercent, week.ResetsAt = 0, nil
	}
	if short != nil && short.ResetsAt != nil && !short.ResetsAt.After(in.Now) {
		short = nil
	}
	long = &week
	s.WeekFree = math.Max(0, 100-long.UsedPercent)
	s.ResetsAt = long.ResetsAt
	hours := windowHours(long.ID)
	if long.ResetsAt != nil {
		hours = math.Max(poolMinHours, long.ResetsAt.Sub(in.Now).Hours())
	}
	s.Urgency = s.WeekFree / 100 * windowHours(long.ID) / hours / float64(1+s.Running)
	s.Score = s.Urgency * s.Weight
	switch {
	case long.UsedPercent >= poolLongWindowFull:
		s.Tier, s.Note = 2, fmt.Sprintf("%s used up", strings.ToLower(long.Label))
	case short != nil && short.UsedPercent >= poolShortWindowFull:
		s.Tier, s.Note = 2, fmt.Sprintf("%.0f%% of %s used", short.UsedPercent, strings.ToLower(short.Label))
	case freshWeek:
		s.Note = strings.ToLower(long.Label) + " has reset since the last reading"
	default:
		s.Note = fmt.Sprintf("%.0f%% of %s left", s.WeekFree, strings.ToLower(long.Label))
		if long.ResetsAt != nil {
			s.Note += ", resets in " + roughDuration(long.ResetsAt.Sub(in.Now))
		}
	}
	return s
}

// windowHours is how long a window of this id lasts: "five_hour", "week",
// or "window_<minutes>" as a harness names any other length. An id that says
// nothing counts as a week.
func windowHours(id string) float64 {
	switch id {
	case "five_hour":
		return 5
	case "week":
		return 7 * 24
	}
	if rest, ok := strings.CutPrefix(id, "window_"); ok {
		if mins, err := strconv.Atoi(rest); err == nil && mins > 0 {
			return float64(mins) / 60
		}
	}
	return 7 * 24
}

// windowsOf picks the long window a pool balances on (the week, or the longest
// one there is) and the 5-hour window that can stop a member before it.
func windowsOf(ws []LimitWindow) (long, short *LimitWindow) {
	for i := range ws {
		switch ws[i].ID {
		case "week":
			long = &ws[i]
		case "five_hour":
			short = &ws[i]
		}
	}
	if long == nil {
		// No weekly window: the latest-resetting one stands in for it.
		for i := range ws {
			if ws[i].ResetsAt == nil || &ws[i] == short {
				continue
			}
			if long == nil || ws[i].ResetsAt.After(*long.ResetsAt) {
				long = &ws[i]
			}
		}
	}
	if long == nil && short != nil {
		long, short = short, nil
	}
	return long, short
}

// roughDuration is a duration the way a run log says it: "4 h", "2 d", "25 min".
func roughDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d d", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	case d > time.Minute:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return "a moment"
}

// PoolChoice records which member a pool run went to and how the members
// stood, for the run's log.
type PoolChoice struct {
	Pool   Pool
	Scores []MemberScore
	// Resumed is set when the member was chosen because it holds the task's
	// interrupted session, not by rank.
	Resumed bool
}

// Note is the sentence the run log opens with.
func (c PoolChoice) Note(chosen Instance) string {
	if c.Resumed {
		return fmt.Sprintf("Pool %s: continuing the interrupted session on %s", c.Pool.Label(), chosen.Label())
	}
	parts := make([]string, 0, len(c.Scores))
	for _, s := range c.Scores {
		parts = append(parts, fmt.Sprintf("%s (%s)", s.Name, s.Note))
	}
	return fmt.Sprintf("Pool %s chose %s: %s", c.Pool.Label(), chosen.Label(), strings.Join(parts, "; "))
}
