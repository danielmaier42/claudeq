package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// claudeProvider is the instance a migrated configuration produces. Tests build
// their config with it so they exercise the same shape the store seeds.
func claudeProvider(binary, model string) store.Provider {
	return store.Provider{
		ID: store.DefaultProviderID, Kind: store.DefaultProviderKind, Name: store.DefaultProviderName,
		BinaryPath: binary, DefaultModel: model, Enabled: true,
	}
}

// TestTickResolvesTheExecutionIdentity checks that what the engine hands the
// executor follows the resolution table, and that a migrated configuration
// still produces exactly the Claude Code run it always did.
func TestTickResolvesTheExecutionIdentity(t *testing.T) {
	tests := []struct {
		name           string
		binary         string
		providerModel  string
		taskProvider   string
		taskModel      string
		permissions    task.Permissions
		wantProviderID string
		wantBinary     string
		wantModel      string
		wantAccess     provider.AccessMode
	}{
		{
			name:           "migrated configuration runs on the claude instance",
			binary:         "/opt/claude",
			providerModel:  "sonnet",
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantBinary:     "/opt/claude",
			wantModel:      "sonnet",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "a task model overrides the provider default",
			providerModel:  "sonnet",
			taskModel:      "opus",
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantModel:      "opus",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "naming the default provider explicitly changes nothing",
			providerModel:  "sonnet",
			taskProvider:   provider.DefaultInstanceID,
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantModel:      "sonnet",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "skip permissions asks for full access",
			permissions:    task.PermissionsSkip,
			wantProviderID: provider.DefaultInstanceID,
			wantAccess:     provider.AccessFullAccess,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
			r := &stub{}
			e, st := newTestEngine(t, r, fc)

			tk := asapTask("a", false)
			tk.Provider, tk.Model, tk.Permissions = tc.taskProvider, tc.taskModel, tc.permissions
			if err := st.SaveConfig(store.Config{
				Providers: []store.Provider{claudeProvider(tc.binary, tc.providerModel)},
				Tasks:     []task.Task{tk},
			}); err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}

			if err := e.Tick(context.Background()); err != nil {
				t.Fatalf("Tick: %v", err)
			}
			e.WaitIdle()

			reqs := r.requests()
			if len(reqs) != 1 {
				t.Fatalf("expected 1 run, got %d", len(reqs))
			}
			got := reqs[0]
			if got.Provider.ID != tc.wantProviderID {
				t.Fatalf("provider = %q, want %q", got.Provider.ID, tc.wantProviderID)
			}
			if got.Provider.Kind != provider.KindClaudeCode {
				t.Fatalf("kind = %q, want %q", got.Provider.Kind, provider.KindClaudeCode)
			}
			if got.Provider.BinaryPath != tc.wantBinary {
				t.Fatalf("binary = %q, want %q", got.Provider.BinaryPath, tc.wantBinary)
			}
			if got.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", got.Model, tc.wantModel)
			}
			if got.AccessMode != tc.wantAccess {
				t.Fatalf("access mode = %q, want %q", got.AccessMode, tc.wantAccess)
			}
		})
	}
}

// notInstalled is the verdict of a provider whose CLI is missing.
var notInstalled = provider.Health{
	State:  provider.HealthNotInstalled,
	Reason: "Claude Code was not found.",
}

// TestTickLeavesATaskQueuedWhileItsProviderIsUnready is the whole point of the
// readiness gate: nothing is started, nothing is recorded, and no scheduling
// state moves — so the task simply runs later instead of failing at 3am.
func TestTickLeavesATaskQueuedWhileItsProviderIsUnready(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	ad.set(notInstalled)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got := r.requests(); len(got) != 0 {
		t.Fatalf("an unready provider must start nothing, got %+v", got)
	}
	runs, err := st.Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("a blocked task must record no run, got %+v", runs)
	}
	state, err := st.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.IsCompletedOnce("a") {
		t.Fatal("a blocked one-shot task must not be marked completed")
	}
	if _, ok := state.LastRun("a"); ok {
		t.Fatal("a blocked task must not record a start")
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Tasks) != 1 {
		t.Fatalf("the task should still be queued, got %+v", cfg.Tasks)
	}

	// Ready again: it starts by itself, without anyone touching the task.
	ad.set(provider.Health{State: provider.HealthReady})
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if got := r.requests(); len(got) != 1 {
		t.Fatalf("expected exactly one run once the provider was ready, got %d", len(got))
	}
}

