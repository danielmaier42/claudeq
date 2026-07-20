package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/update"
	"github.com/danielmaier42/claudeq/internal/version"
)

// stubFetcher implements update.Fetcher with canned releases.
type stubFetcher struct {
	rels []update.Release
	err  error
}

func (f stubFetcher) Releases(context.Context) ([]update.Release, error) { return f.rels, f.err }

// newUpdateServer wires an API server with an update service backed by stub
// releases and a stub installer download, and pins the running version. The
// first release's PkgURL "usePkgSrv" is rewritten to a live test asset server.
func newUpdateServer(t *testing.T, current string, rels ...update.Release) (*httptest.Server, *store.Store, *string) {
	t.Helper()
	orig := version.Version
	version.Version = current
	t.Cleanup(func() { version.Version = orig })

	// A fake GitHub asset endpoint for the download test.
	pkgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("installer-bytes"))
	}))
	t.Cleanup(pkgSrv.Close)
	for i := range rels {
		if rels[i].PkgURL == "usePkgSrv" {
			rels[i].PkgURL = pkgSrv.URL + "/x.pkg"
		}
	}

	opened := new(string)
	svc := update.NewService(stubFetcher{rels: rels}, time.Hour,
		update.WithDownloadDir(t.TempDir()),
		update.WithHTTPClient(pkgSrv.Client()),
		update.WithOpener(func(_ context.Context, path string) error { *opened = path; return nil }),
	)
	if len(rels) > 0 {
		if _, err := svc.CheckNow(context.Background()); err != nil {
			t.Fatalf("seed CheckNow: %v", err)
		}
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, Updates: svc}))
	t.Cleanup(srv.Close)
	return srv, st, opened
}

func TestUpdateStatusAvailable(t *testing.T) {
	srv, _, _ := newUpdateServer(t, "v0.1.0", update.Release{
		Version: "0.2.0", Tag: "v0.2.0", PkgURL: "https://example.test/x.pkg",
		PkgName: "claudeq-0.2.0.pkg", Notes: "new stuff", HTMLURL: "https://example.test",
	})
	var s updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s)

	if !s.Supported || !s.Available {
		t.Fatalf("expected supported+available, got %+v", s)
	}
	if s.Current != "0.1.0" || s.Latest != "0.2.0" {
		t.Errorf("current/latest = %q/%q", s.Current, s.Latest)
	}
	if !s.HasInstaller {
		t.Error("has_installer should be true")
	}
	if s.AllReleasesURL == "" {
		t.Error("all_releases_url should be set so the user can browse the full history")
	}
}

// A user who skipped versions must see the notes of every release in between,
// newest first, not just the latest.
func TestUpdateStatusAggregatesSkippedNotes(t *testing.T) {
	srv, _, _ := newUpdateServer(t, "v0.1.1",
		update.Release{Version: "0.1.4", Tag: "v0.1.4", PkgURL: "https://example.test/x.pkg", PkgName: "p.pkg", Notes: "notes for 0.1.4"},
		update.Release{Version: "0.1.3", Tag: "v0.1.3", Notes: "notes for 0.1.3"},
		update.Release{Version: "0.1.2", Tag: "v0.1.2", Notes: "notes for 0.1.2"},
		update.Release{Version: "0.1.1", Tag: "v0.1.1", Notes: "notes for 0.1.1 (already installed)"},
	)
	var s updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s)

	if !s.Available || s.Latest != "0.1.4" {
		t.Fatalf("expected 0.1.4 available, got %+v", s)
	}
	if s.SkippedCount != 3 {
		t.Errorf("skipped_count = %d, want 3 (0.1.4, 0.1.3, 0.1.2)", s.SkippedCount)
	}
	for _, want := range []string{"notes for 0.1.4", "notes for 0.1.3", "notes for 0.1.2"} {
		if !strings.Contains(s.Notes, want) {
			t.Errorf("aggregated notes missing %q\ngot: %s", want, s.Notes)
		}
	}
	// The already-installed version's notes must not be included.
	if strings.Contains(s.Notes, "already installed") {
		t.Errorf("notes should not include the installed version: %s", s.Notes)
	}
	// Order: newest (0.1.4) appears before older (0.1.2).
	if strings.Index(s.Notes, "0.1.4") > strings.Index(s.Notes, "0.1.2") {
		t.Errorf("notes should be newest-first:\n%s", s.Notes)
	}
}

