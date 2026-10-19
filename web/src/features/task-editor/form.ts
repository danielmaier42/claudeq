import {pad2} from '@/lib/format'
import type {Task, TaskInput, TaskKind, TaskTrigger} from '@/lib/types'

// TaskForm is what the editor edits: every field as the controls hold it. The
// provider and the pool share one choice ("pool:ID" for a pool), because a task
// runs on one or the other, never both.
export interface TaskForm {
  name: string
  // The queue section the task sits in; "" is none.
  group: string
  kind: TaskKind
  prompt: string
  workingDir: string
  trigger: TaskTrigger
  // datetime-local text ("2026-10-19T20:00"), local time.
  fixedAt: string
  cron: string
  target: string
  model: string
  effort: string
  skipPermissions: boolean
  parallel: boolean
  notify: boolean
  quietHistory: boolean
  backfill: boolean
}

export type FieldKey = 'name' | 'group' | 'prompt' | 'workingDir' | 'fixedAt' | 'cron' | 'target'
export type FieldErrors = Partial<Record<FieldKey, string>>

// The daemon's limit (task.MaxGroupLen): a group name is a section header.
export const MAX_GROUP_LEN = 60

export const POOL_PREFIX = 'pool:'
export const poolOf = (target: string) =>
  target.startsWith(POOL_PREFIX) ? target.slice(POOL_PREFIX.length) : ''

export function toLocalDT(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}T${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

export function formFromTask(t: Partial<Task>): TaskForm {
  const trigger = t.trigger ?? 'asap'
  return {
    name: t.name ?? '',
    group: t.group ?? '',
    kind: t.kind === 'script' ? 'script' : 'agent',
    prompt: t.prompt ?? '',
    workingDir: t.working_dir ?? '',
    trigger,
    fixedAt: trigger === 'fixed' && t.fixed_at ? toLocalDT(t.fixed_at) : '',
    cron: trigger === 'cron' ? (t.cron ?? '') : '',
    target: t.pool ? POOL_PREFIX + t.pool : (t.provider ?? ''),
    model: t.model ?? '',
    effort: t.reasoning_effort ?? '',
    skipPermissions: t.permissions === 'skip',
    parallel: !!t.parallel,
    notify: !!t.notify_on_result,
    quietHistory: !!t.quiet_history,
    backfill: !!t.backfill,
  }
}

// setKind switches between the two job types. A script has no permission grant
// and cannot be backfill work (the daemon refuses both), so those are cleared
// rather than kept hidden and sent.
export function withKind(f: TaskForm, kind: TaskKind): TaskForm {
  return kind === 'script' ? {...f, kind, skipPermissions: false, backfill: false} : {...f, kind}
}

// Changing where a task runs must not carry the old provider's model (or
// effort) into the new one.
export function withTarget(f: TaskForm, target: string): TaskForm {
  return {...f, target, model: '', effort: ''}
}

// A new recurring task starts from the most common schedule rather than an
// empty field: once a night, while nobody is at the Mac.
export const DEFAULT_CRON = '0 3 * * *'

// withTrigger switches how the task is triggered; an empty schedule is
// prefilled so Recurring is never a blank expression to work out.
export function withTrigger(f: TaskForm, trigger: TaskTrigger): TaskForm {
  return trigger === 'cron' && !f.cron.trim() ? {...f, trigger, cron: DEFAULT_CRON} : {...f, trigger}
}

// validate catches what the daemon would refuse anyway, so the reason lands on
// the field instead of arriving as one line from the server. The cron
// expression is judged by the daemon itself (useCronCheck), not here.
export function validate(f: TaskForm): FieldErrors {
  const e: FieldErrors = {}
  if (!f.prompt.trim())
    e.prompt = f.kind === 'script' ? 'Write the script to run.' : 'Tell Claude what to do.'
  if ([...f.group.trim()].length > MAX_GROUP_LEN) e.group = `At most ${MAX_GROUP_LEN} characters.`
  if (!f.workingDir.trim()) e.workingDir = 'Please choose a working directory.'
  if (f.trigger === 'fixed') {
    if (!f.fixedAt) e.fixedAt = 'Choose when it may start.'
    else if (Number.isNaN(new Date(f.fixedAt).getTime())) e.fixedAt = 'Not a valid date and time.'
  }
  return e
}

// Fields a task carries that the editor does not show. An edit keeps them as
// stored (PUT replaces the whole task), and an add keeps what its caller
// prefilled.
const CARRIED = ['id', 'depends_on', 'include_results', 'parent_run', 'workflow_id', 'enabled'] as const
type Carried = Pick<TaskInput, (typeof CARRIED)[number]>

export function carriedFields(t: Partial<TaskInput> | undefined): Carried {
  const out: Carried = {}
  if (!t) return out
  for (const k of CARRIED) if (t[k] !== undefined) Object.assign(out, {[k]: t[k]})
  return out
}

// buildPayload turns the form into the request body. What only an agent job
// has is left off entirely for a script rather than sent empty: the daemon
// refuses a script that names any of it.
//
// keptGroup is the group an edited task had when the editor opened. Left as it
// was, the group is not sent at all: the daemon then keeps the stored one, so a
// drag to another group while the editor is open is not undone by saving.
export function buildPayload(
  f: TaskForm,
  opts: {takesEffort: boolean; carried?: Carried; keptGroup?: string},
): TaskInput {
  const script = f.kind === 'script'
  const pool = poolOf(f.target)
  const group = f.group.trim()
  return {
    enabled: true,
    ...opts.carried,
    name: f.name.trim(),
    group: opts.keptGroup !== undefined && group === opts.keptGroup ? undefined : group,
    kind: script ? 'script' : undefined,
    prompt: f.prompt,
    working_dir: f.workingDir.trim(),
    trigger: f.trigger,
    fixed_at: f.trigger === 'fixed' && f.fixedAt ? new Date(f.fixedAt).toISOString() : undefined,
    cron: f.trigger === 'cron' ? f.cron.trim() : undefined,
    parallel: f.parallel,
    notify_on_result: f.notify,
    quiet_history: f.quietHistory,
    backfill: !script && f.backfill,
    model: script ? undefined : f.model || undefined,
    permissions: !script && f.skipPermissions ? 'skip' : 'default',
    provider: script || pool ? undefined : f.target || undefined,
    pool: !script && pool ? pool : undefined,
    reasoning_effort: script || !opts.takesEffort ? undefined : f.effort || undefined,
  }
}

// fieldForError files a refusal from the daemon under the field it is about,
// so it shows where the fix is. Anything else stays a message for the footer.
export function fieldForError(message: string): FieldKey | null {
  const m = message.toLowerCase()
  if (/missing (prompt|script)/.test(m)) return 'prompt'
  if (m.includes('invalid group')) return 'group'
  if (m.includes('working_dir') || m.includes('working directory')) return 'workingDir'
  if (m.includes('fixed_at')) return 'fixedAt'
  if (m.includes('cron') || /\b(minute|hour|day|month|weekday|day of week)\b.*field/.test(m)) return 'cron'
  if (m.includes('provider') || m.includes('pool')) return 'target'
  return null
}
