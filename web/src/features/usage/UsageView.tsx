import {ChartNoAxesColumn, RefreshCw} from 'lucide-react'
import {useState} from 'react'
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
import {DailyChart} from './DailyChart'
import {ProviderBreakdown} from './ProviderBreakdown'
import {UsageSummary} from './UsageSummary'
import {dayLong, METRICS, metricDef, type Metric} from './usage'

// Usage counts only what claudeq's own runs consumed (tokens recorded per
// run), not the account's overall usage. The poll keeps it current; the query
// cache keeps a hovered bar in place when the numbers have not changed.
export function UsageView() {
  const stats = useStats()
  return (
    <ViewLayout title="Usage" subtitle="Tokens, runs and API-equivalent cost of claudeq's own runs">
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
        <UsageContent stats={stats.data} />
      )}
    </ViewLayout>
  )
}

function UsageContent({stats}: {stats: Stats}) {
  const [metric, setMetric] = useState<Metric>('tokens')
  const def = metricDef(metric)
  const total = stats.per_day.reduce((n, d) => n + def.value(d), 0)
  return (
    <>
      <Section>
        <UsageSummary stats={stats} />
      </Section>
      <Section
        title="Per day"
        description={`Last ${stats.per_day.length} days · ${def.fmt(total)} ${def.id === 'cost' ? 'API-equiv.' : def.label.toLowerCase()} in total`}
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
        <Card className="px-4 pt-5 pb-3">
          <DailyChart days={stats.per_day} metric={def} />
          <DayTable stats={stats} />
        </Card>
      </Section>
      {!!stats.by_provider?.length && <ProviderBreakdown providers={stats.by_provider} />}
      <p className="text-[12px] text-muted-foreground">
        Counts only what claudeq's own runs consumed (tokens recorded per run), not your overall Claude
        account usage. Cost is the API-equivalent price Claude Code reports: what these requests would cost at
        pay-as-you-go API rates. On a subscription you are not billed for it; it draws on your usage limit.
      </p>
    </>
  )
}

// The chart's numbers as a table for screen readers.
function DayTable({stats}: {stats: Stats}) {
  return (
    <table className="sr-only">
      <caption>Usage per day</caption>
      <thead>
        <tr>
          <th>Day</th>
          {METRICS.map((m) => (
            <th key={m.id}>{m.label}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {stats.per_day.map((d) => (
          <tr key={d.date}>
            <td>{dayLong(d.date)}</td>
            {METRICS.map((m) => (
              <td key={m.id}>{m.fmt(m.value(d))}</td>
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
