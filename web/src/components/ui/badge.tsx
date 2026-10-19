import {cva, type VariantProps} from 'class-variance-authority'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export const badgeVariants = cva(
  'inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] leading-4 font-semibold whitespace-nowrap [&_svg]:size-3',
  {
    variants: {
      tone: {
        neutral: 'bg-surface-3 text-muted-foreground',
        accent: 'bg-accent-soft text-accent-text',
        success: 'bg-success-soft text-success',
        warning: 'bg-warning-soft text-[color-mix(in_oklab,var(--warning)_75%,var(--foreground))]',
        danger: 'bg-danger-soft text-danger',
        info: 'bg-info-soft text-info',
        ai: 'bg-ai-soft text-ai',
        outline: 'border border-border text-muted-foreground',
      },
    },
    defaultVariants: {tone: 'neutral'},
  },
)

export type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>['tone']>

export interface BadgeProps extends ComponentProps<'span'>, VariantProps<typeof badgeVariants> {
  // dot puts a status dot in front; pulse makes it breathe (something running).
  dot?: boolean
  pulse?: boolean
}

export function Badge({className, tone, dot, pulse, children, ...props}: BadgeProps) {
  return (
    <span className={cn(badgeVariants({tone}), className)} {...props}>
      {dot && <span className={cn('size-1.5 rounded-full bg-current', pulse && 'animate-pulse-dot')} />}
      {children}
    </span>
  )
}

// CountBadge is the small number next to a sidebar entry or a tab.
export function CountBadge({count, className}: {count: number; className?: string}) {
  if (!count) return null
  return (
    <span
      className={cn(
        'inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-accent px-1.5 text-[11px] font-semibold text-accent-foreground tabular-nums',
        className,
      )}
    >
      {count > 99 ? '99+' : count}
    </span>
  )
}
