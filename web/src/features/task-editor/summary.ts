import {pad2} from '@/lib/format'
import {dayText, describeCron} from '@/features/queue/schedule'
import type {TaskForm} from './form'

// cronPhrase is describeCron's wording fitted to follow "Runs": "every weekday
// at 07:00", "on Mondays at 08:00", "monthly on the 1st at 04:00".
function cronPhrase(expr: string): string {
  if (!expr.trim()) return 'repeatedly (no schedule yet)'
  const d = describeCron(expr)
  if (!d) return `on the schedule ${expr.trim()}`
  if (d.startsWith('Weekdays ')) return 'every weekday ' + d.slice('Weekdays '.length)
  if (d.startsWith('Weekends ')) return 'on weekends ' + d.slice('Weekends '.length)
  if (/^(Every|Monthly|Yearly) /.test(d)) return d[0].toLowerCase() + d.slice(1)
  return 'on ' + d
}

function oncePhrase(fixedAt: string, now: Date): string {
  const d = new Date(fixedAt)
  if (!fixedAt || Number.isNaN(d.getTime())) return 'once (no time chosen yet)'
  const day = dayText(d, d.getFullYear() !== now.getFullYear())
  return `once on ${day}, ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

// triggerSummary is the editor's one-line reading of the form: when the task
// runs and on what, so the whole setup can be checked from the footer.
// runsOn is the queue's label for the target ("Claude Code · Opus", "Pool
// Night"); a script runs on nothing.
export function triggerSummary(
  f: Pick<TaskForm, 'kind' | 'trigger' | 'cron' | 'fixedAt'>,
  runsOn: string,
  now = new Date(),
): string {
  const when =
    f.trigger === 'cron'
      ? cronPhrase(f.cron)
      : f.trigger === 'fixed'
        ? oncePhrase(f.fixedAt, now)
        : 'as soon as possible'
  const on = f.kind === 'script' ? 'as a script' : 'on ' + runsOn.replace(/^Pool /, 'pool ')
  return `Runs ${when} ${on}`
}
