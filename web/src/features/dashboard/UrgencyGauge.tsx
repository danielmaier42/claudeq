import {Tooltip} from '@/components/ui/tooltip'
import type {PoolRanking} from '@/lib/types'
import {cn} from '@/lib/utils'
import {BACKFILL_MAX, urgencyText} from './limits'

// UrgencyGauge puts a provider's urgency on the threshold's own scale, with
// the threshold as a tick, so "above it" reads without comparing numbers.
export function UrgencyGauge({u, threshold, spare}: {u: PoolRanking; threshold: number; spare: boolean}) {
  const text = urgencyText(u)
  if (u.tier > 0) return <div className="text-[12px] text-muted-foreground">{text}</div>
  const pos = (v: number) => `${(Math.min(v, BACKFILL_MAX) / BACKFILL_MAX) * 100}%`
  return (
    <Tooltip
      content={
        spare
          ? 'Above the backfill threshold: its weekly allowance would otherwise go unused, so backfill tasks run on it.'
          : `Below the backfill threshold of ${threshold.toFixed(1)}. Urgency 1 means spending evenly from now on uses up the week.`
      }
    >
      <div
        className="flex w-fit items-center gap-2.5 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
        tabIndex={0}
      >
        <div className="relative h-1.5 w-20 rounded-full bg-surface-3">
          <div
            className={cn(
              'h-full rounded-full transition-[width] duration-300',
              spare ? 'bg-success' : 'bg-subtle-foreground',
            )}
            style={{width: pos(u.urgency)}}
          />
          <div
            className="absolute -top-1 h-3.5 w-0.5 -translate-x-1/2 rounded-full bg-foreground/60"
            style={{left: pos(threshold)}}
          />
        </div>
        <span className="text-[12px] text-muted-foreground tabular-nums">{text}</span>
      </div>
    </Tooltip>
  )
}
