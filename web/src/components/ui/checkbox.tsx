import {Check} from 'lucide-react'
import {Checkbox as CheckboxPrimitive} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Checkbox({className, ...props}: ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      className={cn(
        'peer size-4 shrink-0 rounded-[5px] border border-border-strong bg-surface shadow-xs outline-none focus-visible:ring-3 focus-visible:ring-accent/25 disabled:opacity-50 data-[state=checked]:border-accent data-[state=checked]:bg-accent data-[state=checked]:text-accent-foreground',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator className="flex items-center justify-center">
        <Check className="size-3" strokeWidth={3} />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}
