import {ToggleGroup} from 'radix-ui'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'
import {Tooltip} from './tooltip'

export interface SegmentOption<T extends string> {
  value: T
  label: ReactNode
  tip?: ReactNode
}

// Segmented is a single-choice control with one option always selected, like
// All | Running. Clicking the selected option again keeps it selected. The
// chosen option is styled by aria-checked, not data-state: a Tooltip around an
// option puts its own data-state (closed / open) on the same element.
export function Segmented<T extends string>({
  value,
  onValueChange,
  options,
  size = 'md',
  tone = 'neutral',
  className,
  'aria-label': ariaLabel,
}: {
  value: T
  onValueChange: (v: T) => void
  options: SegmentOption<T>[]
  size?: 'sm' | 'md'
  // accent fills the chosen option with the accent colour: for a choice that
  // defines what is being made (a job's type), not for a view filter.
  tone?: 'neutral' | 'accent'
  className?: string
  'aria-label'?: string
}) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      aria-label={ariaLabel}
      onValueChange={(v) => v && onValueChange(v as T)}
      className={cn('inline-flex shrink-0 items-center rounded-md bg-surface-3/80 p-0.5', className)}
    >
      {options.map((o) => (
        <Tooltip key={o.value} content={o.tip}>
          <ToggleGroup.Item
            value={o.value}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-[6px] px-2.5 font-medium text-muted-foreground transition-all outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40 aria-checked:bg-surface aria-checked:text-foreground aria-checked:shadow-sm dark:aria-checked:bg-border-strong [&_svg]:size-3.5',
              size === 'sm' ? 'h-6 text-[12px]' : 'h-7 text-[12.5px]',
              tone === 'accent' &&
                'aria-checked:bg-accent aria-checked:text-accent-foreground dark:aria-checked:bg-accent',
            )}
          >
            {o.label}
          </ToggleGroup.Item>
        </Tooltip>
      ))}
    </ToggleGroup.Root>
  )
}
