// Command claudeqd is the claudeq background daemon: it runs the scheduling
// loop that executes due tasks via the Claude Code CLI, and can install itself
// as a launchd LaunchAgent (PLAN.md build phases 2–3).
//
// Usage:
//
//	claudeqd --version
//	claudeqd run [--interval 5s]
//	claudeqd install     # install & start the LaunchAgent (autostart)
//	claudeqd uninstall    # stop & remove the LaunchAgent
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/danielmaier42/claudeq/internal/api"
	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/engine"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/launchd"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/system"
	"github.com/danielmaier42/claudeq/internal/version"
	"github.com/danielmaier42/claudeq/internal/wake"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claudeqd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: claudeqd run [--interval 5s] | install | uninstall | --version")
		return nil
	}
	switch args[0] {
	case "--version", "-version":
		fmt.Println(version.String())
		return nil
	case "run":
		return cmdRun(args[1:])
	case "install":
		return cmdInstall()
	case "uninstall":
		return cmdUninstall()
	default:
		fmt.Println("usage: claudeqd run [--interval 5s] | install | uninstall | --version")
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	interval := fs.Duration("interval", 5*time.Second, "scheduler tick interval")
	noWake := fs.Bool("no-wake", false, "do not schedule pmset wakes")
	addr := fs.String("addr", "127.0.0.1:8765", "dashboard/API listen address (loopback only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	home, err := store.DefaultHome()
	if err != nil {
		return err
	}
	st, err := store.Open(home)
	if err != nil {
		return err
	}

	c := clock.Real{}
	eng := engine.New(st, limit.New(c), &executor.Executor{}, c)
	if !*noWake {
		eng.SetWaker(&wake.Scheduler{Runner: system.Real{}, Sudo: true})
	}
	eng.SetNotifier(buildNotifier(st))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	accentFn := api.MacAccent(system.Real{})
	themeHub := api.NewThemeHub(accentFn())
	go watchAccent(ctx, themeHub, accentFn)

	httpSrv := &http.Server{
		Addr: *addr,
		Handler: api.Handler(api.Deps{
			Store: st, Runner: eng, Models: api.BinaryModelLister("claude"),
			ChooseFolder: api.OSAScriptFolderChooser(system.Real{}), ActiveTasks: eng.ActiveTaskIDs,
			Accent: accentFn, Theme: themeHub,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "claudeqd: http server:", err)
		}
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	fmt.Printf("claudeqd %s: watching %s\n  dashboard: http://%s  (tick %s, wake %t)\n",
		version.String(), home, *addr, *interval, !*noWake)
	if err := eng.Loop(ctx, *interval); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	fmt.Println("claudeqd: stopped")
	return nil
}

// watchAccent watches the macOS global preferences for accent-color changes and
// pushes them to the theme hub (event-driven, no polling). It watches the
// Preferences directory so it survives the atomic rewrite of the plist.
func watchAccent(ctx context.Context, hub *api.ThemeHub, read func() string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer func() { _ = w.Close() }()

	prefsDir := filepath.Join(home, "Library", "Preferences")
	globalPrefs := filepath.Join(prefsDir, ".GlobalPreferences.plist")
	if err := w.Add(prefsDir); err != nil {
		return
	}
	_ = w.Add(globalPrefs) // best-effort: also catch in-place writes

	var debounce *time.Timer
	fire := func() { hub.Publish(read()) }
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if !strings.Contains(ev.Name, "GlobalPreferences") {
				continue
			}
			// After an atomic replace the file watch points at the old inode;
			// re-arm it so the next in-place write is still caught.
			_ = w.Add(globalPrefs)
			// cfprefsd writes lazily; debounce and let it settle before reading.
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(400*time.Millisecond, fire)
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// buildNotifier assembles the notification channels: native macOS always, plus
// Pushover when credentials are configured (FA-39/40).
func buildNotifier(st *store.Store) notify.Notifier {
	notifiers := []notify.Notifier{notify.Mac{Runner: system.Real{}}}
	if cfg, err := st.LoadConfig(); err == nil {
		po := notify.Pushover{Token: cfg.Settings.Pushover.Token, UserKey: cfg.Settings.Pushover.UserKey}
		if cfg.Settings.Pushover.Enabled && po.Configured() {
			notifiers = append(notifiers, po)
		}
	}
	return notify.Multi{Notifiers: notifiers}
}

func cmdInstall() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	home, err := store.DefaultHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}

	cfg := launchd.Config{
		Label:      launchd.DefaultLabel,
		BinPath:    self,
		Args:       []string{"run"},
		StdoutPath: filepath.Join(home, "claudeqd.out.log"),
		StderrPath: filepath.Join(home, "claudeqd.err.log"),
	}
	plist, err := launchd.Plist(cfg)
	if err != nil {
		return err
	}

	agentsDir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	agent := launchd.Agent{Runner: system.Real{}, Dir: agentsDir, Label: launchd.DefaultLabel, UID: os.Getuid()}
	if err := agent.Install(context.Background(), plist); err != nil {
		return err
	}

	fmt.Printf("installed LaunchAgent %s (%s)\n", launchd.DefaultLabel, agent2path(agentsDir))
	fmt.Println("claudeqd will now start at login and restart on exit.")
	fmt.Println()
	fmt.Println("To enable wake-from-sleep, pmset must run as root. Add a sudoers entry once:")
	fmt.Printf("  echo '%s ALL=(root) NOPASSWD: /usr/bin/pmset' | sudo tee /etc/sudoers.d/claudeq\n", currentUser())
	return nil
}

func cmdUninstall() error {
	agentsDir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	agent := launchd.Agent{Runner: system.Real{}, Dir: agentsDir, Label: launchd.DefaultLabel, UID: os.Getuid()}
	if err := agent.Uninstall(context.Background()); err != nil {
		return err
	}
	fmt.Printf("removed LaunchAgent %s\n", launchd.DefaultLabel)
	fmt.Println("If you added the pmset sudoers entry, remove it with: sudo rm -f /etc/sudoers.d/claudeq")
	return nil
}

func launchAgentsDir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(h, "Library", "LaunchAgents"), nil
}

func agent2path(dir string) string {
	return filepath.Join(dir, launchd.DefaultLabel+".plist")
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "<your-username>"
}
