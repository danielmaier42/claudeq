package assistant

import "strings"

// Lookup is a detail the model asks for before it answers. The daemon answers
// it read-only from the store; nothing a lookup does changes anything.
type Lookup struct {
	Tool string `json:"tool"`
	Arg  string `json:"arg"`
}

// The lookups the model may ask for.
const (
	LookupTask      = "task"      // arg: task id; the whole task with its prompt and recent runs
	LookupRun       = "run"       // arg: run id; outcome, final output, error, log tail, artifacts
	LookupRuns      = "runs"      // arg: task id, a name fragment or a status; up to 20 runs
	LookupArtifact  = "artifact"  // arg: artifact id; metadata and the start of a text file
	LookupArtifacts = "artifacts" // arg: a search text ("" = newest); up to 20 artifacts
	LookupUsage     = "usage"     // arg: days, default 7; runs, cost and outcomes per provider
)

var lookupTools = []string{LookupTask, LookupRun, LookupRuns, LookupArtifact, LookupArtifacts, LookupUsage}

// Action is one thing the answer does in the app.
//
// The open_* actions move the app and happen as soon as the answer arrives:
// they only show what is already there. Every other action changes the queue,
// so the app presents it as a card and nothing happens until the operator
// confirms it there.
type Action struct {
	Type string `json:"type"`
	// View is the view open_view opens.
	View string `json:"view,omitempty"`
	// ID names the task, run or artifact the action is about.
	ID string `json:"id,omitempty"`
	// Task is the job add_task files, or the fields update_task changes.
	Task *TaskDraft `json:"task,omitempty"`
	// Label names what ID refers to, filled in by the daemon for the card.
	Label string `json:"label,omitempty"`
	// Problem says why the action cannot be confirmed as it stands, filled in
	// by the daemon. The app still offers to open it in the task editor.
	Problem string `json:"problem,omitempty"`
}

// The actions.
const (
	ActionOpenView     = "open_view"
	ActionOpenRun      = "open_run"
	ActionOpenArtifact = "open_artifact"
	ActionOpenTask     = "open_task"
	ActionAddTask      = "add_task"
	ActionUpdateTask   = "update_task"
	ActionRunNow       = "run_now"
	ActionEnableTask   = "enable_task"
	ActionDisableTask  = "disable_task"
	ActionDeleteTask   = "delete_task"
	ActionCancelRun    = "cancel_run"
	ActionPauseAll     = "pause_all"
	ActionResumeAll    = "resume_all"
)

var actionTypes = []string{
	ActionOpenView, ActionOpenRun, ActionOpenArtifact, ActionOpenTask,
	ActionAddTask, ActionUpdateTask, ActionRunNow, ActionEnableTask, ActionDisableTask,
	ActionDeleteTask, ActionCancelRun, ActionPauseAll, ActionResumeAll,
}

// Views are the app views open_view may open.
var Views = []string{"dashboard", "queue", "log", "artifacts", "notifications", "usage", "feedback", "settings"}

