import {useMemo} from 'react'
import {useHealth, useModels, usePools, useProviders, useRuns} from '@/lib/queries'
import {modelLabel, providerName} from '@/lib/providers'
import type {QueueTask} from '@/lib/types'

// What a row needs to know beyond the task itself, looked up once per render
// of the list instead of once per row.
export interface QueueContext {
  // What a task actually runs on: "Claude · Opus", "Pool Night · Sonnet", "Script".
  runsOn: (t: QueueTask) => string
  // Whether the task runs on something still in beta, and which kind.
  beta: (t: QueueTask) => 'pool' | 'provider' | null
  // The live run of a running task, to open its log from the row.
  liveRun: (t: QueueTask) => string | undefined
  limitedUntil: string | undefined
}

export function useQueueContext(): QueueContext {
  const providers = useProviders().data
  const pools = usePools().data
  const runs = useRuns().data
  const limitedUntil = useHealth().data?.limited_until || undefined
  const models = useModels().data

  return useMemo(() => {
    const live = new Map<string, string>()
    // Runs come newest first, so the first running one per task is its live run.
    for (const r of runs ?? [])
      if (r.status === 'running' && !live.has(r.task_id)) live.set(r.task_id, r.run_id)
    const providerOf = (t: QueueTask) =>
      providers?.find((p) => p.id === (t.provider || '')) ?? providers?.find((p) => p.default)
    const withModel = (who: string, model: string) => {
      const label = model ? modelLabel(models, model) : ''
      return label ? `${who} · ${label}` : who
    }
    return {
      runsOn: (t) => {
        if (t.kind === 'script') return 'Script'
        // A pool task names the pool: which member a run lands on is decided
        // at the start, from the allowance left then.
        if (t.pool)
          return withModel('Pool ' + (pools?.find((p) => p.id === t.pool)?.name ?? t.pool), t.model || '')
        const p = providerOf(t)
        return withModel(
          p ? providerName(p) : t.provider || 'default provider',
          t.model || p?.default_model || '',
        )
      },
      beta: (t) => {
        if (t.kind === 'script') return null
        if (t.pool) return 'pool'
        const p = t.provider ? providers?.find((x) => x.id === t.provider) : providers?.find((x) => x.default)
        return p?.beta ? 'provider' : null
      },
      liveRun: (t) => live.get(t.id),
      limitedUntil,
    } satisfies QueueContext
  }, [providers, pools, runs, limitedUntil, models])
}
