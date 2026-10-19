import {ChartNoAxesColumn, RefreshCw} from 'lucide-react'
import {useMemo, useState} from 'react'
import {Section, ViewLayout} from '@/components/layout/view-layout'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {Card} from '@/components/ui/card'
import {EmptyState} from '@/components/ui/empty-state'
import {Segmented} from '@/components/ui/segmented'
import {Skeleton} from '@/components/ui/skeleton'
import {errorMessage} from '@/lib/api'
import {useStats} from '@/lib/queries'
import type {Stats} from '@/lib/types'
import {cn} from '@/lib/utils'
import {bucketTitle, chartBuckets, periodLabel, RANGES, type ChartBucket, type ChartUnit} from './chart'
import {ProviderBreakdown} from './ProviderBreakdown'
import {setUsageRange, useUsageRange} from './store'
import {UsageChart} from './UsageChart'
import {UsageSummary} from './UsageSummary'
import {emptyText, METRICS, metricDef, type Metric} from './usage'

// Usage counts only what claudeq's own runs consumed (tokens recorded per
// run), not the account's overall usage. The poll keeps it current; the query
// cache keeps a hovered bar in place when the numbers have not changed.
export function UsageView() {
  const range = useUsageRange((s) => s.range)
  const stats = useStats(range)
  return (
    <ViewLayout
      title="Usage"
      subtitle="Tokens, runs and API-equivalent cost of claudeq's own runs"
      actions={
        <Segmented
          aria-label="Period"
          value={String(range)}
          onValueChange={(v) => setUsageRange(RANGES.find((r) => r.value === v)?.range ?? range)}
          options={RANGES.map((r) => ({value: r.value, label: r.label}))}
        />
      }
    >
      {stats.isPending ? (
        <UsageSkeleton />
      ) : stats.isError && !stats.data ? (
        <Alert
          tone="danger"
          title="Could not load usage"
          actions={
            <Button size="sm" onClick={() => stats.refetch()} loading={stats.isFetching}>
              <RefreshCw />
              Retry
            </Button>
          }
        >
          {errorMessage(stats.error)}
        </Alert>
      ) : !stats.data || stats.data.totals.runs === 0 ? (
        <EmptyState
          icon={<ChartNoAxesColumn />}
          title="Nothing consumed yet"
          description="Token usage appears here after claudeq has run some tasks."
        />
      ) : (
        // The previous period stays on screen, dimmed, until the new one is
        // read, so switching never flashes the page empty.
        <div
          aria-busy={stats.isPlaceholderData}
          className={cn('transition-opacity', stats.isPlaceholderData && 'opacity-60')}
        >
          <UsageContent stats={stats.data} />
        </div>
      )}
    </ViewLayout>
  )
}

function UsageContent({stats}: {stats: Stats}) {
  const [metric, setMetric] = useState<Metric>('tokens')
  const def = metricDef(metric)
  const {unit, buckets} = useMemo(() => chartBuckets(stats.per_day), [stats.per_day])
  const total = buckets.reduce((n, b) => n + def.value(b), 0)
  const label = periodLabel(stats)
  // A history longer than the daemon's chart cap is charted in its newest
  // part; the description says so rather than calling it all time.
  const chartLabel = stats.per_day.length < stats.days ? `Last ${stats.per_day.length} days` : label
  return (
    <>
      <Section>
        <UsageSummary stats={stats} />
      </Section>
      <Section
        title={unit === 'week' ? 'Per week' : 'Per day'}
        description={`${chartLabel} · ${def.fmt(total)} ${def.id === 'cost' ? 'API-equiv.' : def.label.toLowerCase()} in total`}
        actions={
          <Segmented
            aria-label="Measure"
            size="sm"
            value={metric}
            onValueChange={setMetric}
            options={METRICS.map((m) => ({value: m.id, label: m.id === 'cost' ? 'Cost' : m.label}))}
          />
        }
      >
        {total > 0 ? (
          <Card className="px-4 pt-5 pb-3">
            <UsageChart buckets={buckets} unit={unit} metric={def} />
            <BucketTable buckets={buckets} unit={unit} />
          </Card>
        ) : (
          <Card className="flex items-center gap-2 px-4 py-6 text-[12.5px] text-muted-foreground">
            <ChartNoAxesColumn className="size-4 text-subtle-foreground" />
            {emptyText(metric)}
          </Card>
        )}
      </Section>
      {!!stats.by_provider?.length && <ProviderBreakdown providers={stats.by_provider} period={label} />}
      <p className="text-[12px] text-muted-foreground">
        Counts only what claudeq's own runs consumed (tokens recorded per run), not your overall Claude
        account usage. Cost is the API-equivalent price Claude Code reports: what these requests would cost at
        pay-as-you-go API rates. On a subscription you are not billed for it; it draws on your usage limit.
      </p>
    </>
  )
}

// The chart's numbers as a table for screen readers.
function BucketTable({buckets, unit}: {buckets: ChartBucket[]; unit: ChartUnit}) {
  return (
    <table className="sr-only">
      <caption>Usage per {unit}</caption>
      <thead>
        <tr>
          <th>{unit === 'week' ? 'Week' : 'Day'}</th>
          {METRICS.map((m) => (
            <th key={m.id}>{m.label}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {buckets.map((b) => (
          <tr key={b.date}>
            <td>{bucketTitle(b, unit)}</td>
            {METRICS.map((m) => (
              <td key={m.id}>{m.fmt(m.value(b))}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function UsageSkeleton() {
  return (
    <div className="space-y-7">
      <div className="grid gap-3 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <Skeleton className="h-36" />
        <Skeleton className="h-36" />
      </div>
      <Skeleton className="h-60" />
    </div>
  )
}
