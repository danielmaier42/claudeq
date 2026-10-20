import {Activity, Coins} from 'lucide-react'
import type {ReactNode} from 'react'
import {OverviewCard} from '@/features/dashboard/OverviewCard'
import {Tooltip} from '@/components/ui/tooltip'
import {duration, usd} from '@/lib/format'
import type {Stats} from '@/lib/types'
import {periodLabel} from './chart'
import {bucketTokens, fmtTokens, successRate} from './usage'

const COST_TIP =
  'The API-equivalent price Claude Code reports: what these requests would cost at pay-as-you-go API rates. On a subscription you are not billed for it; it draws on your usage limit.'

// UsageSummary leads with the one figure that matters most, the tokens
// dispatchq consumed in the chosen period, and puts how the runs went next to
// it, both in the Dashboard's tiles. The all-time total stays in view as a
// smaller figure, so a short period never hides how much has gone through
// dispatchq overall.
export function UsageSummary({stats}: {stats: Stats}) {
  const p = stats.period
  const t = stats.totals
  const label = periodLabel(stats)
  const rate = successRate(p)
  const perDay = bucketTokens(p) / Math.max(1, stats.days)
  return (
    <div className="grid gap-3 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
      <OverviewCard icon={<Coins />} title="Consumed via dispatchq" count={<Period label={label} />}>
        <div className="flex flex-1 flex-col px-2 pt-1 pb-1">
          <div className="text-[44px] leading-[1.1] font-semibold tracking-[-0.02em] tabular-nums">
            {fmtTokens(bucketTokens(p))}
          </div>
          <div className="text-[12.5px] text-muted-foreground">tokens</div>
          <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-[12.5px] text-muted-foreground tabular-nums">
            <span>{fmtTokens(p.input_tokens)} in</span>
            <span>{fmtTokens(p.output_tokens)} out</span>
            {stats.days > 1 && <span>{fmtTokens(Math.round(perDay))} per day</span>}
          </div>
          {!stats.all && (
            <div className="mt-auto flex flex-wrap items-baseline gap-x-3 gap-y-1 pt-3 text-[12.5px] text-muted-foreground tabular-nums">
              <span className="font-medium text-foreground">All time</span>
              <span>{fmtTokens(bucketTokens(t))} tokens</span>
              <span>{usd(t.cost_usd)} API-equiv.</span>
              <span>
                {t.runs} run{t.runs === 1 ? '' : 's'}
              </span>
            </div>
          )}
        </div>
      </OverviewCard>
      <OverviewCard icon={<Activity />} title="Runs" count={<Period label={label} />}>
        <div className="grid grid-cols-3 gap-x-4 gap-y-3 px-2 pt-1 pb-1">
          <Figure label="runs" value={String(p.runs)} />
          <Figure label="succeeded" value={String(p.success)} sub={rate === null ? undefined : `${rate}%`} />
          <Figure label="failed" value={String(p.failed)} danger={p.failed > 0} />
          <Figure
            label={
              <Tooltip content={COST_TIP}>
                <span tabIndex={0} className="underline decoration-dotted underline-offset-2 outline-none">
                  API-equiv. cost
                </span>
              </Tooltip>
            }
            value={usd(p.cost_usd)}
          />
          <Figure label="runtime" value={duration(p.duration_ms)} />
          <Figure label="per run" value={p.runs ? duration(p.duration_ms / p.runs) : '–'} />
        </div>
      </OverviewCard>
    </div>
  )
}

function Figure({
  label,
  value,
  sub,
  danger,
}: {
  label: ReactNode
  value: string
  sub?: string
  danger?: boolean
}) {
  return (
    <div className="min-w-0">
      <div className="flex items-baseline gap-1.5">
        <span className={`text-[20px] leading-7 font-semibold tabular-nums ${danger ? 'text-danger' : ''}`}>
          {value}
        </span>
        {sub && <span className="text-[12px] text-muted-foreground tabular-nums">{sub}</span>}
      </div>
      <div className="truncate text-[12px] text-muted-foreground">{label}</div>
    </div>
  )
}

// Period names the chosen range next to a tile's title.
export function Period({label}: {label: ReactNode}) {
  return <span className="min-w-0 truncate text-[12px] text-muted-foreground">{label}</span>
}
