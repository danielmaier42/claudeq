import {Slider as S} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export function Slider({className, ...props}: ComponentProps<typeof S.Root>) {
  return (
    <S.Root
      className={cn('relative flex h-5 w-full touch-none items-center select-none', className)}
      {...props}
    >
      <S.Track className="relative h-1.5 grow overflow-hidden rounded-full bg-surface-3">
        <S.Range className="absolute h-full bg-accent" />
      </S.Track>
      {(props.value ?? props.defaultValue ?? [0]).map((_, i) => (
        <S.Thumb
          key={i}
          className="block size-4 rounded-full border border-border-strong bg-white shadow-sm transition-shadow outline-none focus-visible:ring-4 focus-visible:ring-accent/25"
        />
      ))}
    </S.Root>
  )
}
