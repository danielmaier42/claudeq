import {FileUp, ListOrdered, Pause, Plus} from 'lucide-react'
import {useState} from 'react'
import {ViewLayout} from '@/components/layout/view-layout'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {EmptyState} from '@/components/ui/empty-state'
import {Segmented} from '@/components/ui/segmented'
import {Skeleton} from '@/components/ui/skeleton'
import {Tooltip} from '@/components/ui/tooltip'
import {pickAndImportTask} from '@/features/task-editor/import'
import {openAddTask} from '@/features/task-editor/store'
import {errorMessage} from '@/lib/api'
import {useGroups, useSettings, useTasks} from '@/lib/queries'
import {setPaused} from '@/features/settings/actions'
import {QueueBoard} from './QueueBoard'
import {setOnlyActive, useQueueFilter} from './store'
import {useQueueContext} from './use-queue-context'

export function QueueView() {
  const tasksQ = useTasks()
  const groupsQ = useGroups()
  const paused = !!useSettings().data?.paused
  const onlyActive = useQueueFilter((s) => s.onlyActive)
  const ctx = useQueueContext()

  const tasks = tasksQ.data
  // The daemon owns the fold state, so it survives a reload, a restart and
  // the window closing.
  const groups = groupsQ.data ?? []
  const collapsed = (name: string) => !!groups.find((g) => g.name === name)?.collapsed

  return (
    <ViewLayout
      title="Queue"
      subtitle={tasks && subtitle(tasks)}
      actions={
        <>
          <Tooltip content="Add a task from a .claudeq file">
            <Button variant="secondary" onClick={pickAndImportTask}>
              <FileUp /> Import…
            </Button>
          </Tooltip>
          <Button variant="primary" onClick={() => openAddTask()}>
            <Plus /> New task
          </Button>
        </>
      }
      toolbar={
        <Segmented
          aria-label="Which tasks to show"
          value={onlyActive ? 'active' : 'all'}
          onValueChange={(v) => setOnlyActive(v === 'active')}
          size="sm"
          options={[
            {value: 'all', label: 'All', tip: 'Show every task, paused ones included'},
            {value: 'active', label: 'Active', tip: 'Hide paused tasks'},
          ]}
        />
      }
    >
      {paused && <PausedBanner />}
      {tasks === undefined ? (
        tasksQ.isError ? (
          <Alert tone="danger" title="Could not load the queue">
            {errorMessage(tasksQ.error)}
          </Alert>
        ) : (
          <QueueSkeleton />
        )
      ) : !tasks.length ? (
        <EmptyState
          icon={<ListOrdered />}
          title="No tasks yet"
          description="Create your first task to queue work for Claude."
          action={
            <>
              <Button variant="secondary" onClick={pickAndImportTask}>
                <FileUp /> Import…
              </Button>
              <Button variant="primary" onClick={() => openAddTask()}>
                <Plus /> New task
              </Button>
            </>
          }
        />
      ) : onlyActive && !tasks.some((t) => t.enabled) ? (
        <EmptyState
          icon={<Pause />}
          title="No active tasks"
          description={`All ${tasks.length} task${tasks.length !== 1 ? 's are' : ' is'} paused. Switch the filter to All to see them.`}
          action={
            <Button variant="secondary" onClick={() => setOnlyActive(false)}>
              Show all tasks
            </Button>
          }
        />
      ) : (
        <QueueBoard
          tasks={tasks}
          onlyActive={onlyActive}
          collapsed={collapsed}
          groups={groups.map((g) => g.name)}
          ctx={ctx}
          paused={paused}
        />
      )}
    </ViewLayout>
  )
}

function subtitle(tasks: {enabled: boolean; running: boolean}[]): string {
  const running = tasks.filter((t) => t.running).length
  const off = tasks.filter((t) => !t.enabled).length
  const parts = [`${tasks.length} task${tasks.length !== 1 ? 's' : ''}`]
  if (running) parts.push(`${running} running`)
  if (off) parts.push(`${off} paused`)
  return parts.join(' · ')
}

// The queue looks idle either way, so the reason has to be visible where the
// tasks are, not only in Settings.
function PausedBanner() {
  const [busy, setBusy] = useState(false)
  return (
    <Alert
      tone="warning"
      title="All runs are paused"
      className="mb-5"
      actions={
        <Button
          size="sm"
          loading={busy}
          onClick={async () => {
            setBusy(true)
            await setPaused(false)
            setBusy(false)
          }}
        >
          Resume runs
        </Button>
      }
    >
      Nothing starts, not a task that comes due and not Run now, until you resume. A run already in flight
      keeps going.
    </Alert>
  )
}

function QueueSkeleton() {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface">
      {Array.from({length: 5}, (_, i) => (
        <div key={i} className="flex h-[52px] items-center gap-3 border-b border-border px-3 last:border-b-0">
          <Skeleton className="h-4 w-3" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5" style={{width: `${30 + ((i * 17) % 30)}%`}} />
            <Skeleton className="h-3 w-1/3" />
          </div>
          <Skeleton className="h-5 w-9 rounded-full" />
        </div>
      ))}
    </div>
  )
}
