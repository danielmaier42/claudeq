import {Check, ChevronDown} from 'lucide-react'
import {Select as SelectPrimitive} from 'radix-ui'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

export interface SelectOption {
  value: string
  label: ReactNode
  description?: ReactNode
  disabled?: boolean
}

export interface SelectGroupDef {
  label?: ReactNode
  options: SelectOption[]
}

// Radix forbids an empty item value, so "" (the conventional "none / default"
// in this API) travels as a sentinel inside the menu and is mapped back here.
const EMPTY = '\u0000'
const enc = (v: string) => (v === '' ? EMPTY : v)
const dec = (v: string) => (v === EMPTY ? '' : v)

export function Select({
  value,
  onValueChange,
  options,
  groups,
  placeholder,
  disabled,
  className,
  size = 'md',
  'aria-label': ariaLabel,
  id,
}: {
  value: string
  onValueChange: (v: string) => void
  options?: SelectOption[]
  groups?: SelectGroupDef[]
  placeholder?: ReactNode
  disabled?: boolean
  className?: string
  size?: 'sm' | 'md'
  'aria-label'?: string
  id?: string
}) {
  const all = groups ?? [{options: options ?? []}]
  return (
    <SelectPrimitive.Root value={enc(value)} onValueChange={(v) => onValueChange(dec(v))} disabled={disabled}>
      <SelectPrimitive.Trigger
        id={id}
        aria-label={ariaLabel}
        className={cn(
          'inline-flex w-full min-w-0 items-center justify-between gap-2 rounded-md border border-border bg-surface px-2.5 text-left text-[13px] shadow-xs outline-none focus-visible:border-accent focus-visible:ring-3 focus-visible:ring-accent/20 disabled:opacity-50 data-[placeholder]:text-subtle-foreground',
          size === 'sm' ? 'h-7 text-[12.5px]' : 'h-8',
          className,
        )}
      >
        <span className="truncate">
          <SelectPrimitive.Value placeholder={placeholder} />
        </span>
        <SelectPrimitive.Icon>
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          position="popper"
          sideOffset={4}
          className="z-[60] max-h-[min(360px,var(--radix-select-content-available-height))] min-w-[var(--radix-select-trigger-width)] animate-in overflow-hidden rounded-lg border border-border bg-surface shadow-md data-[state=closed]:animate-out"
        >
          <SelectPrimitive.Viewport className="p-1">
            {all.map((g, gi) => (
              <SelectPrimitive.Group key={gi}>
                {g.label && (
                  <SelectPrimitive.Label className="px-2 pt-2 pb-1 text-[11px] font-semibold tracking-wide text-subtle-foreground uppercase">
                    {g.label}
                  </SelectPrimitive.Label>
                )}
                {g.options.map((o) => (
                  <SelectPrimitive.Item
                    key={o.value}
                    value={enc(o.value)}
                    disabled={o.disabled}
                    className="relative flex cursor-default items-center gap-2 rounded-md py-1.5 pr-8 pl-2 text-[13px] outline-none select-none data-[disabled]:opacity-50 data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
                  >
                    <div className="min-w-0">
                      <SelectPrimitive.ItemText>{o.label}</SelectPrimitive.ItemText>
                      {o.description && <div className="text-[11.5px] opacity-70">{o.description}</div>}
                    </div>
                    <SelectPrimitive.ItemIndicator className="absolute right-2">
                      <Check className="size-3.5" />
                    </SelectPrimitive.ItemIndicator>
                  </SelectPrimitive.Item>
                ))}
              </SelectPrimitive.Group>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}
