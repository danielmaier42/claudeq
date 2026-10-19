import {useSortable} from '@dnd-kit/sortable'
import {CSS} from '@dnd-kit/utilities'
import {Cpu, GripVertical, Pencil, Play, Workflow} from 'lucide-react'
import {useState, type ComponentProps, type CSSProperties, type Ref} from 'react'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Switch} from '@/components/ui/switch'
import {Tooltip} from '@/components/ui/tooltip'
import {openRunLog} from '@/features/log/store'
import {openTaskEditor} from '@/features/task-editor/store'
import type {QueueTask} from '@/lib/types'
import {cn} from '@/lib/utils'
import {runNow, setEnabled} from './actions'
import {DRAG_OFF} from './GroupSection'
import {lastOutcome, type Outcome, type OutcomeTone} from './outcome'
import {ScheduleText} from './ScheduleText'
import {STATUS_CHIPS, TaskChips, taskChips} from './TaskChips'
import {TaskMenu} from './TaskMenu'
import type {QueueContext} from './use-queue-context'

export interface TaskRowProps {
  task: QueueTask
  ctx: QueueContext
  paused: boolean
  groups: string[]
  prevId?: string
  nextId?: string
  dragDisabled?: boolean
}

// SortableTaskRow is a row the queue can drag: only the grip starts a drag, so
// selecting text and hitting the buttons still work.
export function SortableTaskRow(props: TaskRowProps) {
  const {setNodeRef, setActivatorNodeRef, attributes, listeners, transform, transition, isDragging} =
    useSortable({
      id: props.task.id,
      data: {kind: 'task', group: props.task.group || ''},
      disabled: props.dragDisabled,
    })
  return (
    <TaskRow
      {...props}
      rowRef={setNodeRef}
      style={{transform: CSS.Translate.toString(transform), transition}}
      dragging={isDragging}
      grip={{ref: setActivatorNodeRef, ...attributes, ...listeners}}
    />
  )
}

