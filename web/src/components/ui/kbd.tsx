import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Kbd({className, ...props}: ComponentProps<'kbd'>) {
  return (
    <kbd
      className={cn(
        'inline-flex h-5 min-w-5 items-center justify-center rounded border border-border bg-surface-2 px-1 font-sans text-[11px] font-medium text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}
