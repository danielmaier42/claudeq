package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeFetcher returns canned releases or an error, counting calls.
type fakeFetcher struct {
	mu    sync.Mutex
	rels  []Release
	err   error
	calls int
}

func (f *fakeFetcher) Releases(context.Context) ([]Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.rels, f.err
}

func (f *fakeFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestServiceCheckNowCachesResult(t *testing.T) {
	f := &fakeFetcher{rels: []Release{{Version: "0.2.0", Tag: "v0.2.0", PkgURL: "u", PkgName: "p.pkg"}}}
	s := NewService(f, time.Hour)

	// Before any check the snapshot is empty.
	if snap := s.Snapshot(); snap.Latest() != nil {
		t.Fatal("expected empty snapshot before first check")
	}

	rels, err := s.CheckNow(context.Background())
	if err != nil || len(rels) != 1 || rels[0].Version != "0.2.0" {
		t.Fatalf("CheckNow = %+v, %v", rels, err)
	}
	snap := s.Snapshot()
	if snap.Latest() == nil || snap.Latest().Version != "0.2.0" {
		t.Fatalf("snapshot latest = %+v", snap.Latest())
	}
	if snap.Checking {
		t.Error("checking should be false after CheckNow returns")
	}
	if snap.CheckedAt.IsZero() {
		t.Error("checkedAt should be set")
	}
}

func TestServiceLatestPicksHighestVersion(t *testing.T) {
	// Out-of-order list: the highest version must win regardless of position.
	f := &fakeFetcher{rels: []Release{{Version: "0.1.9"}, {Version: "0.2.0"}, {Version: "0.1.10"}}}
	s := NewService(f, time.Hour)
	if _, err := s.CheckNow(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if got := s.Snapshot().Latest(); got == nil || got.Version != "0.2.0" {
		t.Fatalf("latest = %+v, want 0.2.0", got)
	}
}

func TestServiceCheckNowKeepsLastGoodOnError(t *testing.T) {
	f := &fakeFetcher{rels: []Release{{Version: "0.2.0"}}}
	s := NewService(f, time.Hour)
	if _, err := s.CheckNow(context.Background()); err != nil {
		t.Fatalf("first check: %v", err)
	}
	// Now make it fail; the cached good release must survive, error recorded.
	f.mu.Lock()
	f.rels, f.err = nil, errors.New("network down")
	f.mu.Unlock()
	if _, err := s.CheckNow(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	snap := s.Snapshot()
	if snap.Latest() == nil || snap.Latest().Version != "0.2.0" {
		t.Errorf("cached release should survive an error, got %+v", snap.Latest())
	}
	if snap.Err == "" {
		t.Error("error should be recorded in the snapshot")
	}
}

func TestServiceRunChecksAndStops(t *testing.T) {
	f := &fakeFetcher{rels: []Release{{Version: "1.0.0"}}}
	s := NewService(f, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	// Wait for at least the initial check plus one tick.
	deadline := time.Now().Add(2 * time.Second)
	for s.snapshotCalls() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.callCount() < 2 {
		t.Fatalf("expected repeated checks, got %d", f.callCount())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// snapshotCalls is a tiny helper mirroring fetcher call count via the snapshot's
// CheckedAt not being enough; we just proxy to the fetcher.
func (s *Service) snapshotCalls() int {
	if f, ok := s.fetcher.(*fakeFetcher); ok {
		return f.callCount()
	}
	return 0
}

func TestServiceDownloadWritesFileAndOpens(t *testing.T) {
	const body = "PKGDATA-installer-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	var opened string
	f := &fakeFetcher{rels: []Release{{Version: "0.3.0", PkgURL: srv.URL + "/claudeq-0.3.0.pkg", PkgName: "claudeq-0.3.0.pkg"}}}
	s := NewService(f, time.Hour,
		WithDownloadDir(dir),
		WithHTTPClient(srv.Client()),
		WithOpener(func(_ context.Context, path string) error { opened = path; return nil }),
	)
	if _, err := s.CheckNow(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}

	path, err := s.Download(context.Background())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	want := filepath.Join(dir, "claudeq-0.3.0.pkg")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if opened != want {
		t.Errorf("opener called with %q, want %q", opened, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != body {
		t.Errorf("downloaded content = %q, want %q", data, body)
	}
	// No leftover temp files in the download dir.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected exactly the .pkg in dir, got %d entries", len(entries))
	}
}

func TestServiceDownloadNoRelease(t *testing.T) {
	s := NewService(&fakeFetcher{}, time.Hour, WithDownloadDir(t.TempDir()))
	if _, err := s.Download(context.Background()); err == nil {
		t.Fatal("expected an error when no release is cached")
	}
}

// A crafted asset name must not escape the download directory.
func TestServiceDownloadSanitizesAssetName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	f := &fakeFetcher{rels: []Release{{Version: "0.3.0", PkgURL: srv.URL + "/x.pkg", PkgName: "../../../evil.pkg"}}}
	s := NewService(f, time.Hour, WithDownloadDir(dir),
		WithHTTPClient(srv.Client()),
		WithOpener(func(context.Context, string) error { return nil }))
	if _, err := s.CheckNow(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	path, err := s.Download(context.Background())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Dir(path) != filepath.Clean(dir) {
		t.Fatalf("download escaped the dir: %q (dir %q)", path, dir)
	}
	if filepath.Base(path) != "evil.pkg" {
		t.Errorf("base name = %q, want evil.pkg", filepath.Base(path))
	}
}
