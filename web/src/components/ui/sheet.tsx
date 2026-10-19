import {X} from 'lucide-react'
import {Dialog as DialogPrimitive} from 'radix-ui'
import type {ComponentProps, ReactNode} from 'react'
import {cn} from '@/lib/utils'
import {DialogOverlay} from './dialog'

// Sheet is a panel sliding in from the right edge: for editors and detail views
// that benefit from the list staying visible behind them.
export const Sheet = DialogPrimitive.Root
export const SheetClose = DialogPrimitive.Close

const widths = {md: 'w-[min(520px,100vw)]', lg: 'w-[min(720px,100vw)]', xl: 'w-[min(960px,100vw)]'}

export function SheetContent({
  className,
  width = 'lg',
  children,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content> & {width?: keyof typeof widths}) {
  return (
    <DialogPrimitive.Portal>
      <DialogOverlay />
      <DialogPrimitive.Content
        className={cn(
          'fixed inset-y-2 right-2 z-50 flex flex-col overflow-hidden rounded-xl border border-border bg-surface shadow-lg outline-none data-[state=closed]:animate-slide-out data-[state=open]:animate-slide-in',
          widths[width],
          className,
        )}
        {...props}
      >
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

export function SheetHeader({
  title,
  description,
  actions,
  className,
  children,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
  children?: ReactNode
}) {
  return (
    <div className={cn('shrink-0 border-b border-border px-5 py-3.5', className)}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <DialogPrimitive.Title className="truncate text-[15px] leading-6 font-semibold">
            {title}
          </DialogPrimitive.Title>
          {description ? (
            <DialogPrimitive.Description className="text-[12.5px] text-muted-foreground">
              {description}
            </DialogPrimitive.Description>
          ) : (
            <DialogPrimitive.Description className="sr-only">{title}</DialogPrimitive.Description>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {actions}
          <DialogPrimitive.Close
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-3 hover:text-foreground"
            aria-label="Close"
          >
            <X className="size-4" />
          </DialogPrimitive.Close>
        </div>
      </div>
      {children}
    </div>
  )
}

export function SheetBody({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('min-h-0 flex-1 overflow-y-auto px-5 py-4', className)} {...props} />
}

export function SheetFooter({className, ...props}: ComponentProps<'div'>) {
  return (
    <div
      className={cn(
        'flex shrink-0 items-center justify-end gap-2 border-t border-border bg-surface-2/60 px-5 py-3',
        className,
      )}
      {...props}
    />
  )
}