export function TaskRow({
  task: t,
  ctx,
  paused,
  groups,
  prevId,
  nextId,
  dragDisabled,
  rowRef,
  style,
  dragging,
  grip,
  overlay,
}: TaskRowProps & {
  rowRef?: Ref<HTMLDivElement>
  style?: CSSProperties
  dragging?: boolean
  grip?: ComponentProps<'button'> & {ref?: Ref<HTMLButtonElement>}
  // overlay is the copy that follows the pointer during a drag.
  overlay?: boolean
}) {
  // The hover actions stay put while the row's menu is open.
  const [menuOpen, setMenuOpen] = useState(false)
  const chips = taskChips(t, ctx)
  const status = chips.filter((c) => STATUS_CHIPS.has(c.key))
  const traits = chips.filter((c) => !STATUS_CHIPS.has(c.key))
  const canRun = !t.running && !paused
  const runTip = t.running
    ? 'Already running'
    : paused
      ? 'All runs are paused'
      : 'Start this task now, outside its schedule'
  const live = ctx.liveRun(t)
  const RunsOnIcon = t.pool ? Workflow : Cpu
  const outcome = lastOutcome(ctx.lastRun(t))

  return (
    <div
      ref={rowRef}
      style={style}
      className={cn(
        'group/row relative flex min-h-[52px] items-center gap-2 border-b border-border bg-surface py-2 pr-2 pl-1 last:border-b-0',
        'transition-colors focus-within:bg-surface-2 hover:bg-surface-2',
        dragging && 'z-10 opacity-40',
        overlay && 'rounded-lg border shadow-lg',
      )}
    >
      <Tooltip
        content={dragDisabled ? DRAG_OFF : 'Drag to reorder, or onto a group to move it there'}
        side="left"
      >
        {/* The hit area is wider than the icon, so the grip is easy to catch. */}
        <button
          type="button"
          aria-label={`Reorder ${t.name}`}
          aria-disabled={dragDisabled}
          {...grip}
          className={cn(
            'flex h-8 w-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-sm text-subtle-foreground opacity-50 outline-none group-hover/row:opacity-100 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-accent/40 active:cursor-grabbing aria-disabled:cursor-not-allowed',
            overlay && 'cursor-grabbing opacity-100',
          )}
        >
          <GripVertical className="size-4" />
        </button>
      </Tooltip>

      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <button
            type="button"
            onClick={() => openTaskEditor({mode: 'edit', task: t})}
            className={cn(
              'min-w-0 truncate rounded-sm text-left text-[13.5px] font-medium outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent/40',
              !t.enabled && 'text-muted-foreground',
            )}
          >
            {t.name}
          </button>
          {t.running && (
            <Tooltip content={live ? 'Open the live log' : 'Running now'}>
              <Badge
                tone="accent"
                dot
                pulse
                role={live ? 'button' : undefined}
                tabIndex={0}
                onClick={() => live && openRunLog(live)}
                onKeyDown={(e) => live && (e.key === 'Enter' || e.key === ' ') && openRunLog(live)}
                className={cn(
                  'outline-none focus-visible:ring-2 focus-visible:ring-accent/40',
                  live && 'cursor-pointer',
                )}
              >
                Running
              </Badge>
            </Tooltip>
          )}
          {!t.enabled && <Badge tone="outline">Paused</Badge>}
          <TaskChips chips={status} />
        </div>
        <div className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[12.5px] text-muted-foreground">
          <ScheduleText task={t} />
          {/* A script runs on no provider; its Script chip says so. */}
          {t.kind !== 'script' && (
            <>
              <span className="text-border-strong">·</span>
              <span className="inline-flex min-w-0 items-center gap-1">
                <RunsOnIcon className="size-3.5 shrink-0 text-subtle-foreground" />
                <span className="truncate">{ctx.runsOn(t)}</span>
              </span>
            </>
          )}
          {outcome && <LastOutcome outcome={outcome} />}
          {traits.length > 0 && (
            <span className="flex flex-wrap items-center gap-1">
              <TaskChips chips={traits} />
            </span>
          )}
        </div>
      </div>

      {!overlay && (
        <>
          <div
            className={cn(
              'flex items-center gap-0.5 opacity-0 transition-opacity group-hover/row:opacity-100 focus-within:opacity-100',
              menuOpen && 'opacity-100',
            )}
          >
            <Tooltip content={runTip}>
              {/* A disabled button gets no hover, so the span carries the tooltip. */}
              <span
                tabIndex={canRun ? -1 : 0}
                className="rounded-md outline-none focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                <Button variant="ghost" size="sm" disabled={!canRun} onClick={() => void runNow(t)}>
                  <Play className="size-3.5" /> Run now
                </Button>
              </span>
            </Tooltip>
            <Tooltip content="Edit">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Edit ${t.name}`}
                onClick={() => openTaskEditor({mode: 'edit', task: t})}
              >
                <Pencil />
              </Button>
            </Tooltip>
          </div>
          <Tooltip
            content={
              t.enabled
                ? 'On: runs on its schedule. Switch off to pause it.'
                : 'Paused: switch on to let it run again.'
            }
          >
            <span className="flex px-1.5">
              <Switch
                checked={t.enabled}
                aria-label={`${t.name} enabled`}
                onCheckedChange={(on) => setEnabled(t, on)}
              />
            </span>
          </Tooltip>
          <TaskMenu
            task={t}
            canRun={canRun}
            groups={groups}
            prevId={prevId}
            nextId={nextId}
            onOpenChange={setMenuOpen}
          />
        </>
      )}
    </div>
  )
}

const DOT: Record<OutcomeTone, string> = {
  success: 'bg-success',
  danger: 'bg-danger',
  warning: 'bg-warning',
  neutral: 'bg-subtle-foreground',
}

// LastOutcome says how the last run went and opens its log. A success stays
// in the meta line's grey; a failure takes the colour of its dot.
function LastOutcome({outcome: o}: {outcome: Outcome}) {
  return (
    <>
      <span className="text-border-strong">·</span>
      <Tooltip content="Open the log of the last run">
        <button
          type="button"
          onClick={() => openRunLog(o.runId)}
          className={cn(
            'inline-flex shrink-0 items-center gap-1.5 rounded-sm outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent/40',
            !o.quiet && (o.tone === 'danger' ? 'text-danger' : 'text-foreground'),
          )}
        >
          <span className={cn('size-1.5 shrink-0 rounded-full', DOT[o.tone])} aria-hidden />
          {o.text}
        </button>
      </Tooltip>
    </>
  )
}
