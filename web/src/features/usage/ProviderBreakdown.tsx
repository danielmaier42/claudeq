import {Section} from '@/components/layout/view-layout'
import {Card} from '@/components/ui/card'
import {duration, usd} from '@/lib/format'
import type {StatsProvider} from '@/lib/types'
import {bucketTokens, fmtTokens} from './usage'

// Several providers mean several allowances, so the split says which account
// the work went to. With one provider the daemon sends none: nothing to split.
export function ProviderBreakdown({providers}: {providers: StatsProvider[]}) {
  const total = providers.reduce((n, p) => n + bucketTokens(p), 0)
  const runs = providers.reduce((n, p) => n + p.runs, 0)
  return (
    <Section
      title="By provider"
      description="All recorded runs, busiest first. The bar is the provider's share of the tokens, or of the runs when none were recorded."
    >
      <Card className="divide-y divide-border">
        {providers.map((p) => {
          const share = total ? bucketTokens(p) / total : runs ? p.runs / runs : 0
          return (
            <div key={p.id || '_'} className="flex items-center gap-4 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="truncate text-[13.5px] font-medium">{p.name || p.id || 'Not recorded'}</div>
                <div className="truncate text-[12px] text-muted-foreground tabular-nums">
                  {p.runs} run{p.runs === 1 ? '' : 's'} · {p.success} succeeded · {p.failed} failed ·{' '}
                  {duration(p.duration_ms)}
                </div>
              </div>
              <div className="hidden w-40 shrink-0 items-center gap-2 sm:flex">
                <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-3">
                  <div className="h-full rounded-full bg-accent" style={{width: `${share * 100}%`}} />
                </div>
                <span className="w-9 text-right text-[11.5px] text-muted-foreground tabular-nums">
                  {Math.round(share * 100)}%
                </span>
              </div>
              <div className="w-24 shrink-0 text-right">
                <div className="text-[14px] font-semibold tabular-nums">{fmtTokens(bucketTokens(p))}</div>
                <div className="text-[12px] text-muted-foreground tabular-nums">{usd(p.cost_usd)}</div>
              </div>
            </div>
          )
        })}
      </Card>
    </Section>
  )
}
