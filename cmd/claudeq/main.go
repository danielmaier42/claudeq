// Command claudeq is the control CLI for the claudeq task queue: it manages
// tasks, triggers a manual test run, and shows run status/history. It shares
// the on-disk store with the claudeqd daemon (PLAN.md D11).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/engine"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
	"github.com/danielmaier42/claudeq/internal/version"
)

const usage = `claudeq - control the Claude Code task queue

Usage:
  claudeq list
  claudeq add    --id ID --prompt P --dir DIR [--name N] [--trigger asap|fixed|cron]
                 [--at RFC3339] [--cron EXPR] [--model M] [--parallel] [--skip-permissions]
  claudeq queue  --prompt P [--at RFC3339 | --in DUR | --cron EXPR] [--dir DIR] [--name N]
                 (queue a follow-up task; inherits the calling task's settings)
  claudeq rm ID
  claudeq enable ID | claudeq disable ID
  claudeq move   ID INDEX          (0 = highest priority)
  claudeq run-now ID               (run once, now, for testing)
  claudeq status [--all]           (recent runs; unread marked *)
  claudeq read RUNID | claudeq read-all
  claudeq settings [--default-model M] [--skip-permissions=BOOL]
                   [--pushover-token T] [--pushover-user U]
  claudeq --version`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claudeq:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Println(version.String())
		return nil
	}

	st, err := openStore()
	if err != nil {
		return err
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		return cmdList(st)
	case "add":
		return cmdAdd(st, rest)
	case "queue":
		return cmdQueue(st, rest)
	case "rm":
		return withID(rest, func(id string) error { return app.RemoveTask(st, id) })
	case "enable":
		return withID(rest, func(id string) error { return app.SetEnabled(st, id, true) })
	case "disable":
		return withID(rest, func(id string) error { return app.SetEnabled(st, id, false) })
	case "move":
		return cmdMove(st, rest)
	case "run-now":
		return withID(rest, func(id string) error { return cmdRunNow(st, id) })
	case "status":
		return cmdStatus(st, rest)
	case "read":
		return withID(rest, func(id string) error { return app.MarkRead(st, id) })
	case "read-all":
		return app.MarkAllRead(st)
	case "settings":
		return cmdSettings(st, rest)
	default:
		fmt.Println(usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func openStore() (*store.Store, error) {
	home, err := store.DefaultHome()
	if err != nil {
		return nil, err
	}
	return store.Open(home)
}

func withID(args []string, fn func(string) error) error {
	if len(args) != 1 {
		return fmt.Errorf("expected exactly one task/run id")
	}
	return fn(args[0])
}

func cmdList(st *store.Store) error {
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Tasks) == 0 {
		fmt.Println("no tasks")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "#\tID\tNAME\tTRIGGER\tWHEN\tPARALLEL\tENABLED")
	for i, t := range cfg.Tasks {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%t\t%t\n",
			i, t.ID, t.Name, t.Trigger, triggerWhen(t), t.Parallel, t.Enabled)
	}
	return w.Flush()
}

func triggerWhen(t task.Task) string {
	switch t.Trigger {
	case task.TriggerFixed:
		return t.FixedAt.Local().Format(time.RFC3339)
	case task.TriggerCron:
		return t.Cron
	default:
		return "-"
	}
}

