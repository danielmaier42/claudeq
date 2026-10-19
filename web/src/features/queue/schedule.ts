import {clockTime, pad2, relTime} from '@/lib/format'
import type {QueueTask} from '@/lib/types'

const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const DAY_NAMES = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// month reads a single month (1-12 or JAN-DEC) into 0-11, or -1.
function month(s: string): number {
  const n = num(s)
  if (!Number.isNaN(n)) return n >= 1 && n <= 12 ? n - 1 : -1
  return MONTHS.findIndex((m) => m.toUpperCase() === s.toUpperCase())
}

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
// 09:30", "Every 15 minutes", "Yearly on 1 Jan at 00:00"), or returns null for
// one it cannot put simply; the row shows the expression itself then.
export function describeCron(expr: string): string | null {
  const f = expr.trim().split(/\s+/)
  if (f.length !== 5) return null
  const [min, hour, dom, mon, dow] = f
  const m = num(min)
  const step = (s: string) => (/^\*\/\d+$/.test(s) ? Number(s.slice(2)) : NaN)

  // A month narrows only a fixed day of it, once a year; anything else with a
  // month in it is left as the expression.
  if (mon !== '*') {
    const mo = month(mon)
    const d = num(dom)
    const hh = num(hour)
    // The longest each month gets (February in a leap year): a day past it
    // never comes, and is left as the expression rather than described.
    const longest = [31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
    if (
      mo < 0 ||
      dow !== '*' ||
      Number.isNaN(d) ||
      d < 1 ||
      d > longest[mo] ||
      Number.isNaN(m) ||
      Number.isNaN(hh)
    )
      return null
    if (m > 59 || hh > 23) return null
    return `Yearly on ${d} ${MONTHS[mo]} at ${pad2(hh)}:${pad2(m)}`
  }
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

// dayText is a calendar day spelled out so it cannot be misread as month/day
// ("1 Jan 2027"); withYear false leaves the year off.
export function dayText(d: Date, withYear = true): string {
  return `${d.getDate()} ${MONTHS[d.getMonth()]}` + (withYear ? ` ${d.getFullYear()}` : '')
}

// nextRunText is when a recurring task comes up next: relative while it is
// within the week, then the day itself.
export function nextRunText(iso: string, now = Date.now()): string {
  const d = new Date(iso)
  if (d.getTime() - now < 7 * 86_400_000) return relTime(d, now)
  return dayText(d)
}

// describeSchedule is the row's "when" in words: how the task is triggered.
export function describeSchedule(t: Pick<QueueTask, 'trigger' | 'cron' | 'fixed_at'>): string {
  if (t.trigger === 'fixed') return t.fixed_at ? `Once, ${clockTime(t.fixed_at)}` : 'Once'
  if (t.trigger === 'cron') return describeCron(t.cron ?? '') ?? (t.cron || 'Repeating')
  return 'As soon as possible'
}
