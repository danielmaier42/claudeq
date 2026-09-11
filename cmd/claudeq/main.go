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
  claudeq list [--json]
  claudeq show   ID [--json]       (one task in full, prompt included)
  claudeq add    --id ID --prompt P --dir DIR [--name N] [--trigger asap|fixed|cron]
                 [--at RFC3339] [--cron EXPR] [--model M] [--parallel] [--skip-permissions]
                 [--notify] [--quiet-history]
  claudeq edit   ID                (open the whole task in $EDITOR)
  claudeq edit   ID [--name N] [--prompt P | --prompt-file PATH] [--dir DIR]
                 [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR] [--model M]
                 [--parallel=BOOL] [--enabled=BOOL] [--skip-permissions=BOOL]
                 [--notify=BOOL] [--quiet-history=BOOL]  (only the flags you pass are changed)
  claudeq queue  --prompt P [--at RFC3339 | --in DUR | --cron EXPR] [--dir DIR] [--name N]
                 [--model M] [--parallel=BOOL] [--skip-permissions=BOOL] [--notify=BOOL]
                 [--quiet-history=BOOL]
                 (queue a follow-up task; settings you do not pass are inherited from
                 the calling task)
  claudeq publish --file PATH [--title T] [--description D]
                 (publish a file as an artifact; shows up in the Artifacts view)
  claudeq notify --title T --message M [--url U]
                 (send a notification over the configured channels, no artifact)
  claudeq export ID [--out PATH] [--force]
                 (write the task to a shareable .claudeq file; default ./ID.claudeq)
  claudeq import PATH [--id ID]   (add the task from a .claudeq file, settings as-is)
  claudeq rm ID
  claudeq enable ID | claudeq disable ID
  claudeq move   ID INDEX          (0 = highest priority)
  claudeq run-now ID               (run once, now, for testing)
  claudeq status [--all]           (recent runs; unread marked *)
  claudeq read RUNID | claudeq read-all
  claudeq settings [--json] [--default-model M] [--skip-permissions=BOOL]
                   [--claude-path PATH] [--heartbeat-minutes N]
                   [--idle-timeout-minutes N] [--max-run-history N]
                   [--system-prompt S | --system-prompt-file PATH]
                   [--paused=BOOL] [--pushover=BOOL] [--pushover-token T]
                   [--pushover-user U] [--ntfy=BOOL] [--ntfy-server S]
                   [--ntfy-topic T] [--ntfy-token T] [--webhook=BOOL]
                   [--webhook-url U] [--webhook-template J]
                   (--paused=true stops every run globally; nothing starts, not
                   even run-now, until it is set back to false)
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
		return cmdList(st, rest)
	case "show":
		return cmdShow(st, rest)
	case "edit":
		return cmdEdit(st, rest)
	case "add":
		return cmdAdd(st, rest)
	case "queue":
		return cmdQueue(st, rest)
	case "publish":
		return cmdPublish(st, rest)
	case "notify":
		return cmdNotify(st, rest)
	case "export":
		return cmdExport(st, rest)
	case "import":
		return cmdImport(st, rest)
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

