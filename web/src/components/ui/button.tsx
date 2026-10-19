import {cva, type VariantProps} from 'class-variance-authority'
import {Slot} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'
import {Spinner} from './spinner'

export const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap transition-[background-color,box-shadow,color,opacity,transform] duration-150 select-none active:translate-y-px disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
  {
    variants: {
      variant: {
        primary:
          'bg-accent text-accent-foreground shadow-xs hover:bg-[color-mix(in_oklab,var(--accent)_88%,var(--foreground))]',
        secondary: 'border border-border bg-surface text-foreground shadow-xs hover:bg-surface-2',
        ghost: 'text-foreground hover:bg-surface-3/70',
        subtle: 'bg-surface-3/70 text-foreground hover:bg-surface-3',
        danger: 'bg-danger text-white shadow-xs hover:bg-[color-mix(in_oklab,var(--danger)_88%,black)]',
        'danger-ghost': 'text-danger hover:bg-danger-soft',
        ai: 'border border-ai/30 bg-ai-soft text-ai hover:bg-[color-mix(in_oklab,var(--ai)_20%,transparent)]',
        link: 'h-auto px-0 text-accent-text underline-offset-4 hover:underline',
      },
      size: {
        sm: 'h-7 px-2.5 text-[12.5px]',
        md: 'h-8 px-3 text-[13px]',
        lg: 'h-10 px-4 text-sm',
        icon: 'size-8',
        'icon-sm': 'size-7 [&_svg:not([class*=size-])]:size-3.5',
      },
    },
    defaultVariants: {variant: 'secondary', size: 'md'},
  },
)

export interface ButtonProps extends ComponentProps<'button'>, VariantProps<typeof buttonVariants> {
  asChild?: boolean
  loading?: boolean
}

export function Button({
  className,
  variant,
  size,
  asChild,
  loading,
  disabled,
  children,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot.Root : 'button'
  return (
    <Comp
      data-slot="button"
      className={cn(buttonVariants({variant, size}), className)}
      disabled={disabled || loading}
      {...props}
    >
      {loading && <Spinner />}
      {asChild ? children : <>{children}</>}
    </Comp>
  )
}
