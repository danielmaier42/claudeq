package provider

import (
	"context"
	"testing"
	"time"
)

// countingAdapter records how often it was asked, which is how the cache tests
// tell a reused verdict from a fresh probe.
type countingAdapter struct {
	*fakeAdapter
	checks int
}

func (c *countingAdapter) CheckHealth(_ context.Context, _ Instance, _ Prober) Health {
	c.checks++
	return c.health
}

func newCountingChecker(t *testing.T, kind Kind) (*Checker, *countingAdapter, *time.Time) {
	t.Helper()
	ad := &countingAdapter{fakeAdapter: newFakeAdapter(kind)}
	now := time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC)
	clock := &now
	return &Checker{
		Registry: NewRegistry(ad),
		Now:      func() time.Time { return *clock },
		TTL:      30 * time.Second,
	}, ad, clock
}

func enabled(id string, kind Kind) Instance {
	return Instance{ID: id, Kind: kind, Name: id, Enabled: true}
}

func TestCheckerReusesARecentVerdict(t *testing.T) {
	ch, ad, now := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")

	if h := ch.Check(context.Background(), inst); !h.Ready() {
		t.Fatalf("health = %+v, want ready", h)
	}
	ch.Check(context.Background(), inst)
	if ad.checks != 1 {
		t.Fatalf("adapter probed %d times, want the second answer to come from the cache", ad.checks)
	}

	*now = now.Add(31 * time.Second)
	ch.Check(context.Background(), inst)
	if ad.checks != 2 {
		t.Fatalf("adapter probed %d times, want a re-probe once the verdict aged out", ad.checks)
	}
}

func TestCheckFreshIgnoresTheCache(t *testing.T) {
	ch, ad, _ := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")

	ch.Check(context.Background(), inst)
	ch.CheckFresh(context.Background(), inst)
	if ad.checks != 2 {
		t.Fatalf("adapter probed %d times, want CheckFresh to probe again", ad.checks)
	}
}

// TestCheckerInvalidatesOnAConfigurationChange is what keeps the app from
// reporting a verdict that belongs to a path the operator has already replaced.
func TestCheckerInvalidatesOnAConfigurationChange(t *testing.T) {
	ch, ad, _ := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")

	ch.Check(context.Background(), inst)
	inst.BinaryPath = "/somewhere/else"
	ch.Check(context.Background(), inst)
	if ad.checks != 2 {
		t.Fatalf("adapter probed %d times, want a changed binary path to invalidate the verdict", ad.checks)
	}

	ch.Forget(inst.ID)
	ch.Check(context.Background(), inst)
	if ad.checks != 3 {
		t.Fatalf("adapter probed %d times, want Forget to drop the verdict", ad.checks)
	}
}

func TestCheckerVerdictsWithoutProbing(t *testing.T) {
	tests := []struct {
		name string
		inst Instance
		want HealthState
	}{
		{
			name: "a switched-off instance is not probed",
			inst: Instance{ID: "fake", Kind: "fake", Name: "fake"},
			want: HealthDisabled,
		},
		{
			name: "an instance of an unimplemented kind is a configuration problem",
			inst: enabled("other", "gremlin"),
			want: HealthInvalidConfiguration,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch, ad, _ := newCountingChecker(t, "fake")
			h := ch.Check(context.Background(), tc.inst)
			if h.State != tc.want {
				t.Fatalf("state = %q, want %q", h.State, tc.want)
			}
			if h.Reason == "" {
				t.Fatal("an unready verdict must say what is wrong")
			}
			if ad.checks != 0 {
				t.Fatalf("adapter probed %d times, want no CLI call at all", ad.checks)
			}
		})
	}
}

func TestStatusesMarkTheDefaultInstance(t *testing.T) {
	ch, _, _ := newCountingChecker(t, "fake")
	set, err := NewSet("second", []Instance{enabled("first", "fake"), enabled("second", "fake")})
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	got := ch.Statuses(context.Background(), set)
	if len(got) != 2 {
		t.Fatalf("got %d statuses, want one per instance", len(got))
	}
	if got[0].Default || !got[1].Default {
		t.Fatalf("default marked on %+v", got)
	}
	if !got[0].Health.Ready() || got[0].Health.CheckedAt.IsZero() {
		t.Fatalf("health = %+v, want a ready verdict with a timestamp", got[0].Health)
	}
}