func cmdAdd(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	var (
		id      = fs.String("id", "", "unique task id (required)")
		name    = fs.String("name", "", "display name (defaults to id)")
		prompt  = fs.String("prompt", "", "prompt sent to Claude Code (required)")
		dir     = fs.String("dir", "", "working directory (required)")
		trig    = fs.String("trigger", "asap", "asap|fixed|cron")
		at      = fs.String("at", "", "RFC3339 time for --trigger fixed")
		cronArg = fs.String("cron", "", "crontab expression for --trigger cron")
		model   = fs.String("model", "", "model override (default: global)")
		par     = fs.Bool("parallel", false, "allow running alongside other parallel tasks")
		skip    = fs.Bool("skip-permissions", false, "bypass permission prompts for this task")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	t := task.Task{
		ID: *id, Name: *name, Prompt: *prompt, WorkingDir: *dir,
		Trigger: task.Trigger(*trig), Cron: *cronArg, Model: *model,
		Parallel: *par, Enabled: true,
		Permissions: task.PermissionsDefault,
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if *skip {
		t.Permissions = task.PermissionsSkip
	}
	if *at != "" {
		parsed, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return fmt.Errorf("invalid --at time: %w", err)
		}
		t.FixedAt = parsed
	}
	if err := t.Validate(); err != nil {
		return err
	}
	if err := app.AddTask(st, t); err != nil {
		return err
	}
	fmt.Printf("added task %q\n", t.ID)
	return nil
}

// queueOpts are the caller-supplied parts of `claudeq queue`. Everything else
// (model, permissions, parallel, notify, and the default working dir) is
// inherited from the calling task via the CLAUDEQ_PARENT_TASK environment.
type queueOpts struct {
	prompt string
	at     string // RFC3339 time (--at)
	in     string // Go duration before running (--in)
	cron   string // 5-field cron expression (--cron)
	dir    string // working directory override (--dir)
	name   string // display name (--name)
}

// cmdQueue enqueues a follow-up task. It is meant to be run by Claude from
// inside a task (see executor.selfQueueSystemPrompt) but also works standalone.
func cmdQueue(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	var o queueOpts
	fs.StringVar(&o.prompt, "prompt", "", "prompt sent to Claude Code (required)")
	fs.StringVar(&o.at, "at", "", "RFC3339 time to run at or after")
	fs.StringVar(&o.in, "in", "", "delay before running, e.g. 90m or 2h30m")
	fs.StringVar(&o.cron, "cron", "", "5-field cron expression for a recurring task")
	fs.StringVar(&o.dir, "dir", "", "working directory (default: the calling task's dir)")
	fs.StringVar(&o.name, "name", "", "display name (default: derived from the prompt)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	parentJSON := os.Getenv(executor.EnvParentTask)
	now := time.Now()

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		t, err := buildQueuedTask(parentJSON, newQueueID(now), o, now)
		if err != nil {
			return err // deterministic (independent of the id) — do not retry
		}
		if err := app.AddTask(st, t); err != nil {
			lastErr = err // almost certainly an id collision; regenerate and retry
			continue
		}
		fmt.Printf("queued task %q (%s)\n", t.ID, queueWhen(t))
		return nil
	}
	return lastErr
}

// buildQueuedTask assembles the task to enqueue. It starts from the calling task
// (parentJSON, empty when run standalone) so settings are inherited, then resets
// the identity/scheduling fields and applies the queue options. id is the
// pre-generated task id; now anchors --in.
func buildQueuedTask(parentJSON, id string, o queueOpts, now time.Time) (task.Task, error) {
	var t task.Task
	if parentJSON != "" {
		if err := json.Unmarshal([]byte(parentJSON), &t); err != nil {
			return task.Task{}, fmt.Errorf("parse parent task: %w", err)
		}
	}

	// Keep inherited settings (model, permissions, parallel, notify_on_result and
	// working_dir as the default); reset everything that identifies or schedules.
	t.ID = id
	t.Prompt = o.prompt
	t.Name = o.name
	t.Enabled = true
	t.FixedAt = time.Time{}
	t.Cron = ""
	if o.dir != "" {
		t.WorkingDir = o.dir
	}
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}

	set := 0
	for _, v := range []string{o.at, o.in, o.cron} {
		if v != "" {
			set++
		}
	}
	if set > 1 {
		return task.Task{}, fmt.Errorf("choose at most one of --at, --in, --cron")
	}

	switch {
	case o.at != "":
		parsed, err := time.Parse(time.RFC3339, o.at)
		if err != nil {
			return task.Task{}, fmt.Errorf("invalid --at time (want RFC3339): %w", err)
		}
		t.Trigger = task.TriggerFixed
		t.FixedAt = parsed
	case o.in != "":
		d, err := time.ParseDuration(o.in)
		if err != nil {
			return task.Task{}, fmt.Errorf("invalid --in duration: %w", err)
		}
		if d < 0 {
			return task.Task{}, fmt.Errorf("--in duration must not be negative")
		}
		t.Trigger = task.TriggerFixed
		t.FixedAt = now.Add(d)
	case o.cron != "":
		t.Trigger = task.TriggerCron
		t.Cron = o.cron
	default:
		t.Trigger = task.TriggerASAP
	}

	if t.Name == "" {
		t.Name = defaultQueueName(o.prompt)
	}
	if err := t.Validate(); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

// queueWhen describes when a just-queued task will run, for the CLI confirmation.
func queueWhen(t task.Task) string {
	if t.Trigger == task.TriggerASAP {
		return "as soon as possible"
	}
	return string(t.Trigger) + " " + triggerWhen(t)
}

// defaultQueueName derives a short single-line name from the prompt.
func defaultQueueName(prompt string) string {
	s := strings.TrimSpace(strings.Join(strings.Fields(prompt), " "))
	if s == "" {
		return "queued task"
	}
	if r := []rune(s); len(r) > 40 {
		return string(r[:39]) + "…"
	}
	return s
}

// newQueueID builds a unique-ish task id; the random suffix disambiguates
// several tasks queued within the same second.
func newQueueID(now time.Time) string {
	return "q-" + now.UTC().Format("20060102T150405") + "-" + shortHex(3)
}

func shortHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

func cmdMove(st *store.Store, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: claudeq move ID INDEX")
	}
	to, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid index %q", args[1])
	}
	if err := app.Move(st, args[0], to); err != nil {
		return err
	}
	return cmdList(st)
}

