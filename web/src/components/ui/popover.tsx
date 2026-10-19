import {Popover as P} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export const Popover = P.Root
export const PopoverTrigger = P.Trigger
export const PopoverAnchor = P.Anchor
export const PopoverClose = P.Close

export function PopoverContent({
  className,
  sideOffset = 6,
  align = 'center',
  ...props
}: ComponentProps<typeof P.Content>) {
  return (
    <P.Portal>
      <P.Content
        sideOffset={sideOffset}
        align={align}
        className={cn(
          'z-[60] w-72 animate-in rounded-lg border border-border bg-surface p-3 shadow-md outline-none data-[state=closed]:animate-out',
          className,
        )}
        {...props}
      />
    </P.Portal>
  )
}