// TestKnownUnreadyIsWeakerThanReady separates the two questions the app asks of
// a verdict: may a job *start* on this provider (Ready), and is it *known* not
// to work (KnownUnready). A check that could not reach a verdict answers no to
// both: nothing is started on it, but a follow-up task is still filed.
func TestKnownUnreadyIsWeakerThanReady(t *testing.T) {
	tests := []struct {
		state           HealthState
		ready, knownBad bool
	}{
		{state: HealthReady, ready: true},
		{state: HealthNotInstalled, knownBad: true},
		{state: HealthNotAuthenticated, knownBad: true},
		{state: HealthInvalidConfiguration, knownBad: true},
		{state: HealthDisabled, knownBad: true},
		{state: HealthCheckFailed},
	}
	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			h := Health{State: tc.state}
			if h.Ready() != tc.ready {
				t.Errorf("Ready() = %t, want %t", h.Ready(), tc.ready)
			}
			if h.KnownUnready() != tc.knownBad {
				t.Errorf("KnownUnready() = %t, want %t", h.KnownUnready(), tc.knownBad)
			}
		})
	}
}

func TestReasonOr(t *testing.T) {
	if got := (Health{Reason: "no login"}).ReasonOr("fallback"); got != "no login" {
		t.Errorf("ReasonOr = %q, want the verdict's own reason", got)
	}
	if got := (Health{Reason: "  "}).ReasonOr("fallback"); got != "fallback" {
		t.Errorf("ReasonOr = %q, want the fallback", got)
	}
}

// TestCheckMaybeFreshProbesExactlyOnce guards against computing a verdict only
// to throw it away.
func TestCheckMaybeFreshProbesExactlyOnce(t *testing.T) {
	ch, ad, _ := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")
	ch.CheckMaybeFresh(context.Background(), inst, true)
	if ad.checks != 1 {
		t.Fatalf("adapter probed %d times, want one", ad.checks)
	}
	ch.CheckMaybeFresh(context.Background(), inst, false)
	if ad.checks != 1 {
		t.Fatalf("adapter probed %d times, want the cached verdict", ad.checks)
	}
}

// TestACutShortProbeIsNotAVerdict: a probe killed because the caller went away
// — a cancelled request, a daemon shutting down — says nothing about the
// provider. Remembering it would show a passing blip as a provider problem for
// as long as the verdict lives.
func TestACutShortProbeIsNotAVerdict(t *testing.T) {
	ch, ad, _ := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")

	if h := ch.Check(context.Background(), inst); !h.Ready() {
		t.Fatalf("health = %+v, want the provider ready first", h)
	}

	// Now the request goes away mid-probe.
	ad.health = Health{State: HealthCheckFailed, Reason: "signal: killed"}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if h := ch.CheckFresh(cancelled, inst); !h.Ready() {
		t.Fatalf("health = %+v, want the last real verdict, not the interruption", h)
	}
	// And it must not have been remembered either.
	ad.health = Health{State: HealthReady}
	if h := ch.Check(context.Background(), inst); !h.Ready() {
		t.Fatalf("health = %+v, want the provider still ready", h)
	}
}

// TestACutShortFirstProbeReportsWhatItHas: with nothing to fall back to, the
// interrupted answer is returned — but still not remembered, so the next check
// asks again instead of repeating it.
func TestACutShortFirstProbeReportsWhatItHas(t *testing.T) {
	ch, ad, _ := newCountingChecker(t, "fake")
	inst := enabled("fake", "fake")
	ad.health = Health{State: HealthCheckFailed, Reason: "signal: killed"}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if h := ch.CheckFresh(cancelled, inst); h.State != HealthCheckFailed {
		t.Fatalf("health = %+v, want what the interrupted probe produced", h)
	}
	ad.health = Health{State: HealthReady}
	if h := ch.Check(context.Background(), inst); !h.Ready() {
		t.Fatalf("health = %+v, want a fresh answer rather than the remembered interruption", h)
	}
}
