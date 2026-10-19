import {ChevronRight} from 'lucide-react'
import type {ReactNode} from 'react'
import {Card} from '@/components/ui/card'
import {Skeleton} from '@/components/ui/skeleton'
import {cn} from '@/lib/utils'

// OverviewCard is one of the Dashboard's summary tiles: a heading with a count,
// a few rows, and a link into the view that has the full list.
export function OverviewCard({
  icon,
  title,
  count,
  link,
  onLink,
  loading,
  children,
}: {
  icon: ReactNode
  title: string
  count?: ReactNode
  link: string
  onLink: () => void
  loading?: boolean
  children: ReactNode
}) {
  return (
    <Card className="flex flex-col">
      <div className="flex items-center gap-2 px-4 pt-3.5 pb-2">
        <span className="text-muted-foreground [&_svg]:size-4">{icon}</span>
        <h3 className="text-[13px] font-semibold">{title}</h3>
        {count}
        <button
          type="button"
          onClick={onLink}
          className="ml-auto inline-flex items-center gap-0.5 rounded-sm text-[12px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40"
        >
          {link}
          <ChevronRight className="size-3.5" />
        </button>
      </div>
      <div className="flex flex-col px-2 pb-2">
        {loading ? (
          <div className="space-y-2 px-2 pt-1">
            <Skeleton className="h-9" />
            <Skeleton className="h-9" />
          </div>
        ) : (
          children
        )}
      </div>
    </Card>
  )
}

// OverviewRow is one entry in a tile; clicking opens it where it lives.
export function OverviewRow({
  lead,
  title,
  meta,
  trail,
  onClick,
}: {
  lead?: ReactNode
  title: ReactNode
  meta?: ReactNode
  trail?: ReactNode
  onClick?: () => void
}) {
  const Comp = onClick ? 'button' : 'div'
  return (
    <Comp
      type={onClick ? 'button' : undefined}
      onClick={onClick}
      className={cn(
        'flex w-full min-w-0 items-center gap-2.5 rounded-md px-2 py-1.5 text-left',
        onClick && 'outline-none hover:bg-surface-2 focus-visible:ring-2 focus-visible:ring-accent/40',
      )}
    >
      {lead && <span className="flex size-4 shrink-0 items-center justify-center">{lead}</span>}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13px] font-medium">{title}</span>
        {meta && <span className="block truncate text-[12px] text-muted-foreground">{meta}</span>}
      </span>
      {trail && <span className="shrink-0 text-[12px] text-muted-foreground tabular-nums">{trail}</span>}
    </Comp>
  )
}

// OverviewEmpty is a tile's quiet line when it has nothing to list: one line,
// so an empty tile takes no more room than its heading needs.
export function OverviewEmpty({
  icon,
  children,
  action,
}: {
  icon?: ReactNode
  children: ReactNode
  action?: ReactNode
}) {
  return (
    <div className="flex min-h-8 items-center gap-2 px-2 text-[12.5px] text-muted-foreground">
      {icon && <span className="flex text-subtle-foreground [&_svg]:size-4">{icon}</span>}
      <span className="min-w-0 flex-1">{children}</span>
      {action}
    </div>
  )
}
