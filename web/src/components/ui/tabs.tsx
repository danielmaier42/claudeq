import {Tabs as T} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export const Tabs = T.Root

// An underline tab strip, for switching sections inside one view.
export function TabsList({className, ...props}: ComponentProps<typeof T.List>) {
  return <T.List className={cn('flex items-center gap-1 border-b border-border', className)} {...props} />
}

export function TabsTrigger({className, ...props}: ComponentProps<typeof T.Trigger>) {
  return (
    <T.Trigger
      className={cn(
        'relative -mb-px inline-flex h-9 items-center gap-1.5 border-b-2 border-transparent px-2.5 text-[13px] font-medium text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:text-foreground data-[state=active]:border-accent data-[state=active]:text-foreground [&_svg]:size-4',
        className,
      )}
      {...props}
    />
  )
}

export function TabsContent({className, ...props}: ComponentProps<typeof T.Content>) {
  return <T.Content className={cn('outline-none', className)} {...props} />
}
