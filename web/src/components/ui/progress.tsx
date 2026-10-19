import {cn} from '@/lib/utils'

const tones = {
  accent: 'bg-accent',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  muted: 'bg-subtle-foreground',
}

// Meter is a horizontal bar filled to `value` of `max`, e.g. how much of an
// allowance is used.
export function Meter({
  value,
  max = 100,
  tone = 'accent',
  className,
}: {
  value: number
  max?: number
  tone?: keyof typeof tones
  className?: string
}) {
  const pct = max > 0 ? Math.max(0, Math.min(100, (value / max) * 100)) : 0
  return (
    <div
      className={cn('h-1.5 w-full overflow-hidden rounded-full bg-surface-3', className)}
      role="meter"
      aria-valuenow={value}
      aria-valuemax={max}
    >
      <div
        className={cn('h-full rounded-full transition-[width] duration-500', tones[tone])}
        style={{width: `${pct}%`}}
      />
    </div>
  )
}
