import {Switch as SwitchPrimitive} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Switch({className, ...props}: ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      className={cn(
        'peer inline-flex h-[20px] w-[34px] shrink-0 items-center rounded-full border border-transparent bg-surface-3 shadow-inner transition-colors outline-none focus-visible:ring-3 focus-visible:ring-accent/25 disabled:opacity-50 data-[state=checked]:bg-accent dark:bg-border-strong',
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="pointer-events-none block size-4 translate-x-[1px] rounded-full bg-white shadow-sm ring-0 transition-transform duration-200 data-[state=checked]:translate-x-[15px]" />
    </SwitchPrimitive.Root>
  )
}
