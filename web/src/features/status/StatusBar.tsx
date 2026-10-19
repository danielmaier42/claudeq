import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

export type StatusTone = 'warning' | 'info' | 'danger'

const toneClass: Record<StatusTone, string> = {
  warning: 'bg-warning-soft border-warning/30',
  info: 'bg-info-soft border-info/25',
  danger: 'bg-danger-soft border-danger/25',
}
const iconClass: Record<StatusTone, string> = {
  warning: 'text-[color-mix(in_oklab,var(--warning)_80%,var(--foreground))]',
  info: 'text-info',
  danger: 'text-danger',
}

// StatusBar is one slim, full-width strip above the open view: an icon, the
// headline, the explanation running on after it, and its action at the right.
// `extra` is a second row for what has to be seen whole (a command to copy).
export function StatusBar({
  tone,
  icon,
  title,
  children,
  actions,
  extra,
}: {
  tone: StatusTone
  icon: ReactNode
  title: ReactNode
  children?: ReactNode
  actions?: ReactNode
  extra?: ReactNode
}) {
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className={cn('animate-in border-b px-4 py-2 text-[12.5px] leading-[1.45]', toneClass[tone])}
    >
      <div className="flex items-center gap-3">
        <span className={cn('shrink-0 self-start pt-px [&_svg]:size-4', iconClass[tone])}>{icon}</span>
        <p className="min-w-0 flex-1 text-foreground/85">
          <span className="mr-1.5 font-semibold text-foreground">{title}</span>
          {children}
        </p>
        {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
      </div>
      {extra && <div className="mt-1.5 pl-7">{extra}</div>}
    </div>
  )
}
