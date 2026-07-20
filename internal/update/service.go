package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// DefaultInterval is how often the background worker re-checks for a new
// release.
const DefaultInterval = time.Hour

// Service keeps the latest-release check cached in memory and refreshes it on a
// ticker, so the API can answer instantly without hitting the network on every
// poll. It also downloads the installer `.pkg` on demand. It is safe for
// concurrent use.
type Service struct {
	fetcher  Fetcher
	interval time.Duration

	// download configuration (injectable for tests)
	client  *http.Client
	destDir string
	open    func(ctx context.Context, path string) error

	// checkMu serializes fetches so a manual check and the hourly worker never
	// overlap (they'd otherwise double-fetch and race the checking flag).
	checkMu sync.Mutex

	mu          sync.Mutex
	releases    []Release
	lastErr     error
	checkedAt   time.Time
	checking    bool
	downloading bool
}

// Snapshot is a point-in-time view of the cached check result. Releases holds
// recent published releases (newest first); Latest is the highest-versioned of
// them.
type Snapshot struct {
	Releases    []Release
	Err         string
	CheckedAt   time.Time
	Checking    bool
	Downloading bool
}

// Latest returns the highest-versioned cached release, or nil if none.
func (s Snapshot) Latest() *Release { return Latest(s.Releases) }

// Option configures a [Service].
type Option func(*Service)

// WithHTTPClient sets the HTTP client used to download the installer.
func WithHTTPClient(c *http.Client) Option { return func(s *Service) { s.client = c } }

// WithDownloadDir sets the directory downloads are written to (default
// ~/Downloads).
func WithDownloadDir(dir string) Option { return func(s *Service) { s.destDir = dir } }

// WithOpener overrides how a downloaded installer is launched (default: `open`
// the file, which hands it to the macOS Installer).
func WithOpener(fn func(ctx context.Context, path string) error) Option {
	return func(s *Service) { s.open = fn }
}

// NewService builds a Service. interval <= 0 uses [DefaultInterval].
func NewService(f Fetcher, interval time.Duration, opts ...Option) *Service {
	if interval <= 0 {
		interval = DefaultInterval
	}
	s := &Service{
		fetcher:  f,
		interval: interval,
		client:   &http.Client{Timeout: 5 * time.Minute},
		open:     openWithFinder,
	}
	for _, o := range opts {
		o(s)
	}
	if s.destDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			s.destDir = filepath.Join(home, "Downloads")
		} else {
			s.destDir = os.TempDir()
		}
	}
	return s
}

// Run checks once immediately, then re-checks every interval until ctx is done.
// It is meant to be run in its own goroutine.
func (s *Service) Run(ctx context.Context) {
	_, _ = s.CheckNow(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.CheckNow(ctx)
		}
	}
}

// CheckNow fetches recent releases synchronously and updates the cache. Checks
// are serialized, so a manual check while the hourly worker is mid-fetch waits
// rather than issuing a second overlapping request.
func (s *Service) CheckNow(ctx context.Context) ([]Release, error) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()

	s.mu.Lock()
	s.checking = true
	s.mu.Unlock()

	releases, err := s.fetcher.Releases(ctx)

	s.mu.Lock()
	s.checking = false
	s.checkedAt = time.Now()
	if err != nil {
		s.lastErr = err
	} else {
		s.releases = releases
		s.lastErr = nil
	}
	s.mu.Unlock()
	return releases, err
}

// Snapshot returns the cached check result without touching the network.
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{
		Releases:    s.releases,
		CheckedAt:   s.checkedAt,
		Checking:    s.checking,
		Downloading: s.downloading,
	}
	if s.lastErr != nil {
		snap.Err = s.lastErr.Error()
	}
	return snap
}

// Download fetches the cached latest release's `.pkg` into the download
// directory and opens it (launching the macOS Installer). It returns the path
// of the downloaded file. Only one download runs at a time.
func (s *Service) Download(ctx context.Context) (string, error) {
	s.mu.Lock()
	rel := Latest(s.releases)
	if s.downloading {
		s.mu.Unlock()
		return "", errors.New("a download is already in progress")
	}
	if rel == nil || rel.PkgURL == "" {
		s.mu.Unlock()
		return "", errors.New("no installer is available to download")
	}
	s.downloading = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.downloading = false
		s.mu.Unlock()
	}()

	dest, err := s.downloadPkg(ctx, rel)
	if err != nil {
		return "", err
	}
	if err := s.open(ctx, dest); err != nil {
		// The file is on disk; surface where it landed so the user can open it
		// manually if launching the Installer failed.
		return dest, fmt.Errorf("open installer %q: %w", dest, err)
	}
	return dest, nil
}

// downloadPkg streams the release's `.pkg` to a temp file and renames it into
// place, so an interrupted download never leaves a truncated installer behind.
func (s *Service) downloadPkg(ctx context.Context, rel *Release) (string, error) {
	if err := os.MkdirAll(s.destDir, 0o755); err != nil {
		return "", fmt.Errorf("create download dir: %w", err)
	}
	// Reduce the asset name to a bare file name so a crafted release asset (e.g.
	// "../../evil.pkg") can never write outside the download directory.
	name := filepath.Base(rel.PkgName)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = fmt.Sprintf("claudeq-%s.pkg", rel.Version)
	}
	dest := filepath.Join(s.destDir, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.PkgURL, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download installer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download installer: unexpected status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(s.destDir, ".claudeq-dl-*.pkg")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write installer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close installer: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", fmt.Errorf("finalize installer: %w", err)
	}
	return dest, nil
}

// openWithFinder hands the downloaded package to macOS, which opens it in the
// Installer app.
func openWithFinder(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "open", path).Run()
}
