import type {ReactNode} from 'react'
import {Badge, type BadgeTone} from '@/components/ui/badge'
import {Tooltip} from '@/components/ui/tooltip'
import {resumeAt, resumeText} from '@/lib/format'
import type {QueueTask} from '@/lib/types'
import {lastOutcome} from './outcome'
import {waitedFor} from './queue-model'
import {describeSchedule} from './schedule'
import type {QueueContext} from './use-queue-context'

// The chips that say why a task is not running yet; they sit next to the name.
// The rest describe how it runs and sit with the schedule.
export const STATUS_CHIPS = new Set(['blocked', 'rescheduled', 'hold', 'deps'])

export interface Chip {
  key: string
  label: ReactNode
  tone: BadgeTone
  tip: string
}

// taskChips is everything worth knowing about a task at a glance, each with
// the sentence that explains it on hover. The order puts what blocks the task
// first and what merely describes it last.
export function taskChips(t: QueueTask, ctx: QueueContext): Chip[] {
  const out: Chip[] = []
  if (t.blocked_reason)
    out.push({
      key: 'blocked',
      label: 'Blocked',
      tone: 'danger',
      tip: t.blocked_reason + ' The task keeps its place and starts by itself once the provider works again.',
    })
  if (t.waiting_for_limit)
    out.push({
      key: 'rescheduled',
      label: 'Rescheduled',
      tone: 'warning',
      tip:
        'The rate limit interrupted this task. Its Claude session ' +
        resumeText(resumeAt(undefined, ctx.limitedUntil)) +
        ' — drop the resume in the Log with “Cancel resume”.',
    })
  // The scheduler's own verdict on why a due task did not start, so a task that
  // sits in the queue says what it waits for, and since when.
  if (t.hold && !t.blocked_reason) {
    const waited = waitedFor(t.hold.since)
    out.push({
      key: 'hold',
      label: waited ? `Waiting ${waited}` : 'Waiting',
      tone: 'warning',
      tip:
        t.hold.reason +
        ' Due since ' +
        new Date(t.hold.since).toLocaleTimeString() +
        '; it starts by itself once that changes.',
    })
  }
  // A job that waits for other jobs looks like one that never starts, unless
  // the queue says what it is waiting for.
  const deps = t.waiting_for ?? []
  if (deps.length)
    out.push({
      key: 'deps',
      label: `Waiting for ${deps.length} job${deps.length > 1 ? 's' : ''}`,
      tone: 'neutral',
      tip: `Starts by itself once these jobs have finished: ${deps.join(', ')}. It runs even if one of them fails.`,
    })
  if (t.kind === 'script')
    out.push({
      key: 'script',
      label: 'Script',
      tone: 'neutral',
      tip: 'Runs as a program, without a model: no provider, no usage, and a rate limit never holds it up',
    })
  if (t.backfill)
    out.push({
      key: 'backfill',
      label: 'Backfill',
      tone: 'accent',
      tip: "Runs only while its provider's weekly allowance would otherwise go unused (urgency above the threshold on the Dashboard)",
    })
  if (t.parallel)
    out.push({
      key: 'parallel',
      label: 'Parallel',
      tone: 'neutral',
      tip: 'Runs alongside other parallel tasks',
    })
  if (t.permissions === 'skip')
    out.push({key: 'granted', label: 'Granted', tone: 'warning', tip: 'Skips permission prompts'})
  if (t.notify_on_result)
    out.push({
      key: 'notifies',
      label: 'Notifies',
      tone: 'accent',
      tip: 'Sends outcome and last message when it finishes',
    })
  if (t.quiet_history)
    out.push({
      key: 'silent',
      label: 'Silent',
      tone: 'neutral',
      tip: 'Successful runs stay out of the Log; failures are kept',
    })
  return out
}

// searchTexts is what the queue's search looks through for a task: what the
// row shows, in the words it shows it.
export function searchTexts(t: QueueTask, ctx: QueueContext): string[] {
  const out = [t.name, t.group ?? '', describeSchedule(t), t.cron ?? '']
  if (t.kind !== 'script') out.push(ctx.runsOn(t))
  if (!t.enabled) out.push('Paused')
  if (t.running) out.push('Running')
  const last = lastOutcome(ctx.lastRun(t))
  if (last) out.push(last.text)
  for (const c of taskChips(t, ctx)) if (typeof c.label === 'string') out.push(c.label)
  return out
}

export function TaskChips({chips}: {chips: Chip[]}) {
  return (
    <>
      {chips.map((c) => (
        <Tooltip key={c.key} content={c.tip}>
          {/* Focusable so the explanation is reachable without a mouse. */}
          <Badge
            tone={c.tone}
            tabIndex={0}
            className="cursor-help outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            {c.label}
          </Badge>
        </Tooltip>
      ))}
    </>
  )
}
