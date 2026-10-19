import {X} from 'lucide-react'
import {Dialog as DialogPrimitive} from 'radix-ui'
import type {ComponentProps, ReactNode} from 'react'
import {cn} from '@/lib/utils'

export const Dialog = DialogPrimitive.Root
export const DialogTrigger = DialogPrimitive.Trigger
export const DialogClose = DialogPrimitive.Close

const sizes = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-4xl',
  full: 'max-w-[min(1200px,calc(100vw-48px))] h-[calc(100vh-48px)]',
}

export function DialogOverlay({className, ...props}: ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      className={cn(
        'fixed inset-0 z-50 bg-overlay data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in',
        className,
      )}
      {...props}
    />
  )
}

// DialogContent is a centred modal. The body scrolls on its own between a fixed
// header and footer, so a long form never pushes its buttons off screen.
export function DialogContent({
  className,
  size = 'md',
  children,
  hideClose,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content> & {size?: keyof typeof sizes; hideClose?: boolean}) {
  return (
    <DialogPrimitive.Portal>
      <DialogOverlay />
      <DialogPrimitive.Content
        className={cn(
          'fixed top-1/2 left-1/2 z-50 flex max-h-[calc(100vh-48px)] w-[calc(100vw-48px)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-xl border border-border bg-surface shadow-lg outline-none data-[state=closed]:animate-out data-[state=open]:animate-in',
          sizes[size],
          className,
        )}
        {...props}
      >
        {children}
        {!hideClose && (
          <DialogPrimitive.Close
            className="absolute top-3.5 right-3.5 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-3 hover:text-foreground"
            aria-label="Close"
          >
            <X className="size-4" />
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

export function DialogHeader({
  title,
  description,
  icon,
  className,
  children,
}: {
  title: ReactNode
  description?: ReactNode
  icon?: ReactNode
  className?: string
  children?: ReactNode
}) {
  return (
    <div
      className={cn(
        'flex shrink-0 items-start gap-3 border-b border-border px-5 pt-4 pr-12 pb-3.5',
        className,
      )}
    >
      {icon && (
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-text [&_svg]:size-4">
          {icon}
        </div>
      )}
      <div className="min-w-0 flex-1">
        <DialogPrimitive.Title className="text-[15px] leading-6 font-semibold">{title}</DialogPrimitive.Title>
        {description ? (
          <DialogPrimitive.Description className="text-[12.5px] text-muted-foreground">
            {description}
          </DialogPrimitive.Description>
        ) : (
          <DialogPrimitive.Description className="sr-only">{title}</DialogPrimitive.Description>
        )}
        {children}
      </div>
    </div>
  )
}

export function DialogBody({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('min-h-0 flex-1 overflow-y-auto px-5 py-4', className)} {...props} />
}

export function DialogFooter({className, ...props}: ComponentProps<'div'>) {
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