func cmdList(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the tasks as JSON, prompts included")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	if *asJSON {
		if cfg.Tasks == nil {
			cfg.Tasks = []task.Task{}
		}
		return printJSON(cfg.Tasks)
	}
	if len(cfg.Tasks) == 0 {
		fmt.Println("no tasks")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "#\tID\tNAME\tTRIGGER\tWHEN\tPARALLEL\tENABLED")
	for i, t := range cfg.Tasks {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%t\t%t\n",
			i, t.ID, listName(t.Name), t.Trigger, triggerWhen(t), t.Parallel, t.Enabled)
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
		s       taskSettings
	)
	s.register(fs, "off")
	if err := fs.Parse(args); err != nil {
		return err
	}

	t := task.Task{
		ID: *id, Name: *name, Prompt: *prompt, WorkingDir: *dir,
		Trigger: task.Trigger(*trig), Cron: *cronArg, Enabled: true,
		Model: s.model, Parallel: s.parallel, NotifyOnResult: s.notify,
		QuietHistory: s.quietHistory, Permissions: task.PermissionsFor(s.skipPerms),
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if *at != "" {
		parsed, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return fmt.Errorf("invalid --at time: %w", err)
		}
		t.FixedAt = parsed
	}
	if err := task.CheckID(t.ID); err != nil {
		return err
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

// queueOpts are the caller-supplied parts of `claudeq queue`. Everything not
// passed (model, permissions, parallel, notify, and the default working dir) is
// inherited from the calling task via the CLAUDEQ_PARENT_TASK environment.
type queueOpts struct {
	prompt string
	at     string // RFC3339 time (--at)
	in     string // Go duration before running (--in)
	cron   string // 5-field cron expression (--cron)
	dir    string // working directory override (--dir)
	name   string // display name (--name)

	// Per-call overrides of the inherited settings, the same flags `add` and
	// `edit` take; set records which of them were passed.
	taskSettings
	set map[string]bool
}

func (o queueOpts) has(name string) bool { return o.set[name] }

// parseQueueOpts parses the `claudeq queue` flags and records which of the
// override flags were passed.
func parseQueueOpts(args []string) (queueOpts, error) {
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	var o queueOpts
	fs.StringVar(&o.prompt, "prompt", "", "prompt sent to Claude Code (required)")
	fs.StringVar(&o.at, "at", "", "RFC3339 time to run at or after")
	fs.StringVar(&o.in, "in", "", "delay before running, e.g. 90m or 2h30m")
	fs.StringVar(&o.cron, "cron", "", "5-field cron expression for a recurring task")
	fs.StringVar(&o.dir, "dir", "", "working directory (default: the calling task's dir)")
	fs.StringVar(&o.name, "name", "", "display name (default: derived from the prompt)")
	o.register(fs, "inherited")
	if err := fs.Parse(args); err != nil {
		return queueOpts{}, err
	}
	var err error
	if o.set, err = passedFlags(fs); err != nil {
		return queueOpts{}, err
	}
	return o, nil
}

// cmdQueue enqueues a follow-up task. It is meant to be run by Claude from
// inside a task (see executor.selfQueueSystemPrompt) but also works standalone.
func cmdQueue(st *store.Store, args []string) error {
	o, err := parseQueueOpts(args)
	if err != nil {
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
// the identity/scheduling fields, applies the queue options and finally any
// per-call overrides. id is the pre-generated task id; now anchors --in.
func buildQueuedTask(parentJSON, id string, o queueOpts, now time.Time) (task.Task, error) {
	var t task.Task
	if parentJSON != "" {
		if err := json.Unmarshal([]byte(parentJSON), &t); err != nil {
			return task.Task{}, fmt.Errorf("parse parent task: %w", err)
		}
	}

	// Keep inherited settings (model, permissions, parallel, notify_on_result and
	// working_dir as the default); reset everything that identifies or schedules.
	// Quiet history is not inherited: it suits a watcher's routine ticks, but a
	// follow-up it queues is real work whose run the operator wants to see. The
	// caller can still ask for it explicitly with --quiet-history below.
	t.ID = id
	t.Prompt = o.prompt
	t.Name = o.name
	t.Enabled = true
	t.FixedAt = time.Time{}
	t.Cron = ""
	t.QuietHistory = false
	if o.dir != "" {
		t.WorkingDir = o.dir
	}
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}

	// Explicit overrides win over inheritance, e.g. a cheap watcher queueing a
	// review that must run on a stronger model with a visible run.
	o.apply(&t, o.has)

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

// cmdPublish publishes a file as an artifact. It is meant to be run by Claude
// from inside a task (see executor.artifactSystemPrompt) but also works
// standalone. Attribution (task/run) comes from the environment the daemon sets.
func cmdPublish(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	file := fs.String("file", "", "path to the file to publish (required)")
	title := fs.String("title", "", "short title (default: the file name)")
	desc := fs.String("description", "", "optional one-line summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*file) == "" {
		return fmt.Errorf("--file is required")
	}

	src := callingRun()
	now := time.Now()
	in := app.PublishInput{
		SourcePath:  *file,
		Title:       *title,
		Description: *desc,
		TaskID:      src.taskID,
		TaskName:    src.taskName,
		RunID:       src.runID,
		Now:         now,
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		in.ID = newArtifactID(now)
		art, err := app.PublishArtifact(st, in)
		if err != nil {
			// An id collision is the only retryable case; anything else (missing
			// file, copy failure) is deterministic, so fail fast.
			if strings.Contains(err.Error(), "already exists") {
				lastErr = err
				continue
			}
			return err
		}
		fmt.Printf("published artifact %q (%s)\n", art.Title, art.ID)
		return nil
	}
	return lastErr
}

// runSource identifies the task and run a CLI call was made from, for
// attributing what it produces (an artifact, a notification).
type runSource struct {
	taskID, taskName, runID string
}

// callingRun reads the attribution the daemon injects into every run's
// environment. Outside a run all fields are empty.
func callingRun() runSource {
	src := runSource{taskID: os.Getenv(executor.EnvTaskID), runID: os.Getenv(executor.EnvRunID)}
	src.taskName = src.taskID
	if parent := os.Getenv(executor.EnvParentTask); parent != "" {
		var pt task.Task
		if err := json.Unmarshal([]byte(parent), &pt); err == nil {
			if pt.ID != "" {
				src.taskID = pt.ID
			}
			if pt.Name != "" {
				src.taskName = pt.Name
			}
		}
	}
	return src
}

// newArtifactID builds a unique-ish artifact id.
func newArtifactID(now time.Time) string { return newID("a-", now) }

// newID builds a unique-ish, time-sortable id; the random suffix disambiguates
// several ids minted within the same second.
func newID(prefix string, now time.Time) string {
	return prefix + now.UTC().Format("20060102T150405") + "-" + shortHex(3)
}

// queueWhen describes when a just-queued task will run, for the CLI confirmation.
func queueWhen(t task.Task) string {
	if t.Trigger == task.TriggerASAP {
		return "as soon as possible"
	}
	return string(t.Trigger) + " " + triggerWhen(t)
}

// nameWidth is how many runes of a name fit on one line: the NAME column of
// `claudeq list` is cut to it (the full name stays in `show` and `--json`),
// and a name derived from a prompt is never longer.
const nameWidth = 40

// listName renders a task name for the `claudeq list` table: on one line
// (a name with a newline or tab in it would break the columns) and cut to
// nameWidth.
func listName(name string) string {
	return truncate(strings.Join(strings.Fields(name), " "), nameWidth)
}

// truncate cuts s to at most width runes, ending in an ellipsis when it had to
// cut anything. A width below one yields the empty string.
func truncate(s string, width int) string {
	if width < 1 {
		return ""
	}
	if len(s) <= width { // bytes >= runes, so this cannot need cutting
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

// defaultQueueName derives a short single-line name from the prompt.
func defaultQueueName(prompt string) string {
	s := strings.Join(strings.Fields(prompt), " ")
	if s == "" {
		return "queued task"
	}
	return truncate(s, nameWidth)
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
	return cmdList(st, nil)
}

func cmdRunNow(st *store.Store, id string) error {
	c := clock.Real{}
	// Give the manual run the same self-queue context the daemon provides, so a
	// task tested with run-now can queue follow-up work too. This process is the
	// claudeq CLI, so its own path is the queue binary.
	self, _ := os.Executable()
	eng := engine.New(st, limit.New(c), &executor.Executor{Home: st.Home(), QueueBin: self}, c)
	fmt.Printf("running task %q now...\n", id)
	started := c.Now()
	if err := eng.RunTaskNow(context.Background(), id); err != nil {
		return err
	}
	return printLatestRun(st, id, started)
}

// printLatestRun reports the task's run that started at or after since — the
// one run-now just made — rather than whatever older run history holds.
func printLatestRun(st *store.Store, taskID string, since time.Time) error {
	runs, err := st.Runs()
	if err != nil {
		return err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].TaskID == taskID && !runs[i].StartedAt.Before(since) {
			r := runs[i]
			fmt.Printf("result: %s (exit %d)\n", r.Status, r.ExitCode)
			if r.Error != "" {
				fmt.Printf("detail: %s\n", r.Error)
			}
			fmt.Printf("log:    %s\n", r.LogPath)
			return nil
		}
	}
	// A quiet-history task records only runs that need attention, so finding
	// nothing is the expected outcome there, not a missing record.
	if t, err := findTask(st, taskID); err == nil && t.QuietHistory {
		fmt.Println("no run recorded (quiet history: only failures are kept)")
		return nil
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
