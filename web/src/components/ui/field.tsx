import type {ReactNode} from 'react'
import {cn} from '@/lib/utils'
import {Label} from './label'

// Field is one labelled form control: the label, the control, then either the
// error (when there is one) or the hint below it.
export function Field({
  label,
  htmlFor,
  hint,
  error,
  aside,
  className,
  children,
}: {
  label?: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  aside?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      {(label || aside) && (
        <div className="flex min-h-4 items-center justify-between gap-2">
          {label && <Label htmlFor={htmlFor}>{label}</Label>}
          {aside}
        </div>
      )}
      {children}
      {error ? (
        <p className="text-[12px] text-danger">{error}</p>
      ) : hint ? (
        <p className="text-[12px] text-muted-foreground">{hint}</p>
      ) : null}
    </div>
  )
}