func TestUpdateStatusUpToDate(t *testing.T) {
	srv, _, _ := newUpdateServer(t, "v0.2.0", update.Release{
		Version: "0.2.0", Tag: "v0.2.0", PkgURL: "https://example.test/x.pkg", PkgName: "p.pkg",
	})
	var s updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s)
	if s.Available {
		t.Errorf("should not be available when on the latest version: %+v", s)
	}
	if !s.Supported {
		t.Error("a real version should be supported")
	}
}

func TestUpdateStatusDevBuildUnsupported(t *testing.T) {
	srv, _, _ := newUpdateServer(t, "dev", update.Release{
		Version: "0.2.0", Tag: "v0.2.0", PkgURL: "https://example.test/x.pkg", PkgName: "p.pkg",
	})
	var s updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s)
	if s.Supported || s.Available {
		t.Errorf("dev build must not offer updates: %+v", s)
	}
}

func TestUpdateDismissSuppressesThenNewerReappears(t *testing.T) {
	srv, st, _ := newUpdateServer(t, "v0.1.0", update.Release{
		Version: "0.2.0", Tag: "v0.2.0", PkgURL: "https://example.test/x.pkg", PkgName: "p.pkg",
	})

	var s updateStatus
	do(t, srv, "POST", "/api/update/dismiss", map[string]string{"version": "0.2.0"}).into(t, &s)
	if s.Available {
		t.Fatalf("dismissed version should not be available: %+v", s)
	}
	// Persisted to state.
	state, _ := st.LoadState()
	if state.DismissedUpdate() != "0.2.0" {
		t.Errorf("dismissed version not persisted, got %q", state.DismissedUpdate())
	}
	// GET reflects the dismissal too.
	var s2 updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s2)
	if s2.Available {
		t.Errorf("GET should honor dismissal: %+v", s2)
	}
}

func TestUpdateDismissRequiresVersion(t *testing.T) {
	srv, _, _ := newUpdateServer(t, "v0.1.0", update.Release{Version: "0.2.0", PkgURL: "x", PkgName: "p.pkg"})
	if r := do(t, srv, "POST", "/api/update/dismiss", map[string]string{}); r.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 without a version, got %d", r.Status)
	}
}

func TestUpdateDownloadOpensInstaller(t *testing.T) {
	srv, _, opened := newUpdateServer(t, "v0.1.0", update.Release{
		Version: "0.2.0", Tag: "v0.2.0", PkgURL: "usePkgSrv", PkgName: "claudeq-0.2.0.pkg",
	})
	var out struct {
		Path string `json:"path"`
	}
	r := do(t, srv, "POST", "/api/update/download", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("download status = %d (%s)", r.Status, r.Body)
	}
	r.into(t, &out)
	if out.Path == "" {
		t.Fatal("expected a download path")
	}
	if *opened != out.Path {
		t.Errorf("installer not opened: opened=%q path=%q", *opened, out.Path)
	}
	if _, err := os.Stat(out.Path); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestUpdateEndpointsUnavailableWithoutService(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st})) // no Updates
	t.Cleanup(srv.Close)

	// GET returns a benign status (not an error) so the UI degrades gracefully.
	var s updateStatus
	do(t, srv, "GET", "/api/update", nil).into(t, &s)
	if s.Available || s.Supported {
		t.Errorf("no service should mean nothing offered: %+v", s)
	}
	// Mutating endpoints report unavailable.
	if r := do(t, srv, "POST", "/api/update/download", nil); r.Status != http.StatusServiceUnavailable {
		t.Errorf("download without service = %d, want 503", r.Status)
	}
}
