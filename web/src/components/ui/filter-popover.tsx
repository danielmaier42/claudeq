import {ListFilter} from 'lucide-react'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'
import {CountBadge} from './badge'
import {Button} from './button'
import {Field} from './field'
import {Popover, PopoverContent, PopoverTrigger} from './popover'
import {Select, type SelectOption} from './select'

// FilterPopover keeps a list's secondary filters behind one "Filter" button so
// the toolbar stays a single calm row. The button counts what is set inside;
// Reset clears exactly those filters, not the search or the quick switch
// beside the button.
export function FilterPopover({
  active,
  onReset,
  children,
  className,
}: {
  // How many filters inside the popover are set.
  active: number
  onReset: () => void
  children: ReactNode
  className?: string
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="secondary"
          className={cn(active > 0 && 'border-accent/40 text-accent-text', className)}
          aria-label={active ? `Filter, ${active} active` : 'Filter'}
        >
          <ListFilter />
          Filter
          <CountBadge count={active} className="-mr-1" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 p-0">
        <div className="flex items-center justify-between border-b border-border px-3 py-2">
          <span className="text-[13px] font-semibold">Filters</span>
          <Button variant="ghost" size="sm" className="-mr-1.5" disabled={!active} onClick={onReset}>
            Reset
          </Button>
        </div>
        <div className="flex flex-col gap-3.5 p-3">{children}</div>
      </PopoverContent>
    </Popover>
  )
}

// FilterSelect is one labelled menu inside a FilterPopover; "" is the "all"
// entry, named by `all`. A menu with a single entry has nothing to choose and
// stays out of the way unless it is set.
export function FilterSelect({
  label,
  hint,
  all,
  value,
  options,
  onChange,
}: {
  label: string
  hint?: ReactNode
  all: string
  value: string
  options: SelectOption[]
  onChange: (v: string) => void
}) {
  if (options.length < 2 && !value) return null
  return (
    <Field label={label} hint={hint}>
      <Select
        aria-label={label}
        value={value}
        onValueChange={onChange}
        options={[{value: '', label: all}, ...options]}
      />
    </Field>
  )
}

// optionLabel finds the label of a menu value, for its chip.
export function optionLabel(options: {value: string; label: ReactNode}[], value: string): ReactNode {
  return options.find((o) => o.value === value)?.label ?? value
}
