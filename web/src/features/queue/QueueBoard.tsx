import {
  closestCenter,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MeasuringStrategy,
  PointerSensor,
  pointerWithin,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import {ChevronRight, FolderMinus, FolderPlus} from 'lucide-react'
import {useMemo, useState} from 'react'
import type {QueueTask} from '@/lib/types'
import {commitGroupMove, commitOrder, moveToNewGroup} from './actions'
import {DropZone, GroupSection, sectionId} from './GroupSection'
import {buildBlocks, placeTask, type Anchor} from './queue-model'
import {SortableTaskRow, TaskRow} from './TaskRow'
import type {QueueContext} from './use-queue-context'

const ZONE_UNGROUP = 'zone:ungroup'
const ZONE_NEW = 'zone:new'

type Kind = 'task' | 'group' | 'header' | 'zone'
const kindOf = (d: {data: {current?: Record<string, unknown>}} | null | undefined) =>
  d?.data.current?.kind as Kind | undefined

// A dragged task aims at the header or tile under the pointer first, then at
// the nearest row; a dragged section only ever aims at other sections.
const collide: CollisionDetection = (args) => {
  const only = (...kinds: Kind[]) => args.droppableContainers.filter((c) => kinds.includes(kindOf(c) as Kind))
  if (kindOf(args.active) === 'group') {
    const sections = only('group')
    const hit = pointerWithin({...args, droppableContainers: sections})
    return hit.length ? hit : closestCenter({...args, droppableContainers: sections})
  }
  const hit = pointerWithin({...args, droppableContainers: only('header', 'zone')})
  return hit.length ? hit : closestCenter({...args, droppableContainers: only('task')})
}

/* The gesture writes one request: the task's group and its new position go to
   the daemon together. While a drag is on, the board draws a draft of the
   queue, so a row that crosses into another group makes room there at once and
   a poll landing meanwhile cannot pull the row out from under the pointer. */
export function QueueBoard({
  tasks,
  onlyActive,
  searching,
  collapsed,
  groups,
  ctx,
  paused,
}: {
  tasks: QueueTask[]
  onlyActive: boolean
  // While a search hides rows, a drop could not say where between the hidden
  // ones a task belongs, so dragging is off.
  searching: boolean
  collapsed: (group: string) => boolean
  groups: string[]
  ctx: QueueContext
  paused: boolean
}) {
  const [draft, setDraft] = useState<QueueTask[] | null>(null)
  const [activeTask, setActiveTask] = useState<QueueTask | null>(null)
  const [activeGroup, setActiveGroup] = useState<string | null>(null)
  const list = draft ?? tasks
  const blocks = useMemo(() => buildBlocks(list, onlyActive), [list, onlyActive])

  const sensors = useSensors(
    useSensor(PointerSensor, {activationConstraint: {distance: 4}}),
    useSensor(KeyboardSensor, {coordinateGetter: sortableKeyboardCoordinates}),
  )

  const reset = () => {
    setDraft(null)
    setActiveTask(null)
    setActiveGroup(null)
  }

  const onDragStart = ({active}: DragStartEvent) => {
    if (kindOf(active) === 'group') setActiveGroup(active.data.current?.group as string)
    else {
      setActiveTask(tasks.find((t) => t.id === active.id) ?? null)
      setDraft(tasks)
    }
  }

  // Crossing into another group moves the row there in the draft; inside its
  // own group the sortable list makes room by itself.
  const onDragOver = ({active, over}: DragOverEvent) => {
    if (kindOf(active) !== 'task' || kindOf(over) !== 'task' || !over || over.id === active.id) return
    setDraft((d) => {
      if (!d) return d
      const me = d.find((t) => t.id === active.id)
      const target = d.find((t) => t.id === over.id)
      if (!me || !target || (me.group || '') === (target.group || '')) return d
      const r = active.rect.current.translated
      const below = !!r && r.top + r.height / 2 > over.rect.top + over.rect.height / 2
      return placeTask(d, me.id, target.group || '', below ? {after: target.id} : {before: target.id})
    })
  }

  const onDragEnd = ({active, over}: DragEndEvent) => {
    const d = draft
    if (kindOf(active) === 'group') {
      const name = activeGroup
      reset()
      if (name === null || !over || over.id === active.id) return
      const order = blocks.map((b) => sectionId(b.group))
      const moved = arrayMove(order, order.indexOf(String(active.id)), order.indexOf(String(over.id)))
      const next = moved[moved.indexOf(sectionId(name)) + 1]
      commitGroupMove(name, next === undefined ? null : next.slice(2))
      return
    }
    reset()
    const id = String(active.id)
    if (!over || !d) return
    if (over.id === ZONE_NEW) return void moveToNewGroup(id)
    const me = d.find((t) => t.id === id)
    if (!me) return
    let group = me.group || ''
    let anchor: Anchor | null = null
    if (over.id === ZONE_UNGROUP) [group, anchor] = ['', {endOf: ''}]
    else if (kindOf(over) === 'header') {
      group = over.data.current?.group as string
      anchor = {endOf: group}
    } else if (kindOf(over) === 'task' && over.id !== id) {
      // Within one group: where the sortable list showed the row, i.e. behind
      // the row it passed going down, in front of it going up.
      const ids = blocks.find((b) => b.group === group)?.tasks.map((t) => t.id) ?? []
      const from = ids.indexOf(id)
      const to = ids.indexOf(String(over.id))
      if (from >= 0 && to >= 0) anchor = from < to ? {after: String(over.id)} : {before: String(over.id)}
    }
    commitOrder(anchor ? placeTask(d, id, group, anchor) : d, id)
  }

  const visibleGroups = new Set(blocks.map((b) => b.group))

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collide}
      // The drop tiles appear only once a drag has started, so they must be
      // measured while it runs.
      measuring={{droppable: {strategy: MeasuringStrategy.Always}}}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDragCancel={reset}
      accessibility={{
        screenReaderInstructions: {
          draggable:
            'To pick up a task or group, press space or enter. Use the arrow keys to move it, space or enter to drop it, escape to cancel.',
        },
      }}
    >
      <SortableContext items={blocks.map((b) => sectionId(b.group))} strategy={verticalListSortingStrategy}>
        {blocks.map((b) => (
          <GroupSection
            key={b.group}
            block={b}
            collapsed={!!b.group && collapsed(b.group)}
            groups={groups}
            dragDisabled={searching}
          >
            {b.tasks.map((t, i) => (
              <SortableTaskRow
                key={t.id}
                task={t}
                ctx={ctx}
                paused={paused}
                groups={groups}
                prevId={b.tasks[i - 1]?.id}
                nextId={b.tasks[i + 1]?.id}
                dragDisabled={searching}
              />
            ))}
          </GroupSection>
        ))}
      </SortableContext>

      {activeTask && (
        <>
          {/* Dragging a task onto these makes it leave its group, or puts it in
              one that did not exist a moment ago. */}
          {!visibleGroups.has('') && (
            <DropZone id={ZONE_UNGROUP}>
              <FolderMinus className="mr-1.5 inline size-4 align-[-3px]" />
              Drop here to take the task out of its group
            </DropZone>
          )}
          <DropZone id={ZONE_NEW}>
            <FolderPlus className="mr-1.5 inline size-4 align-[-3px]" />
            Drop here to start a new group
          </DropZone>
        </>
      )}

      <DragOverlay dropAnimation={{duration: 160, easing: 'cubic-bezier(.2,.8,.2,1)'}}>
        {activeTask ? (
          <TaskRow task={activeTask} ctx={ctx} paused={paused} groups={groups} overlay />
        ) : activeGroup !== null ? (
          <div className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-surface px-2.5 text-[12px] font-semibold tracking-wide text-foreground uppercase shadow-lg">
            <ChevronRight className="size-3.5" />
            {activeGroup}
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  )
}
