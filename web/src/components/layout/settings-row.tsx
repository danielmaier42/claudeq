import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'

// RowGroup and Row are the grouped list of settings: a card of rows, each with
// a label and description on the left and its control on the right.
export function RowGroup({className, children}: {className?: string; children: ReactNode}) {
  return (
    <div
      className={cn(
        'divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface shadow-xs',
        className,
      )}
    >
      {children}
    </div>
  )
}

export function Row({
  label,
  description,
  control,
  children,
  className,
  htmlFor,
}: {
  label: ReactNode
  description?: ReactNode
  control?: ReactNode
  children?: ReactNode
  className?: string
  htmlFor?: string
}) {
  return (
    <div className={cn('px-4 py-3', className)}>
      <div className="flex items-center gap-4">
        <div className="min-w-0 flex-1">
          <label htmlFor={htmlFor} className="block text-[13px] font-medium">
            {label}
          </label>
          {description && <div className="mt-0.5 text-[12.5px] text-muted-foreground">{description}</div>}
        </div>
        {control && <div className="flex shrink-0 items-center gap-2">{control}</div>}
      </div>
      {children && <div className="mt-3">{children}</div>}
    </div>
  )
}