// TestBlockedTaskTakesNoConcurrencySlot checks that a blocked task does not
// stand in the way of one that can run: the provider check happens before the
// priority and parallelism rules are applied.
func TestBlockedTaskTakesNoConcurrencySlot(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	ad.set(provider.Health{State: provider.HealthReady})

	// Two instances: the higher-priority task runs on the unready one.
	blockedProvider := claudeProvider("", "")
	blockedProvider.ID, blockedProvider.Name, blockedProvider.Kind = "blocked", "blocked", "unimplemented"
	first, second := asapTask("blocked-task", false), asapTask("runnable", false)
	first.Provider = "blocked"
	if err := st.SaveConfig(store.Config{
		Settings:  store.Settings{DefaultProvider: store.DefaultProviderID},
		Providers: []store.Provider{blockedProvider, claudeProvider("", "")},
		Tasks:     []task.Task{first, second},
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 || reqs[0].Task.ID != "runnable" {
		t.Fatalf("runs = %+v, want only the task whose provider works", reqs)
	}
}

// TestTickLeavesATaskNamingAnUnknownProviderQueued covers a configuration that
// no longer has the instance a task asks for. There is no substitution and no
// repeated failing run: the task waits, visibly, for someone to fix it.
func TestTickLeavesATaskNamingAnUnknownProviderQueued(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)

	tk := asapTask("a", false)
	tk.Provider = "codex-work"
	saveTasks(t, st, tk)

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got := r.requests(); len(got) != 0 {
		t.Fatalf("an unresolvable provider must not run anything, got %+v", got)
	}
	runs, err := st.Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("nothing ran, so nothing should be recorded, got %+v", runs)
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Tasks) != 1 {
		t.Fatalf("the task should still be queued, got %+v", cfg.Tasks)
	}
}

// TestRunNowReportsAnUnreadyProvider: a manual run answers whoever pressed the
// button instead of filing a failed run they then have to go and read.
func TestRunNowReportsAnUnreadyProvider(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	ad.set(notInstalled)
	saveTasks(t, st, asapTask("a", false))

	err := e.RunTaskNow(context.Background(), "a")
	if err == nil {
		t.Fatal("expected run-now to refuse")
	}
	if !strings.Contains(err.Error(), "Claude Code was not found") {
		t.Fatalf("err = %q, want the readiness reason", err)
	}
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("nothing may run, got %+v", got)
	}
}

// recordingNotifier collects what the engine sent.
type recordingNotifier struct {
	mu   sync.Mutex
	sent []notify.Notification
}

func (n *recordingNotifier) Notify(_ context.Context, msg notify.Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, msg)
	return nil
}

func (n *recordingNotifier) titles() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.sent))
	for i, m := range n.sent {
		out[i] = m.Title
	}
	return out
}

// TestProviderHealthNotifiesOncePerTransition: the scheduler looks at a blocked
// task on every tick, and an alert per tick would bury the one that matters.
func TestProviderHealthNotifiesOncePerTransition(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	n := &recordingNotifier{}
	e.SetNotifier(n)
	ad.set(notInstalled)
	saveTasks(t, st, asapTask("a", false))

	for range 3 {
		if err := e.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	e.WaitIdle()
	if got := n.titles(); len(got) != 1 || !strings.Contains(got[0], "cannot run tasks") {
		t.Fatalf("notifications = %v, want exactly one about the blocked provider", got)
	}
	if body := n.sent[0].Message; !strings.Contains(body, "Claude Code was not found") {
		t.Fatalf("message = %q, want the reason", body)
	}

	// Recovery is worth exactly one more.
	ad.set(provider.Health{State: provider.HealthReady})
	for range 2 {
		if err := e.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
		e.WaitIdle()
	}
	got := n.titles()
	if len(got) != 2 || !strings.Contains(got[1], "ready again") {
		t.Fatalf("notifications = %v, want one recovery alert", got)
	}
}

// TestProviderHealthMemoSurvivesARestart: the daemon restarting is not a change
// in the provider's state, so it must not re-announce it.
func TestProviderHealthMemoSurvivesARestart(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	n := &recordingNotifier{}
	e.SetNotifier(n)
	ad.set(notInstalled)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	// A second engine over the same store is what a restart looks like.
	restarted := New(st, e.gate, r, fc, e.providers)
	restarted.SetNotifier(n)
	if err := restarted.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after restart: %v", err)
	}
	restarted.WaitIdle()

	if got := n.titles(); len(got) != 1 {
		t.Fatalf("notifications = %v, want the condition announced once across the restart", got)
	}
}

