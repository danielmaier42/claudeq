import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Card({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('rounded-lg border border-border bg-surface shadow-xs', className)} {...props} />
}

export function CardHeader({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('flex items-start gap-3 px-4 pt-4 pb-2', className)} {...props} />
}

export function CardTitle({className, ...props}: ComponentProps<'h3'>) {
  return <h3 className={cn('text-[13.5px] leading-5 font-semibold', className)} {...props} />
}

export function CardDescription({className, ...props}: ComponentProps<'p'>) {
  return <p className={cn('text-[12.5px] text-muted-foreground', className)} {...props} />
}

export function CardContent({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('px-4 pb-4', className)} {...props} />
}

export function CardFooter({className, ...props}: ComponentProps<'div'>) {
  return (
    <div className={cn('flex items-center gap-2 border-t border-border px-4 py-3', className)} {...props} />
  )
}
