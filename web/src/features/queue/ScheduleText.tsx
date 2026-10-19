import {CalendarClock, Repeat, Zap} from 'lucide-react'
import {Tooltip} from '@/components/ui/tooltip'
import {fullTime, relTime} from '@/lib/format'
import type {QueueTask} from '@/lib/types'
import {describeSchedule} from './schedule'

// Hover text for a repeating task: the expression, when it runs next, and when
// it last ran (never, until it actually has).
function cronTip(t: QueueTask): string {
  const l = [`Cron: ${t.cron}`]
  if (t.next_run) l.push(`Next run: ${fullTime(t.next_run)}`)
  l.push('Last run: ' + (t.last_run ? `${fullTime(t.last_run)} (${relTime(t.last_run)})` : 'never'))
  return l.join('\n')
}

export function ScheduleText({task: t}: {task: QueueTask}) {
  const Icon = t.trigger === 'cron' ? Repeat : t.trigger === 'fixed' ? CalendarClock : Zap
  const tip =
    t.trigger === 'cron'
      ? cronTip(t)
      : t.trigger === 'fixed' && t.fixed_at
        ? `${fullTime(t.fixed_at)} (${relTime(t.fixed_at)})`
        : 'Starts as soon as nothing holds it back'
  return (
    <Tooltip content={tip} align="start">
      <span
        tabIndex={0}
        className="inline-flex min-w-0 cursor-help items-center gap-1 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        <Icon className="size-3.5 shrink-0 text-subtle-foreground" />
        <span className="truncate">{describeSchedule(t)}</span>
        {t.trigger === 'cron' && t.next_run && t.enabled && (
          <span className="shrink-0 text-subtle-foreground">· next {relTime(t.next_run)}</span>
        )}
      </span>
    </Tooltip>
  )
}
