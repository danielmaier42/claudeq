import {useDndContext, useDroppable} from '@dnd-kit/core'
import {SortableContext, useSortable, verticalListSortingStrategy} from '@dnd-kit/sortable'
import {CSS} from '@dnd-kit/utilities'
import {ChevronRight, GripVertical, Pencil} from 'lucide-react'
import type {ComponentProps, ReactNode, Ref} from 'react'
import {Button} from '@/components/ui/button'
import {Tooltip} from '@/components/ui/tooltip'
import {cn} from '@/lib/utils'
import {renameGroup, setCollapsed} from './actions'
import type {Block} from './queue-model'

export const DRAG_OFF = 'Clear the search to reorder: with rows hidden, a drop has no clear place'

export const sectionId = (group: string) => 'g:' + group
export const headerId = (group: string) => 'h:' + group

// GroupSection is one block of the queue: a named group with a header that
// folds it and drags the whole section, or the ungrouped tasks, which need no
// header at all.
export function GroupSection({
  block,
  collapsed,
  groups,
  dragDisabled,
  children,
}: {
  block: Block
  collapsed: boolean
  groups: string[]
  dragDisabled?: boolean
  children: ReactNode
}) {
  const named = block.group !== ''
  const {setNodeRef, setActivatorNodeRef, attributes, listeners, transform, transition, isDragging} =
    useSortable({
      id: sectionId(block.group),
      data: {kind: 'group', group: block.group},
      // The ungrouped tasks cannot be dragged as a section, but a section can be
      // dropped around them.
      disabled: {draggable: !named || !!dragDisabled, droppable: false},
    })
  return (
    <section
      ref={setNodeRef}
      style={{transform: CSS.Translate.toString(transform), transition}}
      className={cn('mb-5 last:mb-0', isDragging && 'relative z-10 opacity-40')}
    >
      {named && (
        <GroupHeader
          block={block}
          collapsed={collapsed}
          groups={groups}
          dragDisabled={dragDisabled}
          grip={{ref: setActivatorNodeRef, ...attributes, ...listeners}}
        />
      )}
      {!(named && collapsed) && (
        <SortableContext
          id={sectionId(block.group)}
          items={block.tasks.map((t) => t.id)}
          strategy={verticalListSortingStrategy}
        >
          <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
            {children}
          </div>
        </SortableContext>
      )}
    </section>
  )
}

function GroupHeader({
  block,
  collapsed,
  groups,
  dragDisabled,
  grip,
}: {
  block: Block
  collapsed: boolean
  groups: string[]
  dragDisabled?: boolean
  grip: ComponentProps<'button'> & {ref?: Ref<HTMLButtonElement>}
}) {
  const name = block.group
  // A group takes a task dropped anywhere on its header, folded or not; the
  // task lands at the end of the group.
  const {setNodeRef, isOver} = useDroppable({id: headerId(name), data: {kind: 'header', group: name}})
  const {active} = useDndContext()
  const taskOver = isOver && active?.data.current?.kind === 'task'
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'group/hd -mx-1 mb-1.5 flex h-8 items-center gap-1 rounded-md px-1 text-[12px] font-semibold tracking-wide text-muted-foreground uppercase transition-colors',
        taskOver && 'bg-accent-soft text-accent-text ring-1 ring-accent/40',
      )}
    >
      <Tooltip content={dragDisabled ? DRAG_OFF : 'Drag to move the whole group'} side="left">
        <button
          type="button"
          aria-label={`Move the group ${name}`}
          aria-disabled={dragDisabled}
          {...grip}
          className="flex h-6 w-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-sm text-subtle-foreground opacity-0 outline-none group-hover/hd:opacity-100 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-accent/40 active:cursor-grabbing aria-disabled:cursor-not-allowed"
        >
          <GripVertical className="size-3.5" />
        </button>
      </Tooltip>
      <Tooltip content={`${collapsed ? 'Show' : 'Hide'} the tasks in “${name}”`} align="start">
        <button
          type="button"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed(name, !collapsed)}
          className="flex min-w-0 items-center gap-1.5 rounded-sm py-1 pr-1 uppercase outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40"
        >
          <ChevronRight
            className={cn('size-3.5 shrink-0 transition-transform duration-150', !collapsed && 'rotate-90')}
          />
          <span className="truncate">{name}</span>
          <span className="rounded-full bg-surface-3 px-1.5 text-[11px] leading-4 font-semibold tracking-normal text-muted-foreground normal-case tabular-nums">
            {block.tasks.length}
          </span>
        </button>
      </Tooltip>
      <Tooltip content="Rename this group">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Rename the group ${name}`}
          onClick={() => void renameGroup(name, groups)}
          className="size-6 opacity-0 group-hover/hd:opacity-100 focus-visible:opacity-100"
        >
          <Pencil />
        </Button>
      </Tooltip>
    </div>
  )
}

// DropZone is a place to drop with nothing in it: the tile that takes a task out
// of every group, and the one that starts a new group. It only takes up room
// while a task is being dragged.
export function DropZone({id, children}: {id: string; children: ReactNode}) {
  const {setNodeRef, isOver} = useDroppable({id, data: {kind: 'zone'}})
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'mt-3 animate-in rounded-lg border-[1.5px] border-dashed border-border-strong px-4 py-3.5 text-center text-[13px] text-muted-foreground transition-colors',
        isOver && 'border-accent bg-accent-soft text-accent-text',
      )}
    >
      {children}
    </div>
  )
}
