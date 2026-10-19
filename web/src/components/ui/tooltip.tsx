import {Tooltip as TooltipPrimitive} from 'radix-ui'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

export const TooltipProvider = TooltipPrimitive.Provider

// Tooltip wraps one trigger. WKWebView never shows native title= tooltips, so
// every hover explanation in the app goes through this. An empty `content`
// renders the child alone.
export function Tooltip({
  content,
  children,
  side = 'top',
  align = 'center',
  className,
  delay,
}: {
  content: ReactNode
  children: ReactNode
  side?: 'top' | 'bottom' | 'left' | 'right'
  align?: 'start' | 'center' | 'end'
  className?: string
  delay?: number
}) {
  if (content === undefined || content === null || content === '' || content === false) return <>{children}</>
  return (
    <TooltipPrimitive.Root delayDuration={delay}>
      {/* Only keyboard focus opens a tooltip: a dialog that focuses its first
          control on open must not greet the user with a stray tooltip. */}
      <TooltipPrimitive.Trigger
        asChild
        onFocus={(e) => {
          if (!e.currentTarget.matches(':focus-visible')) e.preventDefault()
        }}
      >
        {children}
      </TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={8}
          className={cn(
            'z-[100] max-w-72 animate-in rounded-md border border-border bg-surface px-2.5 py-1.5 text-[12px] leading-snug whitespace-pre-line text-foreground shadow-md data-[state=closed]:animate-out',
            className,
          )}
        >
          {content}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}
