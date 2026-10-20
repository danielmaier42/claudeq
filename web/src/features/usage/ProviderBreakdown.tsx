import {Server} from 'lucide-react'
import {Section} from '@/components/layout/view-layout'
import {OverviewCard} from '@/features/dashboard/OverviewCard'
import {duration, usd} from '@/lib/format'
import type {StatsBucket, StatsProvider} from '@/lib/types'
import {Period} from './UsageSummary'
import {bucketTokens, fmtTokens} from './usage'

// Several providers mean several allowances, so the split says which account
// the work went to. With one provider in the period the daemon sends none:
// nothing to split. Under each provider its models, since the model decides
// what a run costs. A bar is the row's share of all tokens, or of all runs when
// none were recorded, so provider and model bars read on the same scale.
export function ProviderBreakdown({providers, period}: {providers: StatsProvider[]; period: string}) {
  const total = providers.reduce((n, p) => n + bucketTokens(p), 0)
  const runs = providers.reduce((n, p) => n + p.runs, 0)
  const shareOf = (b: StatsBucket) => (total ? bucketTokens(b) / total : runs ? b.runs / runs : 0)
  return (
    <Section>
      <OverviewCard
        icon={<Server />}
        title="By provider"
        count={<Period label={`${period}, busiest first`} />}
      >
        <div className="divide-y divide-border">
          {providers.map((p) => (
            <div key={p.id || '_'} className="py-1">
              <Row name={p.name || p.id || 'Not recorded'} bucket={p} share={shareOf(p)} />
              {p.models?.map((m) => (
                <Row
                  key={m.model || '_'}
                  name={m.model || 'Harness default'}
                  bucket={m}
                  share={shareOf(m)}
                  nested
                />
              ))}
            </div>
          ))}
        </div>
      </OverviewCard>
    </Section>
  )
}

function Row({
  name,
  bucket: b,
  share,
  nested,
}: {
  name: string
  bucket: StatsBucket
  share: number
  nested?: boolean
}) {
  return (
    <div
      className={nested ? 'flex items-center gap-4 py-1 pr-2 pl-6' : 'flex items-center gap-4 px-2 py-1.5'}
    >
      <div className="min-w-0 flex-1">
        <div className={nested ? 'truncate font-mono text-[12.5px]' : 'truncate text-[13.5px] font-medium'}>
          {name}
        </div>
        <div className="truncate text-[12px] text-muted-foreground tabular-nums">
          {b.runs} run{b.runs === 1 ? '' : 's'} · {b.success} succeeded · {b.failed} failed ·{' '}
          {duration(b.duration_ms)}
        </div>
      </div>
      <div className="hidden w-40 shrink-0 items-center gap-2 sm:flex">
        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-3">
          <div
            className={nested ? 'h-full rounded-full bg-accent/50' : 'h-full rounded-full bg-accent'}
            style={{width: `${share * 100}%`}}
          />
        </div>
        <span className="w-9 text-right text-[11.5px] text-muted-foreground tabular-nums">
          {Math.round(share * 100)}%
        </span>
      </div>
      <div className="w-24 shrink-0 text-right">
        <div
          className={
            nested ? 'text-[13px] font-medium tabular-nums' : 'text-[14px] font-semibold tabular-nums'
          }
        >
          {fmtTokens(bucketTokens(b))}
        </div>
        <div className="text-[12px] text-muted-foreground tabular-nums">{usd(b.cost_usd)}</div>
      </div>
    </div>
  )
}
