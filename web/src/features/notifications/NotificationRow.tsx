import {
  Bell,
  ChevronRight,
  CircleCheck,
  CircleX,
  Eye,
  FileText,
  MessageSquareText,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Tooltip} from '@/components/ui/tooltip'
import {fullTime, relTime} from '@/lib/format'
import type {AppNotification} from '@/lib/types'
import {cn} from '@/lib/utils'
import {kindLabel, shortTitle, targetLabel} from './logic'
import {markNotificationRead, openTarget} from './open-target'

// The icon says at a glance what kind of notification a row is.
const KIND_LOOK: Record<string, {icon: LucideIcon; tone: string}> = {
  success: {icon: CircleCheck, tone: 'bg-success-soft text-success'},
  failure: {icon: CircleX, tone: 'bg-danger-soft text-danger'},
  artifact: {icon: FileText, tone: 'bg-accent-soft text-accent-text'},
  provider: {icon: TriangleAlert, tone: 'bg-warning-soft text-warning'},
  task: {icon: MessageSquareText, tone: 'bg-info-soft text-info'},
}
const OTHER = {icon: Bell, tone: 'bg-surface-3 text-muted-foreground'}

// A row does what clicking the macOS notification does: it turns read and what
// it is about opens. The read mark is shown at once, and the target is not
// kept waiting for the daemon's answer.
function openNotification(n: AppNotification) {
  if (n.unread) void markNotificationRead(n.id)
  openTarget(n)
}

// One notification as one row: the unread dot, the kind's icon, the title with
// the message under it, when it was sent and where a click leads; the
// mark-read eye shows while the pointer or focus is on the row.
export function NotificationRow({n}: {n: AppNotification}) {
  const {icon: Icon, tone} = KIND_LOOK[n.kind] ?? OTHER
  const target = targetLabel(n)
  return (
    <div
      role="button"
      tabIndex={0}
      data-notification-row
      onClick={() => openNotification(n)}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget || (e.key !== 'Enter' && e.key !== ' ')) return
        e.preventDefault()
        openNotification(n)
      }}
      className="group relative flex cursor-pointer items-start gap-3 px-3 py-2.5 transition-colors outline-none hover:bg-surface-2 focus-visible:bg-surface-2 focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-inset"
    >
      <div className="flex w-2 shrink-0 justify-center pt-[11px]">
        {n.unread && <span aria-label="Unread" className="size-2 rounded-full bg-accent" />}
      </div>
      <Tooltip content={kindLabel(n.kind)}>
        <div
          className={cn(
            'mt-px flex size-7 shrink-0 items-center justify-center rounded-lg [&_svg]:size-3.5',
            tone,
          )}
        >
          <Icon />
        </div>
      </Tooltip>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span
            className={cn(
              'min-w-0 truncate text-[13px] leading-5',
              n.unread ? 'font-semibold' : 'font-medium',
            )}
          >
            {shortTitle(n.title)}
          </span>
          {n.task_name && (
            <span className="hidden min-w-0 shrink truncate text-[12px] text-subtle-foreground sm:inline">
              {n.task_name}
            </span>
          )}
        </div>
        {/* The message as the notification showed it, line breaks kept, cut
            after a few lines so one long-winded task does not take the page. */}
        {n.message && (
          <p className="line-clamp-4 text-[12.5px] leading-[18px] whitespace-pre-line text-muted-foreground">
            {n.message}
          </p>
        )}
      </div>
      <div className="flex h-7 shrink-0 items-center gap-1">
        <div className="flex items-center opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
          {n.unread && (
            <Tooltip content="Mark read">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Mark read"
                onClick={(e) => {
                  e.stopPropagation()
                  void markNotificationRead(n.id)
                }}
              >
                <Eye />
              </Button>
            </Tooltip>
          )}
        </div>
        <Tooltip content={fullTime(n.sent_at)}>
          <span className="w-16 text-right text-[12px] text-subtle-foreground tabular-nums">
            {relTime(n.sent_at)}
          </span>
        </Tooltip>
        {target ? (
          <Tooltip content={target}>
            <span className="flex size-5 items-center justify-center text-subtle-foreground opacity-50 transition-opacity group-hover:opacity-100">
              <ChevronRight className="size-4" />
            </span>
          </Tooltip>
        ) : (
          <span aria-hidden className="size-5" />
        )}
      </div>
    </div>
  )
}
