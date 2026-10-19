import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

// The same "nothing here" panel for every list that can be empty: an icon in a
// soft circle, a headline, one line of guidance and, optionally, the action
// that fills the list.
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-6 py-16 text-center', className)}>
      {icon && (
        <div className="mb-4 flex size-12 items-center justify-center rounded-2xl bg-surface-3/70 text-muted-foreground [&_svg]:size-6">
          {icon}
        </div>
      )}
      <h3 className="text-[14px] font-semibold">{title}</h3>
      {description && <p className="mt-1 max-w-sm text-[13px] text-muted-foreground">{description}</p>}
      {action && <div className="mt-5 flex items-center gap-2">{action}</div>}
    </div>
  )
}
