import type {BadgeTone} from '@/components/ui/badge'
import {baseName, inDateRange} from '@/lib/format'
import {matchesWords} from '@/lib/search'
import type {Run, RunStatus} from '@/lib/types'

// Pure logic of the Log view: how a run is labelled, what the filters let
// through, what the menus offer and how runs of one workflow are grouped.

// A rate-limited run reads as "rescheduled" while its session is still queued
// to continue; once that plan is gone (resumed, canceled, task deleted) the
// pause is just what happened to this run.
export function statusLabel(r: Pick<Run, 'status' | 'resume_pending'>): string {
  if (r.status === 'rate_limited_waiting') return r.resume_pending ? 'rescheduled' : 'rate limited'
  return r.status.replace(/_/g, ' ')
}

export const statusTone: Record<RunStatus, BadgeTone> = {
  running: 'accent',
  success: 'success',
  failed: 'danger',
  auth_error: 'danger',
  rate_limited_waiting: 'warning',
  canceled: 'neutral',
}

// The outcome as a menu entry: one entry per status, so "rescheduled" and
// "rate limited" share theirs.
const statusMenuLabel: Record<RunStatus, string> = {
  running: 'Running',
  success: 'Success',
  failed: 'Failed',
  auth_error: 'Auth error',
  rate_limited_waiting: 'Rate limited',
  canceled: 'Canceled',
}

export type Show = '' | 'running'

export interface LogFilters {
  query: string
  show: Show
  group: string
  task: string
  provider: string
  status: string
  from: string
  to: string
}

export const emptyFilters: LogFilters = {
  query: '',
  show: '',
  group: '',
  task: '',
  provider: '',
  status: '',
  from: '',
  to: '',
}

// NO_GROUP stands for "no group": a real group name never starts with a space.
export const NO_GROUP = ' none'
export const groupKey = (r: Run) => r.task?.group || NO_GROUP
export const providerKey = (r: Run) => r.provider?.id || r.provider?.name || ''

// What the search looks through: the task, its folder, the harness and model,
// the group, the outcome and the error, if any. The outcome is in both of its
// spellings, so "rate limited" also finds a run the row calls "rescheduled".
function haystack(r: Run): string[] {
  return [
    r.task_name,
    r.task?.working_dir,
    r.provider?.name,
    r.provider?.model,
    r.provider?.pool,
    r.task?.group,
    statusLabel(r),
    r.status.replace(/_/g, ' '),
    r.error,
  ].filter((s): s is string => !!s)
}

export const showMatch = (r: Run, show: Show) => (show === 'running' ? r.status === 'running' : true)

export function runMatches(r: Run, f: LogFilters): boolean {
  return (
    inDateRange(r.started_at, f.from, f.to) &&
    showMatch(r, f.show) &&
    (!f.group || groupKey(r) === f.group) &&
    (!f.task || r.task_id === f.task) &&
    (!f.provider || providerKey(r) === f.provider) &&
    (!f.status || r.status === f.status) &&
    matchesWords(f.query, ...haystack(r))
  )
}

export const anyFilter = (f: LogFilters) => Object.values(f).some(Boolean)

export interface MenuOption {
  value: string
  label: string
}

export interface FilterMenus {
  groups: MenuOption[]
  tasks: MenuOption[]
  providers: MenuOption[]
  statuses: MenuOption[]
}

const byLabel = (m: Map<string, string>, noneKey?: string): MenuOption[] =>
  [...m]
    .sort((x, y) => Number(x[0] === noneKey) - Number(y[0] === noneKey) || x[1].localeCompare(y[1]))
    .map(([value, label]) => ({value, label}))

// The menus fill themselves from the runs there are: groups, tasks, providers
// and outcomes that occur. With "Running" they offer only what the
// segment lets through, and the task menu only offers tasks in the chosen group.
export function filterMenus(runs: Run[], f: LogFilters): FilterMenus {
  const groups = new Map<string, string>()
  const tasks = new Map<string, string>()
  const providers = new Map<string, string>()
  const statuses = new Map<string, string>()
  for (const r of runs) {
    if (!showMatch(r, f.show)) continue
    groups.set(groupKey(r), r.task?.group || 'No group')
    if (r.task_id && (!f.group || groupKey(r) === f.group) && !tasks.has(r.task_id))
      tasks.set(r.task_id, r.task_name)
    const pk = providerKey(r)
    if (pk) providers.set(pk, r.provider?.name || pk)
    statuses.set(r.status, statusMenuLabel[r.status] ?? r.status)
  }
  return {
    groups: byLabel(groups, NO_GROUP),
    tasks: byLabel(tasks),
    providers: byLabel(providers),
    statuses: byLabel(statuses),
  }
}

