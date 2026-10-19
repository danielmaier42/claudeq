import {Check, ChevronRight} from 'lucide-react'
import {DropdownMenu as M} from 'radix-ui'
import type {ComponentProps} from 'react'
import {cn} from '@/lib/utils'

export const DropdownMenu = M.Root
export const DropdownMenuTrigger = M.Trigger
export const DropdownMenuGroup = M.Group
export const DropdownMenuSub = M.Sub
export const DropdownMenuRadioGroup = M.RadioGroup

const content =
  'z-[60] min-w-44 animate-in overflow-hidden rounded-lg border border-border bg-surface p-1 shadow-md data-[state=closed]:animate-out'
const item =
  'relative flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-[13px] outline-none select-none data-[disabled]:pointer-events-none data-[disabled]:opacity-45 data-[highlighted]:bg-surface-3 [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-muted-foreground'

export function DropdownMenuContent({
  className,
  sideOffset = 4,
  align = 'end',
  ...props
}: ComponentProps<typeof M.Content>) {
  return (
    <M.Portal>
      <M.Content sideOffset={sideOffset} align={align} className={cn(content, className)} {...props} />
    </M.Portal>
  )
}

export function DropdownMenuItem({
  className,
  destructive,
  ...props
}: ComponentProps<typeof M.Item> & {destructive?: boolean}) {
  return (
    <M.Item
      className={cn(
        item,
        destructive && 'text-danger data-[highlighted]:bg-danger-soft [&_svg]:text-danger',
        className,
      )}
      {...props}
    />
  )
}

export function DropdownMenuCheckboxItem({
  className,
  children,
  ...props
}: ComponentProps<typeof M.CheckboxItem>) {
  return (
    <M.CheckboxItem className={cn(item, 'pl-7', className)} {...props}>
      <M.ItemIndicator className="absolute left-2">
        <Check className="size-3.5" />
      </M.ItemIndicator>
      {children}
    </M.CheckboxItem>
  )
}

export function DropdownMenuRadioItem({className, children, ...props}: ComponentProps<typeof M.RadioItem>) {
  return (
    <M.RadioItem className={cn(item, 'pl-7', className)} {...props}>
      <M.ItemIndicator className="absolute left-2">
        <Check className="size-3.5" />
      </M.ItemIndicator>
      {children}
    </M.RadioItem>
  )
}

export function DropdownMenuLabel({className, ...props}: ComponentProps<typeof M.Label>) {
  return (
    <M.Label
      className={cn(
        'px-2 pt-1.5 pb-1 text-[11px] font-semibold tracking-wide text-subtle-foreground uppercase',
        className,
      )}
      {...props}
    />
  )
}

export function DropdownMenuSeparator({className, ...props}: ComponentProps<typeof M.Separator>) {
  return <M.Separator className={cn('-mx-1 my-1 h-px bg-border', className)} {...props} />
}

export function DropdownMenuSubTrigger({className, children, ...props}: ComponentProps<typeof M.SubTrigger>) {
  return (
    <M.SubTrigger className={cn(item, 'data-[state=open]:bg-surface-3', className)} {...props}>
      {children}
      <ChevronRight className="ml-auto" />
    </M.SubTrigger>
  )
}

export function DropdownMenuSubContent({className, ...props}: ComponentProps<typeof M.SubContent>) {
  return (
    <M.Portal>
      <M.SubContent className={cn(content, className)} {...props} />
    </M.Portal>
  )
}

export function DropdownMenuShortcut({className, ...props}: ComponentProps<'span'>) {
  return <span className={cn('ml-auto pl-4 text-[11.5px] text-subtle-foreground', className)} {...props} />
}
