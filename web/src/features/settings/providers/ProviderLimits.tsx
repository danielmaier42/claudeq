import {Gauge} from 'lucide-react'
import {Meter} from '@/components/ui/progress'
import {Tooltip} from '@/components/ui/tooltip'
import {fullTime} from '@/lib/format'
import type {ProviderLimits as Limits} from '@/lib/types'
import {cn} from '@/lib/utils'
import {limitStatusText, resetText, windowLeft, windowTone} from './provider-logic'

// ProviderLimits is the provider's allowance windows, the same reading the
// Dashboard shows: what the daemon last read, never a fresh read of its own.
export function ProviderLimits({limits}: {limits: Limits}) {
  const windows = limits.windows ?? []
  const problem = limits.state === 'unavailable'
  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex items-center gap-1.5 text-[12px]">
        <Gauge className={cn('size-3.5', problem ? 'text-warning' : 'text-muted-foreground')} />
        <span className="font-medium">Limits</span>
        <Tooltip content={limits.updated_at ? 'Read ' + fullTime(limits.updated_at) : undefined}>
          <span className={cn('min-w-0 truncate', problem ? 'text-warning' : 'text-muted-foreground')}>
            · {limitStatusText(limits)}
          </span>
        </Tooltip>
        {limits.plan && <span className="ml-auto text-muted-foreground">{limits.plan}</span>}
      </div>
      {windows.length > 0 && (
        <div className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
          {windows.map((w) => {
            const left = windowLeft(w)
            const tone = windowTone(left)
            return (
              <div key={w.id} className="min-w-0">
                <div className="mb-1 flex items-baseline justify-between gap-2 text-[12px]">
                  <span className="truncate text-muted-foreground">{w.label}</span>
                  <span
                    className={cn(
                      'font-semibold tabular-nums',
                      tone === 'danger' && 'text-danger',
                      tone === 'warning' && 'text-warning',
                    )}
                  >
                    {Math.round(left)}% left
                  </span>
                </div>
                <Meter value={left} tone={tone} />
                <div className="mt-1 text-[11.5px] text-subtle-foreground">{resetText(w)}</div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
