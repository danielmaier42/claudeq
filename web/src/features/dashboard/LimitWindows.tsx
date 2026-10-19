import {Meter} from '@/components/ui/progress'
import {Tooltip} from '@/components/ui/tooltip'
import {fullTime} from '@/lib/format'
import type {LimitWindow, ProviderLimits} from '@/lib/types'
import {cn} from '@/lib/utils'
import {leftLevel, resetText, windowLeft, windowSlots} from './limits'

const valueTone = {
  ok: 'text-foreground',
  warning: 'text-[color-mix(in_oklab,var(--warning)_75%,var(--foreground))]',
  danger: 'text-danger',
}
const meterTone = {ok: 'accent', warning: 'warning', danger: 'danger'} as const

// LimitWindows draws a provider's allowance windows as meters of what is left.
// Settings shows the same component, so a window looks alike wherever it is.
export function LimitWindows({
  limits,
  now,
  className,
}: {
  limits: ProviderLimits
  now: number
  className?: string
}) {
  const slots = windowSlots(limits.windows)
  if (!slots.length) return null
  // A failed read keeps its last windows; they are dimmed so the age shows.
  const stale = limits.state === 'unavailable'
  return (
    <div className={cn('grid grid-cols-2 gap-x-5 gap-y-3', stale && 'opacity-60', className)}>
      {slots.map((w, i) => (w ? <WindowMeter key={w.id} w={w} now={now} /> : <div key={i} aria-hidden />))}
    </div>
  )
}

function WindowMeter({w, now}: {w: LimitWindow; now: number}) {
  const left = windowLeft(w)
  const level = leftLevel(left)
  return (
    <div className="min-w-0">
      <div className="flex items-baseline justify-between gap-2 text-[12px]">
        <span className="truncate text-muted-foreground">{w.label}</span>
        <span className={cn('font-semibold tabular-nums', valueTone[level])}>
          {Math.round(left)}% <span className="font-normal text-muted-foreground">left</span>
        </span>
      </div>
      <Meter value={left} tone={meterTone[level]} className="mt-1.5" />
      {w.resets_at && (
        <Tooltip content={fullTime(w.resets_at)}>
          <div className="mt-1 w-fit text-[11.5px] text-subtle-foreground tabular-nums">
            {resetText(w, now)}
          </div>
        </Tooltip>
      )}
    </div>
  )
}