// TestFirstHealthyObservationIsSilent: a working provider is not news.
func TestFirstHealthyObservationIsSilent(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	n := &recordingNotifier{}
	e.SetNotifier(n)
	ad.set(provider.Health{State: provider.HealthReady})
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	for _, title := range n.titles() {
		if strings.Contains(title, "ready again") {
			t.Fatalf("notifications = %v, want nothing about a provider that was fine all along", n.titles())
		}
	}
}

// blockingAdapter holds every readiness probe until the test lets it go, which
// is what a hung CLI looks like from the scheduler's side.
type blockingAdapter struct {
	*healthAdapter
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingAdapter) CheckHealth(ctx context.Context, inst provider.Instance, p provider.Prober) provider.Health {
	a.once.Do(func() { close(a.entered) })
	<-a.release
	return a.healthAdapter.CheckHealth(ctx, inst, p)
}

// TestTickDoesNotHoldTheSchedulerLockWhileProbing: a readiness check spawns a
// CLI, and a CLI can hang. While one does, the dashboard must still be able to
// ask what is running, and a run must still be cancellable.
func TestTickDoesNotHoldTheSchedulerLockWhileProbing(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	ad := &blockingAdapter{
		healthAdapter: &healthAdapter{health: provider.Health{State: provider.HealthReady}},
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	e := New(st, limit.New(fc), r, fc, &provider.Checker{Registry: provider.NewRegistry(ad)})
	saveTasks(t, st, asapTask("a", false))

	ticked := make(chan error, 1)
	go func() { ticked <- e.Tick(context.Background()) }()
	<-ad.entered

	// The probe is in flight. Anything that needs the scheduler lock must answer
	// rather than queue up behind it.
	answered := make(chan struct{})
	go func() { e.ActiveTaskIDs(); close(answered) }()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		t.Fatal("ActiveTaskIDs blocked behind a provider probe")
	}

	close(ad.release)
	if err := <-ticked; err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if got := r.requests(); len(got) != 1 {
		t.Fatalf("expected the task to run once the probe answered, got %d", len(got))
	}
}

// TestOneProbePerProviderPerTick: several tasks waiting on the same harness cost
// one check, not one each.
func TestOneProbePerProviderPerTick(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	ad := &countingHealthAdapter{healthAdapter: &healthAdapter{health: notInstalled}}
	e := New(st, limit.New(fc), r, fc, &provider.Checker{Registry: provider.NewRegistry(ad)})
	saveTasks(t, st, asapTask("a", false), asapTask("b", false), asapTask("c", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if ad.checks() != 1 {
		t.Fatalf("probed %d times for three tasks on one provider, want once", ad.checks())
	}
}

type countingHealthAdapter struct {
	*healthAdapter
	mu sync.Mutex
	n  int
}

func (a *countingHealthAdapter) CheckHealth(ctx context.Context, inst provider.Instance, p provider.Prober) provider.Health {
	a.mu.Lock()
	a.n++
	a.mu.Unlock()
	return a.healthAdapter.CheckHealth(ctx, inst, p)
}

func (a *countingHealthAdapter) checks() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

// TestARemovedProviderIsAnnouncedAgain: a provider removed and re-added under
// the same id is a different thing, so its state has to be reported afresh
// rather than inherited from the one that is gone.
func TestARemovedProviderIsAnnouncedAgain(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	n := &recordingNotifier{}
	e.SetNotifier(n)
	ad.set(notInstalled)

	second := claudeProvider("", "")
	second.ID, second.Name = "second", "second"
	tk := asapTask("a", false)
	tk.Provider = "second"
	base := store.Config{
		Settings:  store.Settings{DefaultProvider: store.DefaultProviderID},
		Providers: []store.Provider{claudeProvider("", ""), second},
		Tasks:     []task.Task{tk},
	}
	if err := st.SaveConfig(base); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if len(n.titles()) != 1 {
		t.Fatalf("notifications = %v, want the broken provider announced once", n.titles())
	}

	// Remove it — which also forgets the memo — and put it back.
	if err := app.RemoveTask(st, "a"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}
	if err := app.RemoveProvider(st, "second"); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if err := st.SaveConfig(base); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got := n.titles(); len(got) != 2 {
		t.Fatalf("notifications = %v, want the re-added provider announced again", got)
	}
}
