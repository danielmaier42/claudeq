import {useState, type KeyboardEvent} from 'react'
import type {StatsDay} from '@/lib/types'
import {cn} from '@/lib/utils'
import {dayAxisLabel, dayLong, METRICS, niceTicks, type MetricDef} from './usage'

const PLOT_H = 168

// DailyChart is one column per day for the chosen measure. Hovering or
// arrowing onto a day shows all three measures for it, so switching the
// measure is only needed to compare the shape. An empty day has no bar but
// keeps its column and its tooltip.
export function DailyChart({days, metric}: {days: StatsDay[]; metric: MetricDef}) {
  const [active, setActive] = useState<number | null>(null)
  const values = days.map(metric.value)
  const ticks = niceTicks(Math.max(0, ...values), metric.integer)
  const top = ticks[ticks.length - 1]

  const onKey = (e: KeyboardEvent) => {
    const last = days.length - 1
    const at = active ?? last
    const to =
      e.key === 'ArrowLeft'
        ? at - 1
        : e.key === 'ArrowRight'
          ? at + 1
          : e.key === 'Home'
            ? 0
            : e.key === 'End'
              ? last
              : null
    if (to === null) return
    e.preventDefault()
    setActive(Math.max(0, Math.min(last, to)))
  }

  return (
    <div className="flex gap-3">
      <div
        className="relative w-12 shrink-0 text-right text-[11px] text-subtle-foreground tabular-nums"
        style={{height: PLOT_H}}
      >
        {ticks.map((t) => (
          <span key={t} className="absolute right-0 -translate-y-1/2" style={{bottom: `${(t / top) * 100}%`}}>
            {metric.fmt(t)}
          </span>
        ))}
      </div>
      <div className="min-w-0 flex-1">
        <div
          role="group"
          aria-label={`${metric.label} per day. Use the arrow keys to read each day.`}
          tabIndex={0}
          onKeyDown={onKey}
          onFocus={() => setActive((a) => a ?? days.length - 1)}
          onBlur={() => setActive(null)}
          onPointerLeave={() => setActive(null)}
          className="relative rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
          style={{height: PLOT_H}}
        >
          {ticks.map((t) => (
            <div
              key={t}
              className="absolute inset-x-0 h-px bg-border"
              style={{bottom: `${(t / top) * 100}%`}}
            />
          ))}
          <div className="absolute inset-0 flex">
            {days.map((d, i) => {
              const v = values[i]
              return (
                <div
                  key={d.date}
                  onPointerEnter={() => setActive(i)}
                  className={cn(
                    'relative flex h-full flex-1 items-end justify-center rounded-sm',
                    active === i && 'bg-surface-3/50',
                  )}
                >
                  {v > 0 && (
                    <div
                      className={cn(
                        'w-[58%] max-w-6 rounded-t-[4px] bg-accent transition-[height,filter] duration-300',
                        active === i && 'brightness-110',
                      )}
                      style={{height: `max(2px, ${(v / top) * 100}%)`}}
                    />
                  )}
                </div>
              )
            })}
          </div>
          {active !== null && (
            <DayTooltip day={days[active]} index={active} count={days.length} metric={metric} />
          )}
        </div>
        <div className="mt-2 flex">
          {days.map((d, i) => (
            <div
              key={d.date}
              className={cn(
                'min-w-0 flex-1 truncate text-center text-[10.5px] text-subtle-foreground',
                active === i && 'font-medium text-foreground',
                // Every second label once the columns get narrow, so they never collide.
                i % 2 !== (days.length - 1) % 2 && 'max-sm:invisible',
              )}
            >
              {dayAxisLabel(days, i)}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function DayTooltip({
  day,
  index,
  count,
  metric,
}: {
  day: StatsDay
  index: number
  count: number
  metric: MetricDef
}) {
  const center = ((index + 0.5) / count) * 100
  // Near the edges the box is anchored to its side so it never leaves the plot.
  const shift = index < 2 ? '0%' : index > count - 3 ? '-100%' : '-50%'
  const rows = [metric, ...METRICS.filter((m) => m.id !== metric.id)]
  return (
    <div
      role="status"
      className="pointer-events-none absolute top-0 z-10 min-w-36 animate-in rounded-md border border-border bg-surface px-2.5 py-2 text-[12px] shadow-md"
      style={{left: `${center}%`, transform: `translate(${shift}, -8px)`}}
    >
      <div className="mb-1 font-medium text-muted-foreground">{dayLong(day.date)}</div>
      {rows.map((m, i) => (
        <div key={m.id} className="flex items-baseline justify-between gap-4">
          <span className="text-muted-foreground">{m.label}</span>
          <span
            className={cn('tabular-nums', i === 0 ? 'font-semibold text-foreground' : 'text-foreground/85')}
          >
            {m.fmt(m.value(day))}
          </span>
        </div>
      ))}
    </div>
  )
}
