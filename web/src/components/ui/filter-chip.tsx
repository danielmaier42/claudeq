import {X} from 'lucide-react'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

// FilterChip shows one active filter next to the Filter button, so what the
// list is narrowed to stays visible with the popover closed. The × removes
// just that filter.
export function FilterChip({
  label,
  value,
  onRemove,
  className,
}: {
  // label names the filter ("Group"); a chip without one shows the value only
  // ("Since 1 Oct").
  label?: ReactNode
  value: ReactNode
  onRemove: () => void
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex h-7 max-w-64 min-w-0 items-center rounded-full border border-accent/25 bg-accent-soft pl-2.5 text-[12px] text-accent-text',
        className,
      )}
    >
      <span className="truncate">
        {label && <span className="opacity-75">{label}: </span>}
        <span className="font-medium">{value}</span>
      </span>
      <button
        type="button"
        onClick={onRemove}
        aria-label={
          typeof label === 'string'
            ? `Remove the ${label.toLowerCase()} filter`
            : typeof value === 'string'
              ? `Remove the filter ${value}`
              : 'Remove filter'
        }
        className="ml-0.5 inline-flex size-6 shrink-0 items-center justify-center rounded-full outline-none hover:bg-accent/15 focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        <X className="size-3" />
      </button>
    </span>
  )
}
