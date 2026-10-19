import {clockTime, pad2} from '@/lib/format'
import type {QueueTask} from '@/lib/types'

const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const DAY_NAMES = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT']

const num = (s: string) => (/^\d+$/.test(s) ? Number(s) : NaN)

function dayNumber(s: string): number {
  const n = num(s)
  if (!Number.isNaN(n)) return n === 7 ? 0 : n
  return DAY_NAMES.indexOf(s.toUpperCase())
}

// days reads a day-of-week field ("1-5", "MON,WED", "6,0") into sorted day
// numbers, or null when it uses a step or anything else unusual.
function days(field: string): number[] | null {
  const out = new Set<number>()
  for (const part of field.split(',')) {
    const [a, b] = part.split('-')
    const from = dayNumber(a)
    const to = b === undefined ? from : dayNumber(b)
    if (from < 0 || to < 0 || from > 6 || to > 6 || from > to) return null
    for (let d = from; d <= to; d++) out.add(d)
  }
  return [...out].sort()
}

function dayWords(ds: number[]): string {
  const key = ds.join(',')
  if (key === '1,2,3,4,5') return 'Weekdays'
  if (key === '0,6') return 'Weekends'
  if (ds.length === 7) return 'Every day'
  return ds.map((d) => DAYS[d] + 's').join(', ')
}

function ordinal(n: number): string {
  const s = n % 100 >= 11 && n % 100 <= 13 ? 'th' : (['th', 'st', 'nd', 'rd'][n % 10] ?? 'th')
  return n + s
}

// describeCron says a five-field cron expression in words ("Weekdays at
// 09:30", "Every 15 minutes"), or returns null for one it cannot put simply;
// the row shows the expression itself then.
export function describeCron(expr: string): string | null {
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return null
  const [min, hour, dom, mon, dow] = f
  if (mon !== '*') return null
  const m = num(min)
  const step = (s: string) => (/^\*\/\d+$/.test(s) ? Number(s.slice(2)) : NaN)

  if (dom === '*' && dow === '*') {
    if (min === '*' && hour === '*') return 'Every minute'
    if (!Number.isNaN(step(min)) && hour === '*') return `Every ${step(min)} minutes`
    if (!Number.isNaN(m) && hour === '*') return m === 0 ? 'Every hour' : `Every hour at :${pad2(m)}`
    if (!Number.isNaN(m) && !Number.isNaN(step(hour)))
      return `Every ${step(hour)} hours` + (m === 0 ? '' : ` at :${pad2(m)}`)
  }
  if (Number.isNaN(m)) return null
  const hours = hour.split(',').map(num)
  if (hours.some((h) => Number.isNaN(h) || h > 23)) return null
  const at = 'at ' + hours.map((h) => `${pad2(h)}:${pad2(m)}`).join(' and ')

  if (dom === '*' && dow === '*') return `Every day ${at}`
  if (dom === '*') {
    const ds = days(dow)
    return ds ? `${dayWords(ds)} ${at}` : null
  }
  if (dow === '*') {
    const d = num(dom)
    return Number.isNaN(d) ? null : `Monthly on the ${ordinal(d)} ${at}`
  }
  return null
}

// describeSchedule is the row's "when" in words: how the task is triggered.
export function describeSchedule(t: Pick<QueueTask, 'trigger' | 'cron' | 'fixed_at'>): string {
  if (t.trigger === 'fixed') return t.fixed_at ? `Once, ${clockTime(t.fixed_at)}` : 'Once'
  if (t.trigger === 'cron') return describeCron(t.cron ?? '') ?? (t.cron || 'Repeating')
  return 'As soon as possible'
}
