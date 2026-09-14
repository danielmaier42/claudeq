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
			inst: enabled("other", "opencode"),
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
