import {useMutation} from '@tanstack/react-query'
import {useState} from 'react'
import {ProviderLogo} from '@/components/domain/provider-logo'
import {Badge} from '@/components/ui/badge'
import {Card} from '@/components/ui/card'
import {toast} from '@/components/ui/toaster'
import {limitStatusText, noLimits} from '@/features/dashboard/limits'
import {errorMessage, post} from '@/lib/api'
import {invalidate, keys, queryClient, useLimits, useSettings} from '@/lib/queries'
import type {BackfillState, Provider, Settings} from '@/lib/types'
import {cn} from '@/lib/utils'
import {BackfillControl} from './BackfillControl'
import {backfillThreshold, isSpare} from './backfill'
import {UrgencyGauge} from './UrgencyGauge'

// BackfillSection is the threshold above which backfill tasks run on a
// provider, with every enabled provider's urgency on the same scale, so moving
// the slider shows at once which of them it lets backfill work onto.
export function BackfillSection({providers}: {providers: Provider[]}) {
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

  const kindOf = (id: string) => providers.find((p) => p.id === id)?.kind ?? ''
  const shown = (limits.data ?? []).filter((p) => p.limits.state !== 'disabled')

  return (
    <Card className="divide-y divide-border overflow-hidden">
      <BackfillControl
        value={threshold}
        saving={save.isPending}
        onDraft={setDraft}
        onCommit={(v) => (v === saved ? setDraft(null) : save.mutate(v))}
      />
      {shown.map((p) => {
        const u = p.urgency
        if (!u) return null
        const spare = draft === null ? p.backfill : isSpare(u, threshold)
        return (
          <div
            key={p.id}
            className={cn(
              'flex min-h-11 items-center gap-3 px-4 py-2 transition-colors',
              spare && 'bg-success-soft shadow-[inset_3px_0_0_var(--success)]',
            )}
          >
            <ProviderLogo kind={kindOf(p.id)} className="size-4" />
            <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{p.name}</span>
            {spare && (
              <Badge tone="success" dot>
                backfill
              </Badge>
            )}
            {noLimits(p.limits) ? (
              <span className="text-[12px] text-muted-foreground">{limitStatusText(p.limits)}</span>
            ) : (
              <UrgencyGauge u={u} threshold={threshold} spare={spare} />
            )}
          </div>
        )
      })}
    </Card>
  )
}
