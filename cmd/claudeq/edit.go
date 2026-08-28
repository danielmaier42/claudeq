// Inspecting and editing existing tasks: `claudeq show` prints a task in full
// (including its whole prompt), `claudeq edit` changes it — either field by
// field with flags, or interactively as TOML in $EDITOR.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// splitTaskID takes the task id off the front of a command's arguments; the
// rest are flags. Keeping the id positional and first is what makes
// `claudeq edit ID --prompt-file P` unambiguous.
func splitTaskID(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("expected a task id")
	}
	if strings.HasPrefix(args[0], "-") {
		return "", nil, fmt.Errorf("expected a task id before the flags, got %q", args[0])
	}
	return args[0], args[1:], nil
}

func findTask(st *store.Store, id string) (task.Task, error) {
	cfg, err := st.LoadConfig()
	if err != nil {
		return task.Task{}, err
	}
	for _, t := range cfg.Tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return task.Task{}, fmt.Errorf("task %q not found", id)
}

func cmdShow(st *store.Store, args []string) error {
	id, rest, err := splitTaskID(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the task as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	t, err := findTask(st, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(t)
	}
	printTask(t)
	return nil
}

func printTask(t task.Task) {
	model := t.Model
	if model == "" {
		model = "(global default)"
	}
	fmt.Printf("id:                %s\n", t.ID)
	fmt.Printf("name:              %s\n", t.Name)
	fmt.Printf("enabled:           %t\n", t.Enabled)
	fmt.Printf("trigger:           %s\n", strings.TrimSpace(string(t.Trigger)+" "+triggerWhen(t)))
	fmt.Printf("working_dir:       %s\n", t.WorkingDir)
	fmt.Printf("parallel:          %t\n", t.Parallel)
	fmt.Printf("model:             %s\n", model)
	fmt.Printf("permissions:       %s\n", t.Permissions)
	fmt.Printf("notify_on_result:  %t\n", t.NotifyOnResult)
	fmt.Printf("\nprompt:\n%s\n", t.Prompt)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// cmdEdit changes an existing task. With flags it patches the named fields;
// with no flags it opens the whole task as TOML in $EDITOR.
func cmdEdit(st *store.Store, args []string) error {
	id, rest, err := splitTaskID(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return editTaskInEditor(st, id)
	}

	patch, err := parseTaskPatch(rest, readPromptFile)
	if err != nil {
		return err
	}
	if err := app.EditTask(st, id, func(t *task.Task) error {
		edited, err := patch.apply(*t)
		if err != nil {
			return err
		}
		if err := edited.Validate(); err != nil {
			return err
		}
		*t = edited
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("updated task %q\n", id)
	return nil
}

// taskPatch is a parsed set of `claudeq edit` flags: the values plus which of
// them the caller actually passed. Only the passed ones are applied, so editing
// the prompt never resets the schedule.
type taskPatch struct {
	set            map[string]bool
	name           string
	prompt         string
	dir            string
	trigger        string
	at             string
	cron           string
	model          string
	parallel       bool
	enabled        bool
	skipPermission bool
	notify         bool
}

func (p taskPatch) has(name string) bool { return p.set[name] }

// parseTaskPatch parses the edit flags. readFile resolves --prompt-file; it is
// injected so the parsing stays testable without touching the filesystem.
func parseTaskPatch(args []string, readFile func(string) ([]byte, error)) (taskPatch, error) {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	var p taskPatch
	promptFile := fs.String("prompt-file", "", "read the prompt from a file ('-' = stdin)")
	fs.StringVar(&p.name, "name", "", "display name")
	fs.StringVar(&p.prompt, "prompt", "", "prompt sent to Claude Code")
	fs.StringVar(&p.dir, "dir", "", "working directory")
	fs.StringVar(&p.trigger, "trigger", "", "asap|fixed|cron")
	fs.StringVar(&p.at, "at", "", "RFC3339 start time (implies --trigger fixed)")
	fs.StringVar(&p.cron, "cron", "", "crontab expression (implies --trigger cron)")
	fs.StringVar(&p.model, "model", "", "model override (empty = global default)")
	fs.BoolVar(&p.parallel, "parallel", false, "allow running alongside other parallel tasks")
	fs.BoolVar(&p.enabled, "enabled", false, "enable or pause the task")
	fs.BoolVar(&p.skipPermission, "skip-permissions", false, "bypass permission prompts")
	fs.BoolVar(&p.notify, "notify", false, "notify on the run's result, not just failures")
	if err := fs.Parse(args); err != nil {
		return taskPatch{}, err
	}
	if fs.NArg() > 0 {
		return taskPatch{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	p.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { p.set[f.Name] = true })
	if len(p.set) == 0 {
		return taskPatch{}, fmt.Errorf("no changes given")
	}
	if p.has("prompt") && p.has("prompt-file") {
		return taskPatch{}, fmt.Errorf("choose either --prompt or --prompt-file")
	}
	if p.has("prompt-file") {
		data, err := readFile(*promptFile)
		if err != nil {
			return taskPatch{}, fmt.Errorf("read prompt file: %w", err)
		}
		p.prompt = string(data)
		p.set["prompt"] = true
	}
	return p, nil
}

// apply layers the patch onto a task. The trigger is derived from --trigger, or
// implied by --at / --cron; when it changes, the timing fields that no longer
// apply are cleared so the stored task stays consistent.
func (p taskPatch) apply(t task.Task) (task.Task, error) {
	if p.has("name") {
		t.Name = p.name
	}
	if p.has("prompt") {
		t.Prompt = p.prompt
	}
	if p.has("dir") {
		t.WorkingDir = p.dir
	}
	if p.has("model") {
		t.Model = p.model
	}
	if p.has("parallel") {
		t.Parallel = p.parallel
	}
	if p.has("enabled") {
		t.Enabled = p.enabled
	}
	if p.has("notify") {
		t.NotifyOnResult = p.notify
	}
	if p.has("skip-permissions") {
		t.Permissions = task.PermissionsDefault
		if p.skipPermission {
			t.Permissions = task.PermissionsSkip
		}
	}

	trigger := t.Trigger
	switch {
	case p.has("trigger"):
		trigger = task.Trigger(p.trigger)
	case p.has("at"):
		trigger = task.TriggerFixed
	case p.has("cron"):
		trigger = task.TriggerCron
	}
	if trigger != t.Trigger {
		if trigger != task.TriggerFixed {
			t.FixedAt = time.Time{}
		}
		if trigger != task.TriggerCron {
			t.Cron = ""
		}
		t.Trigger = trigger
	}

	if p.has("at") {
		parsed, err := time.Parse(time.RFC3339, p.at)
		if err != nil {
			return task.Task{}, fmt.Errorf("invalid --at time (want RFC3339): %w", err)
		}
		t.FixedAt = parsed
	}
	if p.has("cron") {
		t.Cron = p.cron
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	return t, nil
}

// readPromptFile reads a prompt from a file, or from stdin for "-".
func readPromptFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// taskDoc is the editable TOML view of a task. Every setting is always present,
// and the prompt comes last as a multi-line string so a long brief stays
// readable. It mirrors task.Task rather than reusing it, which keeps the on-disk
// config.toml format untouched and lets fixed_at be an empty-able RFC3339
// string.
type taskDoc struct {
	ID             string `toml:"id"`
	Name           string `toml:"name"`
	Enabled        bool   `toml:"enabled"`
	WorkingDir     string `toml:"working_dir"`
	Trigger        string `toml:"trigger"`
	FixedAt        string `toml:"fixed_at"`
	Cron           string `toml:"cron"`
	Parallel       bool   `toml:"parallel"`
	Model          string `toml:"model"`
	Permissions    string `toml:"permissions"`
	NotifyOnResult bool   `toml:"notify_on_result"`
	Prompt         string `toml:"prompt,multiline"`
}

const taskDocHeader = `# claudeq task — edit, save, and close this file to apply.
# Leaving it unchanged (or emptying it) cancels the edit.
#
#   id                 read-only; changing it is rejected
#   trigger            asap | fixed | cron
#   fixed_at           RFC3339 start time, for trigger = "fixed"
#   cron               5-field crontab expression, for trigger = "cron"
#   model              empty = the global default model
#   permissions        default | skip  (skip bypasses permission prompts)
`

func encodeTaskDoc(t task.Task) ([]byte, error) {
	d := taskDoc{
		ID: t.ID, Name: t.Name, Enabled: t.Enabled, WorkingDir: t.WorkingDir,
		Trigger: string(t.Trigger), Cron: t.Cron, Parallel: t.Parallel,
		Model: t.Model, Permissions: string(t.Permissions),
		NotifyOnResult: t.NotifyOnResult, Prompt: t.Prompt,
	}
	if !t.FixedAt.IsZero() {
		d.FixedAt = t.FixedAt.Local().Format(time.RFC3339)
	}
	body, err := toml.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("encode task: %w", err)
	}
	return append([]byte(taskDocHeader+"\n"), body...), nil
}

// decodeTaskDoc parses an edited task document back into a task. orig supplies
// the identity the result is checked against — a renamed id is rejected rather
// than silently creating or overwriting another task.
func decodeTaskDoc(data []byte, orig task.Task) (task.Task, error) {
	var d taskDoc
	if err := toml.Unmarshal(data, &d); err != nil {
		return task.Task{}, fmt.Errorf("parse task: %w", err)
	}
	if d.ID == "" {
		return task.Task{}, fmt.Errorf("the document has no id line (expected id = %q)", orig.ID)
	}
	if d.ID != orig.ID {
		return task.Task{}, fmt.Errorf("task id cannot be changed (%q -> %q)", orig.ID, d.ID)
	}
	t := task.Task{
		ID: d.ID, Name: d.Name, Prompt: d.Prompt, WorkingDir: d.WorkingDir,
		Trigger: task.Trigger(d.Trigger), Cron: d.Cron, Parallel: d.Parallel,
		Enabled: d.Enabled, Model: d.Model,
		Permissions: task.Permissions(d.Permissions), NotifyOnResult: d.NotifyOnResult,
	}
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if strings.TrimSpace(d.FixedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(d.FixedAt))
		if err != nil {
			return task.Task{}, fmt.Errorf("invalid fixed_at (want RFC3339): %w", err)
		}
		t.FixedAt = parsed
	}
	if err := t.Validate(); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

func editTaskInEditor(st *store.Store, id string) error {
	if !isInteractiveTerminal() {
		return fmt.Errorf("no terminal to open an editor in; pass flags instead (claudeq edit %s --help)", id)
	}
	orig, err := findTask(st, id)
	if err != nil {
		return err
	}
	before, err := encodeTaskDoc(orig)
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "claudeq-edit")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	path := filepath.Join(dir, "task-"+id+".toml")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	editorErr := runEditor(path)

	after, err := os.ReadFile(path)
	if err != nil {
		if editorErr != nil {
			return editorErr
		}
		return fmt.Errorf("read edited file: %w", err)
	}
	if editorErr != nil {
		// The editor may well have saved before it failed, so hand back the
		// draft rather than discarding work the user already typed.
		if !bytes.Equal(before, after) {
			keep = true
			return fmt.Errorf("%w\nyour edits are kept at %s", editorErr, path)
		}
		return editorErr
	}
	if bytes.Equal(before, after) || len(bytes.TrimSpace(after)) == 0 {
		fmt.Println("no changes")
		return nil
	}
	if err := applyEditedDoc(st, orig, before, after); err != nil {
		keep = true // never throw away what the user just typed
		return fmt.Errorf("%w\nyour edits are kept at %s", err, path)
	}
	fmt.Printf("updated task %q\n", id)
	return nil
}

// applyEditedDoc stores an edited document. before is what the editor was
// handed; if the stored task no longer matches it, someone else (the app, the
// daemon, another CLI call) changed the task while the editor was open, and
// writing would silently drop their change — so the edit is refused instead.
func applyEditedDoc(st *store.Store, orig task.Task, before, after []byte) error {
	edited, err := decodeTaskDoc(after, orig)
	if err != nil {
		return err
	}
	return app.EditTask(st, orig.ID, func(t *task.Task) error {
		current, err := encodeTaskDoc(*t)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, before) {
			return fmt.Errorf("the task changed while your editor was open; re-run the edit")
		}
		*t = edited
		return nil
	})
}

// runEditor opens path in the user's editor and waits for it to close. The
// command goes through the shell, the way git does it, so $EDITOR may carry
// arguments ("code -w") as well as a path with spaces; the file is passed as an
// argument rather than interpolated, so its name is never re-parsed.
func runEditor(path string) error {
	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi")
	if editor == "" {
		return fmt.Errorf("no editor configured; set $EDITOR")
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", path) // #nosec G204 — the user's own $EDITOR
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q: %w", editor, err)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// isInteractiveTerminal reports whether there is a terminal to hand to an
// editor. Both ends must be one: when claudeq is driven by a script, an agent,
// or the daemon, stdin or stdout is a pipe or socket, and opening an editor
// there would hang or garble the caller's output.
func isInteractiveTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
