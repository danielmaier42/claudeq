import {cva, type VariantProps} from 'class-variance-authority'
import {AlertTriangle, CircleCheck, Info, OctagonAlert, Sparkles} from 'lucide-react'
import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

const alertVariants = cva('flex gap-3 rounded-lg border px-3.5 py-3 text-[13px]', {
  variants: {
    tone: {
      info: 'border-info/25 bg-info-soft',
      success: 'border-success/25 bg-success-soft',
      warning: 'border-warning/35 bg-warning-soft',
      danger: 'border-danger/25 bg-danger-soft',
      ai: 'border-ai/25 bg-ai-soft',
      neutral: 'border-border bg-surface-2',
    },
  },
  defaultVariants: {tone: 'info'},
})

const icons = {
  info: Info,
  success: CircleCheck,
  warning: AlertTriangle,
  danger: OctagonAlert,
  ai: Sparkles,
  neutral: Info,
}
const iconColor = {
  info: 'text-info',
  success: 'text-success',
  warning: 'text-[color-mix(in_oklab,var(--warning)_80%,var(--foreground))]',
  danger: 'text-danger',
  ai: 'text-ai',
  neutral: 'text-muted-foreground',
}

// Alert is an inline banner: a status (paused, limited, an update, a finding),
// what it means, and the action that resolves it.
export function Alert({
  tone = 'info',
  title,
  icon,
  actions,
  className,
  children,
}: VariantProps<typeof alertVariants> & {
  title?: ReactNode
  icon?: ReactNode
  actions?: ReactNode
  className?: string
  children?: ReactNode
}) {
  const t = tone ?? 'info'
  const Icon = icons[t]
  return (
    <div role="status" className={cn(alertVariants({tone: t}), className)}>
      <div className={cn('mt-0.5 shrink-0 [&_svg]:size-4', iconColor[t])}>{icon ?? <Icon />}</div>
      <div className="min-w-0 flex-1">
        {title && <div className="font-semibold">{title}</div>}
        {children && <div className={cn('text-foreground/85', title && 'mt-0.5')}>{children}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-start gap-2 self-center">{actions}</div>}
    </div>
  )
}