func cmdRunNow(st *store.Store, id string) error {
	c := clock.Real{}
	// Give the manual run the same self-queue context the daemon provides, so a
	// task tested with run-now can queue follow-up work too. This process is the
	// claudeq CLI, so its own path is the queue binary.
	self, _ := os.Executable()
	eng := engine.New(st, limit.New(c), &executor.Executor{Home: st.Home(), QueueBin: self}, c)
	fmt.Printf("running task %q now...\n", id)
	if err := eng.RunTaskNow(context.Background(), id); err != nil {
		return err
	}
	return printLatestRun(st, id)
}

func printLatestRun(st *store.Store, taskID string) error {
	runs, err := st.Runs()
	if err != nil {
		return err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].TaskID == taskID {
			r := runs[i]
			fmt.Printf("result: %s (exit %d)\n", r.Status, r.ExitCode)
			if r.Error != "" {
				fmt.Printf("detail: %s\n", r.Error)
			}
			fmt.Printf("log:    %s\n", r.LogPath)
			return nil
		}
	}
	fmt.Println("no run recorded")
	return nil
}

func cmdStatus(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	all := fs.Bool("all", false, "show all runs (default: last 20)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runs, err := st.Runs()
	if err != nil {
		return err
	}
	state, err := st.LoadState()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("no runs yet")
		return nil
	}
	if !*all && len(runs) > 20 {
		runs = runs[len(runs)-20:]
	}

	unread := 0
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "\tRUN\tTASK\tSTATUS\tSTARTED")
	for _, r := range runs {
		mark := " "
		if !state.IsRead(r.RunID) {
			mark = "*"
			unread++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			mark, r.RunID, r.TaskName, r.Status, r.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d unread\n", unread)
	return nil
}

func cmdSettings(st *store.Store, args []string) error {
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	model := fs.String("default-model", "", "global default model")
	skip := fs.Bool("skip-permissions", false, "global skip-permissions default")
	poToken := fs.String("pushover-token", "", "Pushover API token")
	poUser := fs.String("pushover-user", "", "Pushover user key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	changed := false
	fs.Visit(func(f *flag.Flag) {
		changed = true
		switch f.Name {
		case "default-model":
			cfg.Settings.DefaultModel = *model
		case "skip-permissions":
			cfg.Settings.SkipPermissionsDefault = *skip
		case "pushover-token":
			cfg.Settings.Pushover.Token = *poToken
		case "pushover-user":
			cfg.Settings.Pushover.UserKey = *poUser
		}
	})

	if changed {
		if err := st.SaveConfig(cfg); err != nil {
			return err
		}
	}

	fmt.Printf("default_model:            %q\n", cfg.Settings.DefaultModel)
	fmt.Printf("skip_permissions_default: %t\n", cfg.Settings.SkipPermissionsDefault)
	fmt.Printf("pushover configured:      %t\n", cfg.Settings.Pushover.Token != "" && cfg.Settings.Pushover.UserKey != "")
	return nil
}
