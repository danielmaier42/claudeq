import type {QueueTask} from '@/lib/types'

// Order is priority, and a group is a run of tasks that share a name: the queue
// renders the blocks in the order they first appear, so what the file says and
// what the list shows are the same thing. The ungrouped tasks are the block ''.
export interface Block {
  group: string
  tasks: QueueTask[]
}

export function buildBlocks(tasks: QueueTask[], onlyActive: boolean): Block[] {
  const blocks: Block[] = []
  const at = new Map<string, Block>()
  for (const t of tasks) {
    if (onlyActive && !t.enabled) continue
    const g = t.group || ''
    let b = at.get(g)
    if (!b) {
      b = {group: g, tasks: []}
      at.set(g, b)
      blocks.push(b)
    }
    b.tasks.push(t)
  }
  return blocks
}

// blockOrder lists the groups in the order they first appear, '' among them.
export function blockOrder(tasks: QueueTask[]): string[] {
  return [...new Set(tasks.map((t) => t.group || ''))]
}

// groupedOrder puts the tasks of one group together without reordering them
// among themselves, as the daemon does after every move (app.GroupedOrder).
export function groupedOrder(tasks: QueueTask[]): QueueTask[] {
  return blockOrder(tasks).flatMap((g) => tasks.filter((t) => (t.group || '') === g))
}

// Where a moved task lands: above or below another task, or behind the last
// task of a group (the end of the list for a group nobody is in yet).
export type Anchor = {before: string} | {after: string} | {endOf: string}

// placeTask is the queue after one task moved, in the daemon's order. The
// index the move endpoint wants is the task's position in the result: the
// position in the list once the dragged task has been taken out of it.
export function placeTask(tasks: QueueTask[], id: string, group: string, anchor: Anchor): QueueTask[] {
  const moved = tasks.find((t) => t.id === id)
  if (!moved) return tasks
  const rest = tasks.filter((t) => t.id !== id)
  let at = rest.length
  if ('before' in anchor) {
    const i = rest.findIndex((t) => t.id === anchor.before)
    if (i >= 0) at = i
  } else if ('after' in anchor) {
    const i = rest.findIndex((t) => t.id === anchor.after)
    if (i >= 0) at = i + 1
  } else {
    const i = rest.findLastIndex((t) => (t.group || '') === anchor.endOf)
    if (i >= 0) at = i + 1
  }
  rest.splice(at, 0, {...moved, group})
  return groupedOrder(rest)
}

// moveGroup is the queue after a whole section moved in front of another one,
// or to the end (before null), as POST /api/groups/move does it.
export function moveGroup(tasks: QueueTask[], name: string, before: string | null): QueueTask[] {
  if (before === name) return tasks
  const order = blockOrder(tasks).filter((g) => g !== name)
  const at = before === null ? -1 : order.indexOf(before)
  order.splice(at < 0 ? order.length : at, 0, name)
  return order.flatMap((g) => tasks.filter((t) => (t.group || '') === g))
}

// sameOrder tells whether a drop changed anything worth a request.
export function sameOrder(a: QueueTask[], b: QueueTask[]): boolean {
  return (
    a.length === b.length && a.every((t, i) => t.id === b[i].id && (t.group || '') === (b[i].group || ''))
  )
}

// waitedFor is how long a held task has been due, as "12 min" or "2 h", or
// nothing for the first minute.
export function waitedFor(since: string, now = Date.now()): string {
  const m = Math.floor((now - new Date(since).getTime()) / 60000)
  if (m < 1) return ''
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h`
}
