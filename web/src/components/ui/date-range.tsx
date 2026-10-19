import {useId} from 'react'
import {localDate} from '@/lib/format'
import {cn} from '@/lib/utils'
import {inputClass} from './input'
import {Label} from './label'

const PRESETS = [
  {days: 0, label: 'Any time'},
  {days: 1, label: 'Today'},
  {days: 7, label: '7 days'},
  {days: 30, label: '30 days'},
]

// daysBack is the first day of a "last n days" range that ends today.
export function daysBack(n: number, now = new Date()): string {
  const d = new Date(now)
  d.setDate(d.getDate() - (n - 1))
  return localDate(d)
}

function shortDay(day: string, now: Date): string {
  const [y, m, d] = day.split('-').map(Number)
  const date = new Date(y, m - 1, d)
  const opts: Intl.DateTimeFormatOptions = {day: 'numeric', month: 'short'}
  if (y !== now.getFullYear()) opts.year = 'numeric'
  return date.toLocaleDateString(undefined, opts)
}

// dateRangeLabel names a day range for a filter chip: "Since 1 Oct",
// "Until 5 Oct", "On 3 Oct", "1 Oct – 5 Oct"; "" when the range is open.
export function dateRangeLabel(from: string, to: string, now = new Date()): string {
  if (from && to)
    return from === to ? `On ${shortDay(from, now)}` : `${shortDay(from, now)} – ${shortDay(to, now)}`
  if (from) return from === localDate(now) ? 'Today' : `Since ${shortDay(from, now)}`
  if (to) return `Until ${shortDay(to, now)}`
  return ''
}

// DateRange is the from–to day filter the lists share, built for a filter
// popover: quick ranges first, then the two days (YYYY-MM-DD, or "" for
// open). An empty day reads "Any"
// rather than showing a date — WebKit draws today's date into an empty date
// field, which looked like a filter that was not set.
export function DateRange({
  from,
  to,
  onFrom,
  onTo,
  className,
}: {
  from: string
  to: string
  onFrom: (v: string) => void
  onTo: (v: string) => void
  className?: string
}) {
  const now = new Date()
  const preset = PRESETS.find((p) => (p.days ? !to && from === daysBack(p.days, now) : !from && !to))?.days
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div role="group" aria-label="Quick ranges" className="flex flex-wrap gap-1">
        {PRESETS.map((p) => (
          <button
            key={p.days}
            type="button"
            aria-pressed={preset === p.days}
            onClick={() => {
              onFrom(p.days ? daysBack(p.days, now) : '')
              onTo('')
            }}
            className={cn(
              'h-6 rounded-full border px-2.5 text-[12px] font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-accent/40',
              preset === p.days
                ? 'border-transparent bg-accent-soft text-accent-text'
                : 'border-border text-muted-foreground hover:bg-surface-2 hover:text-foreground',
            )}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-2">
        <DayField label="From" value={from} max={to} onChange={onFrom} />
        <DayField label="To" value={to} min={from} onChange={onTo} />
      </div>
    </div>
  )
}

function DayField({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string
  value: string
  min?: string
  max?: string
  onChange: (v: string) => void
}) {
  const id = useId()
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <Label htmlFor={id} className="text-[11.5px] font-normal text-muted-foreground">
        {label}
      </Label>
      <div className="relative">
        <input
          id={id}
          type="date"
          value={value}
          min={min || undefined}
          max={max || undefined}
          data-empty={value ? undefined : ''}
          onChange={(e) => onChange(e.target.value)}
          className={cn(
            inputClass,
            'peer h-8 px-2 text-[12.5px] tabular-nums data-[empty]:not-focus:text-transparent',
          )}
        />
        {!value && (
          <span className="pointer-events-none absolute inset-y-0 left-2 flex items-center text-[12.5px] text-subtle-foreground peer-focus:hidden">
            Any
          </span>
        )}
      </div>
    </div>
  )
}