// A choice that no longer matches anything falls back to "all", so the list
// can never be filtered to nothing by a stale selection. Returns only the
// fields to reset.
export function staleChoices(f: LogFilters, m: FilterMenus): Partial<LogFilters> {
  const has = (opts: MenuOption[], v: string) => !v || opts.some((o) => o.value === v)
  const out: Partial<LogFilters> = {}
  if (!has(m.groups, f.group)) out.group = ''
  if (!has(m.tasks, f.task)) out.task = ''
  if (!has(m.providers, f.provider)) out.provider = ''
  if (!has(m.statuses, f.status)) out.status = ''
  return out
}

export function emptyReason(f: LogFilters): string {
  if (f.query.trim()) return `Nothing matches "${f.query.trim()}". Try fewer or different words.`
  if (f.show === 'running') return 'Nothing is running right now. Switch to All to see the rest.'
  return 'Adjust the date, group, task, provider or outcome filter to see runs.'
}

// Runs that came out of one piece of work are shown together: a fan-out and
// its join are one thing that happened, not four unrelated entries that happen
// to sit near each other. A workflow with only one run in view is not a group;
// every ordinary run has a workflow id, and boxing each one would be noise.
export type LogEntry = {kind: 'run'; run: Run} | {kind: 'workflow'; id: string; runs: WorkflowRun[]}

export interface WorkflowRun {
  run: Run
  // How many of its parents sit in the same group: indents a join under its fan-out.
  depth: number
}

export function groupWorkflows(runs: Run[]): LogEntry[] {
  const byWorkflow = new Map<string, Run[]>()
  for (const r of runs) {
    if (r.workflow_id) byWorkflow.set(r.workflow_id, [...(byWorkflow.get(r.workflow_id) ?? []), r])
  }
  const done = new Set<string>()
  const out: LogEntry[] = []
  for (const r of runs) {
    if (done.has(r.workflow_id ?? '')) continue
    const group = r.workflow_id ? byWorkflow.get(r.workflow_id) : undefined
    if (!group || group.length < 2) {
      out.push({kind: 'run', run: r})
      continue
    }
    done.add(r.workflow_id!)
    out.push({kind: 'workflow', id: r.workflow_id!, runs: orderWorkflow(group)})
  }
  return out
}

// A workflow's runs read oldest first: the order the work actually happened
// in, the only order a fan-out and its join read in.
function orderWorkflow(group: Run[]): WorkflowRun[] {
  const ordered = [...group].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at))
  const byId = new Map(ordered.map((r) => [r.run_id, r]))
  const depth = (r: Run, seen = new Set<string>()): number => {
    const p = r.parent_run_id ? byId.get(r.parent_run_id) : undefined
    if (!p || seen.has(p.run_id)) return 0
    seen.add(p.run_id)
    return Math.min(3, 1 + depth(p, seen))
  }
  return ordered.map((run) => ({run, depth: depth(run)}))
}

// The day an entry is filed under: a workflow sits where its newest run would.
export const entryStart = (e: LogEntry) =>
  e.kind === 'run' ? e.run.started_at : e.runs[e.runs.length - 1].run.started_at

export const runFolder = (r: Run) => (r.task?.working_dir ? baseName(r.task.working_dir) : '')

// A run can be continued by hand once it has finished for good (a rate-limited
// run is about to be resumed by the queue itself) and its session and working
// directory are on record.
export function canContinue(r: Run | undefined): boolean {
  return !!(
    r &&
    ['success', 'failed', 'auth_error', 'canceled'].includes(r.status) &&
    r.session_id &&
    r.task?.working_dir
  )
}

// A running run can be taken over: the unattended process is stopped and its
// session reopens in Terminal, where the user writes the next turn. A script
// has no session to take over.
export function canTakeOver(r: Run | undefined): boolean {
  return !!(
    r &&
    r.status === 'running' &&
    r.session_id &&
    r.task?.working_dir &&
    r.provider?.kind !== 'script'
  )
}

// Both ways a run can be called off: killing the process of a running one, and
// dropping the scheduled resume of one that only waits for the rate limit.
export function cancelMode(r: Run | undefined): 'running' | 'resume' | '' {
  if (r?.status === 'running') return 'running'
  if (r?.resume_pending) return 'resume'
  return ''
}
