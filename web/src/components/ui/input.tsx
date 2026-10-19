import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export const inputClass =
  'w-full min-w-0 rounded-md border border-border bg-surface px-2.5 text-[13px] text-foreground shadow-xs transition-[border-color,box-shadow] outline-none placeholder:text-subtle-foreground focus-visible:border-accent focus-visible:ring-3 focus-visible:ring-accent/20 focus-visible:outline-none disabled:opacity-50 aria-invalid:border-danger aria-invalid:ring-danger/20'

export function Input({className, type = 'text', ...props}: ComponentProps<'input'>) {
  return <input type={type} className={cn(inputClass, 'h-8', className)} {...props} />
}

export function Textarea({className, ...props}: ComponentProps<'textarea'>) {
  return (
    <textarea className={cn(inputClass, 'min-h-20 resize-y py-2 leading-relaxed', className)} {...props} />
  )
}
