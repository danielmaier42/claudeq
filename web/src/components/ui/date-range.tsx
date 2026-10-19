import {cn} from '@/lib/utils'
import {inputClass} from './input'

// DateRange is the from–to day filter the lists share: two date fields, each
// reporting its value (YYYY-MM-DD, or "" for open) as it changes.
export function DateRange({
  from,
  to,
  onFrom,
  onTo,
  className,
}: {
  from: string
  to: string
  onFrom: (v: string) => void
  onTo: (v: string) => void
  className?: string
}) {
  const cls = cn(inputClass, 'h-8 w-[132px] px-2 text-[12.5px] tabular-nums')
  return (
    <div className={cn('flex items-center gap-1.5', className)}>
      <input
        type="date"
        aria-label="From date"
        className={cls}
        value={from}
        max={to || undefined}
        onChange={(e) => onFrom(e.target.value)}
      />
      <span className="text-subtle-foreground">–</span>
      <input
        type="date"
        aria-label="To date"
        className={cls}
        value={to}
        min={from || undefined}
        onChange={(e) => onTo(e.target.value)}
      />
    </div>
  )
}
