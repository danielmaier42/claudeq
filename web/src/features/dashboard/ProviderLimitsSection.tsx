import {useMutation} from '@tanstack/react-query'
import {AlertTriangle, Gauge, RefreshCw} from 'lucide-react'
import {useState} from 'react'
import {openSettings} from '@/app/nav'
import {Section} from '@/components/layout/view-layout'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Card} from '@/components/ui/card'
import {EmptyState} from '@/components/ui/empty-state'
import {Skeleton} from '@/components/ui/skeleton'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {errorMessage, post} from '@/lib/api'
import {fullTime} from '@/lib/format'
import {invalidate, keys, queryClient, useLimits, useSettings} from '@/lib/queries'
import type {BackfillState, LimitsEntry, Settings} from '@/lib/types'
import {cn} from '@/lib/utils'
import {BackfillControl} from './BackfillControl'
import {LimitWindows} from './LimitWindows'
import {backfillThreshold, isSpare, limitStatusText} from './limits'
import {UrgencyGauge} from './UrgencyGauge'

// ProviderLimitsSection is how much of each provider's allowance is left, how
// urgent it is to spend it, and the threshold above which backfill tasks do.
export function ProviderLimitsSection({
  now,
  reading,
  onRefresh,
}: {
  now: number
  reading: boolean
  onRefresh: () => void
}) {
  const limits = useLimits()
  const settings = useSettings()
  const saved = backfillThreshold(settings.data)
  // draft is the slider's value from the first move until the daemon has the
  // new threshold, so neither the poll nor a slow save makes it jump back.
  const [draft, setDraft] = useState<number | null>(null)
  const threshold = draft ?? saved

  const save = useMutation({
    mutationFn: (urgency: number) => post<BackfillState>('/api/backfill', {urgency}),
    onSuccess: (r) => {
      queryClient.setQueryData<Settings>(keys.settings, (s) => (s ? {...s, backfill_urgency: r.urgency} : s))
      return invalidate(keys.limits)
    },
    onError: (e) => toast.error('Could not save the threshold', {description: errorMessage(e)}),
    onSettled: () => setDraft(null),
  })

  const shown = (limits.data ?? []).filter((p) => p.limits.state !== 'disabled')
  const refresh = (
    <Tooltip content="Ask every provider for its allowance again. Readings younger than half a minute are reused.">
      <Button variant="ghost" size="sm" onClick={onRefresh} disabled={reading}>
        <RefreshCw className={cn(reading && 'animate-spin')} />
        {reading ? 'Reading…' : 'Read now'}
      </Button>
    </Tooltip>
  )

  return (
    <Section
      title="Provider limits"
      description="What is left of each account's allowance. Read without spending any: on open, every 15 minutes and after each run."
      actions={refresh}
    >
      {limits.isPending ? (
        <LimitsSkeleton />
      ) : !shown.length ? (
        <Card>
          <EmptyState
            icon={<Gauge />}
            title="No providers"
            description="Enable a provider in Settings → Providers to see its limits here."
            action={<Button onClick={() => openSettings('providers')}>Open Settings</Button>}
          />
        </Card>
      ) : (
        <Card className="divide-y divide-border overflow-hidden">
          <BackfillControl
            value={threshold}
            saving={save.isPending}
            onDraft={setDraft}
            onCommit={(v) => (v === saved ? setDraft(null) : save.mutate(v))}
          />
          {shown.map((p) => (
            <ProviderRow
              key={p.id}
              p={p}
              now={now}
              threshold={threshold}
              spare={draft === null ? p.backfill : isSpare(p.urgency, threshold)}
            />
          ))}
        </Card>
      )}
    </Section>
  )
}

function ProviderRow({
  p,
  now,
  threshold,
  spare,
}: {
  p: LimitsEntry
  now: number
  threshold: number
  spare: boolean
}) {
  const l = p.limits
  const problem = l.state === 'unavailable'
  const hasWindows = !!l.windows?.length
  return (
    // A provider above the threshold is lit up: its allowance is at risk of
    // going unused, and backfill tasks may run on it right now.
    <div
      className={cn(
        'flex flex-wrap items-start gap-x-6 gap-y-3 px-4 py-3.5 transition-colors',
        spare && 'bg-success-soft shadow-[inset_3px_0_0_var(--success)]',
      )}
    >
      <div className="min-w-[200px] flex-1 basis-[220px]">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[13.5px] font-semibold">{p.name}</span>
          {p.type_name && <Badge tone="outline">{p.type_name}</Badge>}
          {p.default && <Badge>default</Badge>}
          {l.plan && <Badge tone="neutral">{l.plan}</Badge>}
          {spare && (
            <Tooltip content="Its weekly allowance would otherwise go unused, so backfill tasks run on it">
              <Badge tone="success" dot tabIndex={0}>
                backfill
              </Badge>
            </Tooltip>
          )}
        </div>
        <StatusLine p={p} now={now} problem={problem} />
        {p.urgency && (
          <div className="mt-2">
            <UrgencyGauge u={p.urgency} threshold={threshold} spare={spare} />
          </div>
        )}
      </div>
      {hasWindows && <LimitWindows limits={l} now={now} className="min-w-[300px] flex-[2] basis-[320px]" />}
    </div>
  )
}

function StatusLine({p, now, problem}: {p: LimitsEntry; now: number; problem: boolean}) {
  const l = p.limits
  const line = (
    <p
      className={cn(
        'mt-1 flex items-start gap-1.5 text-[12.5px]',
        problem ? 'text-[color-mix(in_oklab,var(--warning)_75%,var(--foreground))]' : 'text-muted-foreground',
      )}
    >
      {problem && <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />}
      <span>{limitStatusText(l, now)}</span>
    </p>
  )
  const at = l.updated_at ?? l.checked_at
  return at ? (
    <Tooltip content={`${l.updated_at ? 'Read' : 'Checked'} ${fullTime(at)}`}>{line}</Tooltip>
  ) : (
    line
  )
}

function LimitsSkeleton() {
  return (
    <Card className="divide-y divide-border">
      {[0, 1].map((i) => (
        <div key={i} className="flex gap-6 px-4 py-4">
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-28" />
          </div>
          <div className="grid flex-[2] grid-cols-2 gap-5">
            <Skeleton className="h-8" />
            <Skeleton className="h-8" />
          </div>
        </div>
      ))}
    </Card>
  )
}
