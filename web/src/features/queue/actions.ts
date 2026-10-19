import {setPaused} from '@/features/settings/actions'
import {confirm, promptText} from '@/components/ui/confirm'
import {toast} from '@/components/ui/toaster'
import {navigate} from '@/app/nav'
import {openRunLog} from '@/features/log/store'
import {ApiError, del, errorMessage, get, post} from '@/lib/api'
import {invalidate, keys, queryClient} from '@/lib/queries'
import type {Group, PathResult, QueueTask, Run} from '@/lib/types'
import {moveGroup, placeTask, sameOrder, type Anchor} from './queue-model'

const tasksNow = () => queryClient.getQueryData<QueueTask[]>(keys.tasks) ?? []

// optimistic shows a change at once and writes it behind: a poll in flight is
// cancelled so it cannot paint the old order back, and a failed write puts the
// list back the way the daemon still has it. The cache changes synchronously,
// so a drop never shows the old order for a frame.
async function optimistic<T>(
  key: readonly unknown[],
  next: (prev: T | undefined) => T | undefined,
  write: () => Promise<unknown>,
  failure: string,
) {
  void queryClient.cancelQueries({queryKey: key})
  const prev = queryClient.getQueryData<T>(key)
  queryClient.setQueryData<T>(key, next(prev))
  try {
    await write()
    return true
  } catch (e) {
    queryClient.setQueryData<T>(key, prev)
    toast.error(failure, {description: errorMessage(e)})
    return false
  } finally {
    void invalidate(keys.tasks, keys.groups)
  }
}

const moveURL = (id: string, to: number, group: string) =>
  `/api/tasks/${encodeURIComponent(id)}/move?to=${to}&group=${encodeURIComponent(group)}`

// commitOrder writes a queue the UI already rearranged: the task's group and
// its new position go to the daemon together, so a drop can never half-apply.
export function commitOrder(next: QueueTask[], id: string) {
  const prev = tasksNow()
  if (sameOrder(prev, next)) return
  const to = next.findIndex((t) => t.id === id)
  const group = next[to]?.group ?? ''
  void optimistic<QueueTask[]>(
    keys.tasks,
    () => next,
    () => post(moveURL(id, to, group)),
    'Could not move the task',
  )
}

export function moveTaskTo(id: string, group: string, anchor: Anchor) {
  commitOrder(placeTask(tasksNow(), id, group, anchor), id)
}

// A new group is made of the task that lands in it, so naming and moving are
// one step.
export async function moveToNewGroup(id: string) {
  const name = await promptText({
    title: 'Name the new group',
    placeholder: 'e.g. Nightly sweeps',
    maxLength: 60,
  })
  if (name) moveTaskTo(id, name, {endOf: name})
}

export function commitGroupMove(name: string, before: string | null) {
  void optimistic<QueueTask[]>(
    keys.tasks,
    (prev) => moveGroup(prev ?? [], name, before),
    () => post('/api/groups/move', before === null ? {name} : {name, before}),
    'Could not move the group',
  )
}

export function setCollapsed(name: string, collapsed: boolean) {
  void optimistic<Group[]>(
    keys.groups,
    (prev) => prev?.map((g) => (g.name === name ? {...g, collapsed} : g)),
    () => post('/api/groups/collapse', {name, collapsed}),
    'Could not fold the group',
  )
}

// Renaming leaves the tasks where they are; typing a name already in use
// merges the two sections, so that is asked about first.
export async function renameGroup(name: string, existing: string[]) {
  const to = await promptText({
    title: `Rename the group “${name}”`,
    value: name,
    confirmLabel: 'Rename',
    maxLength: 60,
  })
  if (!to || to === name) return
  if (
    existing.includes(to) &&
    !(await confirm({
      title: `“${to}” already exists`,
      description: `Move the tasks of “${name}” into it? The two groups become one.`,
      confirmLabel: 'Merge',
    }))
  )
    return
  await optimistic<QueueTask[]>(
    keys.tasks,
    (prev) => prev?.map((t) => (t.group === name ? {...t, group: to} : t)),
    () => post('/api/groups/rename', {name, to}),
    'Could not rename the group',
  )
}

export function setEnabled(t: QueueTask, enabled: boolean) {
  void optimistic<QueueTask[]>(
    keys.tasks,
    (prev) => prev?.map((x) => (x.id === t.id ? {...x, enabled} : x)),
    () => post(`/api/tasks/${encodeURIComponent(t.id)}/${enabled ? 'enable' : 'disable'}`),
    enabled ? 'Could not enable the task' : 'Could not pause the task',
  )
}

export async function deleteTask(t: QueueTask) {
  const ok = await confirm({
    title: `Delete task “${t.name}”?`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  const done = await optimistic<QueueTask[]>(
    keys.tasks,
    (prev) => prev?.filter((x) => x.id !== t.id),
    () => del(`/api/tasks/${encodeURIComponent(t.id)}`),
    'Could not delete the task',
  )
  if (done) toast.success('Deleted', {description: t.name})
}

// The daemon opens a save panel; no answer (204) means it was cancelled.
export async function exportTask(t: QueueTask) {
  try {
    const r = await post<PathResult | undefined>(`/api/tasks/${encodeURIComponent(t.id)}/export`)
    if (r?.path) toast.success('Exported', {description: r.path})
  } catch (e) {
    toast.error('Export failed', {description: errorMessage(e)})
  }
}

// runNow starts a task outside its schedule and leaves the user where they
// are: the row shows it running, and the toast's "Open log" follows the run.
// With follow, the Log opens on the run as soon as it appears instead.
export async function runNow(t: QueueTask, opts: {follow?: boolean} = {}) {
  const since = Date.now()
  try {
    await post(`/api/tasks/${encodeURIComponent(t.id)}/run-now`)
  } catch (e) {
    // 409: the global pause went on since the list was drawn. Show the banner
    // that explains it rather than leaving a button that seems to do nothing.
    if (e instanceof ApiError && e.status === 409) {
      void invalidate(keys.settings)
      toast.error('All runs are paused', {
        description: errorMessage(e),
        action: {label: 'Resume', onClick: () => void setPaused(false)},
      })
    } else toast.error('Could not start the task', {description: errorMessage(e)})
    return
  }
  void invalidate(keys.tasks)
  const run = findNewRun(t.id, since)
  const openLog = () => {
    navigate('log')
    void run.then((id) => id && openRunLog(id))
  }
  if (opts.follow) return openLog()
  toast.success('Started', {
    // A one-shot job leaves the queue while it runs; say where it went.
    description: t.trigger === 'cron' ? t.name : `${t.name} · in the Log while it runs`,
    action: {label: 'Open log', onClick: openLog},
  })
}

// findNewRun polls briefly until the just-started run appears and resolves to
// its id, or to undefined when it does not show up in time. The start time
// keeps an earlier run of the same task from being taken for it.
function findNewRun(taskId: string, since: number): Promise<string | undefined> {
  const deadline = Date.now() + 12_000
  return new Promise((resolve) => {
    const tick = async () => {
      try {
        const runs = await get<Run[]>('/api/runs')
        queryClient.setQueryData(keys.runs, runs)
        const run = runs.find(
          (r) => r.task_id === taskId && new Date(r.started_at).getTime() >= since - 2_000,
        )
        if (run) {
          // The run exists, so the task now reports running too.
          void invalidate(keys.tasks)
          return resolve(run.run_id)
        }
      } catch {
        /* the next tick or the regular poll tries again */
      }
      if (Date.now() < deadline) setTimeout(tick, 500)
      else resolve(undefined)
    }
    setTimeout(tick, 400)
  })
}
