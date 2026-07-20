package update

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"v0.1.4":  "0.1.4",
		" v1.2.3": "1.2.3",
		"1.2.3":   "1.2.3",
		"dev":     "dev",
		"":        "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsReleaseVersion(t *testing.T) {
	cases := map[string]bool{
		"v0.1.4":    true,
		"0.1.4":     true,
		"1.0":       true,
		"2":         true,
		"1.2.3-rc1": true,
		"dev":       false,
		"":          false,
		"latest":    false,
		"v":         false,
		"1.2.beta":  false,
	}
	for in, want := range cases {
		if got := IsReleaseVersion(in); got != want {
			t.Errorf("IsReleaseVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCompareAndIsNewer(t *testing.T) {
	type tc struct {
		a, b string
		want int
	}
	cases := []tc{
		{"0.1.4", "0.1.3", 1},
		{"0.1.3", "0.1.4", -1},
		{"0.1.4", "0.1.4", 0},
		{"v0.2.0", "0.1.9", 1},
		{"0.2", "0.2.0", 0}, // missing trailing component counts as 0
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1},    // numeric, not lexicographic
		{"1.2.3-rc1", "1.2.3", 0}, // pre-release suffix dropped
		{"", "0.0.1", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := IsNewer(c.a, c.b); got != (c.want > 0) {
			t.Errorf("IsNewer(%q,%q) = %v, want %v", c.a, c.b, got, c.want > 0)
		}
	}
}

func TestLatest(t *testing.T) {
	if Latest(nil) != nil {
		t.Error("Latest(nil) should be nil")
	}
	rels := []Release{{Version: "0.1.9"}, {Version: "0.2.0"}, {Version: "0.1.10"}}
	if got := Latest(rels); got == nil || got.Version != "0.2.0" {
		t.Errorf("Latest = %+v, want 0.2.0", got)
	}
}

func TestNewerThan(t *testing.T) {
	rels := []Release{{Version: "0.1.2"}, {Version: "0.1.4"}, {Version: "0.1.3"}, {Version: "0.1.1"}}

	// Skipped 0.1.1 -> should see 0.1.4, 0.1.3, 0.1.2 (newest first).
	got := NewerThan(rels, "0.1.1")
	if len(got) != 3 {
		t.Fatalf("expected 3 newer releases, got %d: %+v", len(got), got)
	}
	if got[0].Version != "0.1.4" || got[1].Version != "0.1.3" || got[2].Version != "0.1.2" {
		t.Errorf("order = %v, want newest-first 0.1.4,0.1.3,0.1.2", vers(got))
	}

	// On the latest -> nothing newer.
	if n := len(NewerThan(rels, "0.1.4")); n != 0 {
		t.Errorf("on latest: expected 0 newer, got %d", n)
	}
	// Dev build -> nothing (never "behind").
	if n := len(NewerThan(rels, "dev")); n != 0 {
		t.Errorf("dev build: expected 0 newer, got %d", n)
	}
}

func vers(rels []Release) []string {
	out := make([]string, len(rels))
	for i, r := range rels {
		out[i] = r.Version
	}
	return out
}

func TestReleasesPageURL(t *testing.T) {
	if got := ReleasesPageURL(""); got != "https://github.com/"+DefaultRepo+"/releases" {
		t.Errorf("ReleasesPageURL(\"\") = %q", got)
	}
	if got := ReleasesPageURL("a/b"); got != "https://github.com/a/b/releases" {
		t.Errorf("ReleasesPageURL(a/b) = %q", got)
	}
}
