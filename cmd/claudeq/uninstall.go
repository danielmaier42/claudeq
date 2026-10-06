package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/danielmaier42/claudeq/internal/system"
	"github.com/danielmaier42/claudeq/internal/uninstall"
)

// uninstaller is the part of uninstall.Machine the command drives. Tests swap
// in a fake so nothing on the real Mac is touched.
type uninstaller interface {
	Plan(ctx context.Context, purge bool) (uninstall.Plan, error)
	Run(ctx context.Context, p uninstall.Plan) error
}

// uninstallEnv is what cmdUninstall needs from the outside world.
type uninstallEnv struct {
	mac     uninstaller
	dataDir string
	uid     int
	in      io.Reader
	out     io.Writer
	// alert reports the outcome in a macOS alert (the --gui mode).
	alert func(title, message string, critical bool) error
}

func cmdUninstall(args []string) error {
	mac, err := uninstall.ThisMac()
	if err != nil {
		return err
	}
	return runUninstall(args, uninstallEnv{
		mac: mac, dataDir: mac.DataDir, uid: os.Getuid(), in: os.Stdin, out: os.Stdout,
		alert: func(title, message string, critical bool) error {
			return uninstall.Alert(context.Background(), system.Real{}, title, message, critical)
		},
	})
}

func runUninstall(args []string, env uninstallEnv) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purge := fs.Bool("purge", false, "also delete tasks, run history, artifacts and settings")
	yes := fs.Bool("yes", false, "do not ask before removing")
	gui := fs.Bool("gui", false, "report the outcome in a macOS alert instead of the terminal (the app's Uninstall button)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	// Under sudo the LaunchAgent, the home folder and the launchd domain would
	// all be root's, and the user's own agent would stay behind.
	if env.uid == 0 {
		return errors.New("run claudeq uninstall as yourself, not with sudo: macOS asks for the password when it is needed")
	}

	ctx := context.Background()
	plan, err := env.mac.Plan(ctx, *purge)
	if err != nil {
		return reportFailure(env, *gui, err)
	}
	if !*gui {
		printPlan(env.out, plan, env.dataDir, *purge)
		if !*yes && !confirm(env.in, env.out) {
			fmt.Fprintln(env.out, "Nothing was removed.")
			return nil
		}
	}

	err = env.mac.Run(ctx, plan)
	switch {
	case errors.Is(err, uninstall.ErrCancelled):
		if !*gui {
			fmt.Fprintln(env.out, "Cancelled. Nothing was removed.")
		}
		return nil
	case err != nil:
		return reportFailure(env, *gui, err)
	}

	kept := "Your tasks, run history and settings are still in " + env.dataDir + ". Delete that folder to remove them too."
	if *purge {
		kept = "Your tasks, run history and settings were deleted too."
	}
	if *gui {
		return env.alert("ClaudeQ was removed", kept, false)
	}
	fmt.Fprintln(env.out, "ClaudeQ was removed. "+kept)
	return nil
}

// reportFailure hands err back to the terminal, or in --gui mode shows it in an
// alert: the app that started the uninstall has no window left to show it in.
func reportFailure(env uninstallEnv, gui bool, err error) error {
	if gui {
		if aerr := env.alert("ClaudeQ could not be removed", err.Error(), true); aerr != nil {
			return errors.Join(err, aerr)
		}
	}
	return err
}

func printPlan(w io.Writer, p uninstall.Plan, dataDir string, purge bool) {
	fmt.Fprintln(w, "This removes ClaudeQ from this Mac:")
	for _, app := range p.Apps {
		fmt.Fprintf(w, "  app               %s\n", app)
	}
	agent := p.Agent
	if agent == "" {
		agent = "(not installed; any running daemon is stopped)"
	}
	fmt.Fprintf(w, "  background agent  %s\n", agent)
	if p.Sudoers != "" {
		fmt.Fprintf(w, "  wake permission   %s\n", p.Sudoers)
	}
	if p.Receipt {
		fmt.Fprintf(w, "  installer receipt %s\n", uninstall.BundleID)
	}
	for _, d := range p.Data {
		fmt.Fprintf(w, "  data              %s\n", d)
	}
	if p.SharedDir != "" {
		fmt.Fprintf(w, "Anything else in %s is kept.\n", p.SharedDir)
	}
	if !purge {
		fmt.Fprintf(w, "Kept: your tasks, run history and settings in %s (add --purge to delete them).\n", dataDir)
	}
	if p.Admin {
		fmt.Fprintln(w, "macOS asks for an administrator password for the parts that belong to the system.")
	}
}

// confirm asks a yes/no question on in; only an explicit yes counts.
func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Remove ClaudeQ? [y/N] ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
