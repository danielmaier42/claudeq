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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/danielmaier42/claudeq/internal/api"
	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/engine"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/fileaccess"
	"github.com/danielmaier42/claudeq/internal/launchd"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/system"
	"github.com/danielmaier42/claudeq/internal/update"
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

	// A fresh process means nothing is actually running: mark any run left as
	// "running" (from a crash/power loss) as interrupted, and prune old history.
	if n, err := st.ReconcileRunningRuns(time.Now()); err == nil && n > 0 {
		fmt.Fprintf(os.Stdout, "claudeqd: reconciled %d interrupted run(s)\n", n)
	}
	if cfg, err := st.LoadConfig(); err == nil {
		_ = st.PruneHistory(cfg.Settings.RunHistoryLimit())
	}

	// Resolve the Claude Code binary. An explicit setting wins; otherwise detect
	// it (the daemon's launchd PATH excludes ~/.local/bin, so a plain lookup at
	// exec time would fail). Per-run, the engine still prefers the live setting.
	claudeBin := ""
	if cfg, err := st.LoadConfig(); err == nil {
		claudeBin = cfg.Settings.ClaudePath
	}
	if claudeBin == "" {
		claudeBin = executor.DetectBinary()
	}
	if claudeBin == "" {
		fmt.Fprintln(os.Stderr, "claudeqd: warning: could not locate the 'claude' binary; set it in Settings")
	} else {
		fmt.Fprintln(os.Stdout, "claudeqd: using claude at", claudeBin)
	}

	c := clock.Real{}
	eng := engine.New(st, limit.New(c), &executor.Executor{
		Bin:      claudeBin,
		Home:     home,
		QueueBin: resolveQueueBin(),
	}, c)
	if !*noWake {
		eng.SetWaker(&wake.Scheduler{Runner: system.Real{}, Sudo: true})
	}
	eng.SetNotifier(buildNotifier(st))
	// Ask for notification permission up front (only does anything when running
	// from the app bundle) so run-outcome notifications carry the app icon.
	notify.RequestMacAuthorization()
	// Provoke the macOS file-access (TCC) consent prompt now, at startup — which
	// happens at install and at every login, while the user is present — rather
	// than mid-run overnight, where the prompt has no one to answer it and the
	// run stalls. Reading each task folder is what makes macOS raise the prompt;
	// once answered the decision sticks and this becomes a no-op. Folders added
	// later are warmed per-task from the API (Deps.WarmFileAccess) at add/edit.
	go warmEnabledTasks(st)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background update checker: polls GitHub for a newer release hourly and
	// caches the result so the dashboard's "update available" banner and the
	// manual "Check for updates" button answer instantly (see internal/update).
	updSvc := update.NewService(update.GitHubFetcher{}, update.DefaultInterval)
	go updSvc.Run(ctx)

	httpSrv := &http.Server{
		Addr: *addr,
		Handler: api.Handler(api.Deps{
			Store: st, Runner: eng, Canceler: eng, Models: api.BinaryModelLister(claudeBinOr(claudeBin)),
			ChooseFolder: api.OSAScriptFolderChooser(system.Real{}), ActiveTasks: eng.ActiveTaskIDs,
			WakeError: eng.WakeError, WarmFileAccess: warmFileAccess, Updates: updSvc,
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

// warmFileAccess reads the given directories so macOS raises its file-access
// consent prompt while the user is present (see call sites for why). It is
// best-effort and bounded: a directory stuck behind an unanswered prompt cannot
// wedge it, and it never blocks the scheduler. If access is already granted (the
// common case) it is a silent no-op. Folders are first collapsed to their macOS
// privacy category (ConsentTargets), so many tasks under ~/Documents provoke the
// Documents prompt once rather than one blocked read each; it then probes every
// remaining target (ProbeAll, not the first-block Probe) so distinct categories
// each get their prompt in one pass. Wired into the API as Deps.WarmFileAccess so
// a folder added at runtime is warmed the moment its task is created or edited.
func warmFileAccess(dirs []string) {
	home, _ := os.UserHomeDir()
	targets := fileaccess.ConsentTargets(dirs, home)
	for _, res := range fileaccess.ProbeAll(targets, fileaccess.DefaultProbeTimeout) {
		switch res.Reason {
		case fileaccess.ReasonTimeout:
			// The read didn't return in time — during warming this almost always
			// means macOS is now showing its consent prompt for the folder, which is
			// exactly what we wanted. Nothing to fix; the user just clicks Allow.
			fmt.Fprintf(os.Stdout, "claudeqd: prompting for file access to %q "+
				"(answer the macOS dialog to allow it)\n", res.BlockedPath)
		default:
			// Genuinely denied — point the user at where they can grant it.
			fmt.Fprintf(os.Stderr, "claudeqd: file access to %q is blocked (%s); allow it in "+
				"System Settings > Privacy & Security > Files and Folders (or Full Disk Access)\n",
				res.BlockedPath, res.Reason)
		}
	}
}

// warmEnabledTasks provokes the prompt for every enabled task's folder at
// startup (install time and each login, while the user is present).
func warmEnabledTasks(st *store.Store) {
	// Let the login/session settle so the prompt actually surfaces rather than
	// being lost in login-time churn.
	time.Sleep(2 * time.Second)
	cfg, err := st.LoadConfig()
	if err != nil {
		return
	}
	dirs := make([]string, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		if t.Enabled {
			dirs = append(dirs, t.WorkingDir)
		}
	}
	warmFileAccess(dirs)
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
	// When running from inside the app bundle, tie the agent to it so Login Items
	// shows the ClaudeQ name and icon instead of a bare "claudeqd".
	if strings.Contains(self, ".app/Contents/MacOS/") {
		cfg.AssociatedBundleID = launchd.DefaultLabel // == the app bundle id
	}
	plist, err := launchd.Plist(cfg)
	if err != nil {
		return err
	}

	agentsDir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	// Stop any stray daemon still holding the loopback port (an orphan from a
	// prior run/version that launchd's bootout won't catch) so the freshly
	// bootstrapped one can bind and actually take over — otherwise an update
	// wouldn't take effect until a reboot.
	killStrayDaemons()

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

// killStrayDaemons terminates any running `claudeqd run` process. The current
// process is `claudeqd install`, so it never matches itself. Best-effort.
func killStrayDaemons() {
	_ = exec.Command("pkill", "-f", "claudeqd run").Run()
	// Give the OS a moment to release the port before the caller re-bootstraps.
	time.Sleep(500 * time.Millisecond)
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

// resolveQueueBin finds the claudeq CLI shipped next to this daemon binary (in
// the app bundle's Contents/MacOS, or the dev build directory). Its absolute
// path is handed to each run as CLAUDEQ_BIN so a task can queue follow-up work
// even though the daemon's launchd PATH does not include it. Empty when it
// cannot be located, in which case runs fall back to a bare "claudeq" lookup.
func resolveQueueBin() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	cand := filepath.Join(filepath.Dir(self), "claudeq")
	if info, err := os.Stat(cand); err == nil && !info.IsDir() {
		return cand
	}
	return ""
}

// claudeBinOr falls back to the bare name so the model lister still has
// something to invoke when detection came up empty.
func claudeBinOr(bin string) string {
	if bin != "" {
		return bin
	}
	return "claude"
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
