import type {ReactNode} from 'react'
import {Card} from '@/components/ui/card'
import {Tooltip} from '@/components/ui/tooltip'
import {duration, usd} from '@/lib/format'
import type {Stats} from '@/lib/types'
import {bucketTokens, fmtTokens, successRate} from './usage'

const COST_TIP =
  'The API-equivalent price Claude Code reports: what these requests would cost at pay-as-you-go API rates. On a subscription you are not billed for it; it draws on your usage limit.'

// UsageSummary leads with the one figure that matters most, all tokens claudeq
// has consumed, and puts the last week next to it.
export function UsageSummary({stats}: {stats: Stats}) {
  const t = stats.totals
  const w = stats.last_7d
  const rate = successRate(w)
  return (
    <div className="grid gap-3 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
      <Card className="px-5 py-4">
        <div className="text-[12.5px] text-muted-foreground">Total consumed via claudeq</div>
        <div className="mt-1 text-[44px] leading-[1.1] font-semibold tracking-[-0.02em]">
          {fmtTokens(bucketTokens(t))}
        </div>
        <div className="text-[12.5px] text-muted-foreground">tokens</div>
        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-[12.5px] text-muted-foreground tabular-nums">
          <span>{fmtTokens(t.input_tokens)} in</span>
          <span>{fmtTokens(t.output_tokens)} out</span>
          <Tooltip content={COST_TIP}>
            <span tabIndex={0} className="underline decoration-dotted underline-offset-2 outline-none">
              {usd(t.cost_usd)} API-equiv.
            </span>
          </Tooltip>
          <span>
            {t.runs} run{t.runs === 1 ? '' : 's'}
          </span>
          <span>{duration(t.duration_ms)}</span>
        </div>
      </Card>
      <Card className="px-5 py-4">
        <div className="text-[12.5px] text-muted-foreground">Last 7 days</div>
        <div className="mt-2 grid grid-cols-3 gap-x-4 gap-y-3">
          <Figure label="tokens" value={fmtTokens(bucketTokens(w))} />
          <Figure
            label={
              <Tooltip content={COST_TIP}>
                <span tabIndex={0} className="underline decoration-dotted underline-offset-2 outline-none">
                  API-equiv. cost
                </span>
              </Tooltip>
            }
            value={usd(w.cost_usd)}
          />
          <Figure label="runtime" value={duration(w.duration_ms)} />
          <Figure label="runs" value={String(w.runs)} />
          <Figure label="succeeded" value={String(w.success)} sub={rate === null ? undefined : `${rate}%`} />
          <Figure label="failed" value={String(w.failed)} danger={w.failed > 0} />
        </div>
      </Card>
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
