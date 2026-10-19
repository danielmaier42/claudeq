import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Skeleton({className, ...props}: ComponentProps<'div'>) {
  return <div className={cn('animate-pulse rounded-md bg-surface-3', className)} {...props} />
}