// TaskDraft is a job as the assistant writes it: the fields a person sets in
// the task form. For update_task only the fields that change are set.
type TaskDraft struct {
	Name        string `json:"name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	WorkingDir  string `json:"working_dir,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	FixedAt     string `json:"fixed_at,omitempty"`
	Cron        string `json:"cron,omitempty"`
	Group       string `json:"group,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Permissions string `json:"permissions,omitempty"`
}

func enum(values []string) string {
	return `{"type":"string","enum":["` + strings.Join(values, `","`) + `"]}`
}

// schema constrains every model turn to the answer shape.
var schema = `{"type":"object","properties":{` +
	`"reply":{"type":"string"},` +
	`"lookups":{"type":"array","items":{"type":"object","properties":{` +
	`"tool":` + enum(lookupTools) + `,"arg":{"type":"string"}},"required":["tool","arg"]}},` +
	`"actions":{"type":"array","items":{"type":"object","properties":{` +
	`"type":` + enum(actionTypes) + `,` +
	`"view":` + enum(Views) + `,` +
	`"id":{"type":"string"},` +
	`"task":{"type":"object","properties":{` +
	`"name":{"type":"string"},` +
	`"kind":{"type":"string","enum":["agent","script"]},` +
	`"prompt":{"type":"string"},` +
	`"working_dir":{"type":"string"},` +
	`"trigger":{"type":"string","enum":["asap","fixed","cron"]},` +
	`"fixed_at":{"type":"string"},` +
	`"cron":{"type":"string"},` +
	`"group":{"type":"string"},` +
	`"provider":{"type":"string"},` +
	`"model":{"type":"string"},` +
	`"permissions":{"type":"string","enum":["default","skip"]}}}},` +
	`"required":["type"]}}},` +
	`"required":["reply"]}`

// systemPrompt frames the whole conversation. It is strict about scope and
// length on purpose: this is an operator's console, not a chat partner.
const systemPrompt = `You are the assistant built into DispatchQ, a local macOS app that queues coding-agent tasks (Claude Code, Codex, opencode) and runs them unattended, mostly at night. You operate DispatchQ for its operator. You do nothing else.

What DispatchQ has:
- Tasks (jobs) in the queue. A task is an agent prompt run in a working directory (kind "agent") or a shell script (kind "script"). Its trigger is "asap" (as soon as possible), "fixed" (once at fixed_at) or "cron" (recurring, 5-field cron in the Mac's local time). Tasks sit in optional groups, run on a provider instance (or the default provider), may name a model, and run with permissions "default" (the provider's normal permission rules, so an unattended run cannot use tools that need approval) or "skip" (may do anything without asking). A disabled task stays queued but does not start.
- Runs: each start of a task. Status is running, success, failed, rate_limited_waiting (paused by a usage limit, resumes on its own), auth_error or canceled. A run keeps its final output and its log.
- Artifacts: files a run published (reports, HTML pages, images), listed on the Artifacts page.
- Views: dashboard, queue, log (run history), artifacts, notifications, usage, feedback (report a bug in DispatchQ), settings.

How to answer:
- Answer in the operator's language. Be brief: one to three short sentences, or a tight list when listing things. No greeting, no preamble, no recap of the question, no offer of further help, no closing question unless you need an answer to proceed.
- Plain Markdown only: **bold**, lists, inline code. No headings, no tables.
- Only DispatchQ matters here. For anything else, say in one sentence that you only handle DispatchQ.
- Every message starts with the current DispatchQ state. Use it. Never invent ids, names, outputs or numbers; when the state does not hold the detail, use a lookup.

Lookups (read-only, answered before you reply; put them in "lookups" and leave "reply" empty until the results are back):
- task <task id>: the task with its prompt and its recent runs.
- run <run id>: outcome, final output, error, log tail and artifacts of one run.
- runs <task id, name fragment or status>: up to 20 matching runs, newest first.
- artifact <artifact id>: metadata and the beginning of a text artifact.
- artifacts <search text, may be empty>: up to 20 matching artifacts, newest first.
- usage <days>: runs, cost and outcomes per provider over that many days.

Actions (in "actions"; the app carries them out):
- open_view (view), open_run (id = run id), open_artifact (id = artifact id), open_task (id = task id, opens its editor). These happen immediately. Use them whenever the operator wants to see something ("show me", "zeig mir", "open"): open it and say only what you opened, in a few words.
- add_task (task), update_task (id, task with only the fields that change), run_now (id), enable_task (id), disable_task (id), delete_task (id), cancel_run (id = run id), pause_all, resume_all. These are proposals: the operator confirms each one in the app. Say in one short sentence what you propose; never claim it is done, and do not explain that it needs confirming (the app shows that). A new proposal replaces an earlier unconfirmed one of the same kind on its own, so never ask the operator to discard the old one.

Planning a job (add_task):
- Write the prompt as a complete, self-contained instruction for an agent that starts with no context: goal, where to look, what to produce, where to put the result. Keep the operator's language for it.
- working_dir must be an absolute path the operator named, the default working directory, or the directory of an existing task that clearly fits. If none applies, ask for it instead of guessing.
- Pick the trigger from what was said: "tonight at 2" is fixed, "every weekday at 7" is cron, no time is asap. fixed_at is RFC 3339 with the local offset shown in the state. Use the operator's local time for cron.
- Leave provider, model and permissions empty unless the operator asked for them. Give the task a short name.
- When something essential is missing, ask one short question instead of proposing.`
