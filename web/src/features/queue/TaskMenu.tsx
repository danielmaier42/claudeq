import {
  ArrowDown,
  ArrowUp,
  FolderInput,
  FolderMinus,
  FolderPlus,
  MoreHorizontal,
  Pencil,
  Play,
  Share,
  Trash2,
} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {openTaskEditor} from '@/features/task-editor/store'
import type {QueueTask} from '@/lib/types'
import {deleteTask, exportTask, moveTaskTo, moveToNewGroup, runNow} from './actions'

// TaskMenu holds every row action, including the ones the row shows on hover,
// and the keyboard way to do what dragging does.
export function TaskMenu({
  task: t,
  canRun,
  groups,
  prevId,
  nextId,
  onOpenChange,
}: {
  task: QueueTask
  canRun: boolean
  groups: string[]
  prevId?: string
  nextId?: string
  onOpenChange?: (open: boolean) => void
}) {
  const group = t.group || ''
  const others = groups.filter((g) => g !== group)
  return (
    <DropdownMenu onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={`More actions for ${t.name}`}>
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-52">
        <DropdownMenuItem disabled={!canRun} onSelect={() => void runNow(t)}>
          <Play /> Run now
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => openTaskEditor({mode: 'edit', task: t})}>
          <Pencil /> Edit…
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void exportTask(t)}>
          <Share /> Export as .claudeq file…
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          disabled={!prevId}
          onSelect={() => prevId && moveTaskTo(t.id, group, {before: prevId})}
        >
          <ArrowUp /> Move up
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={!nextId}
          onSelect={() => nextId && moveTaskTo(t.id, group, {after: nextId})}
        >
          <ArrowDown /> Move down
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <FolderInput /> Move to group
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="max-h-80 overflow-y-auto">
            {others.map((g) =>
              g ? (
                <DropdownMenuItem key={g} onSelect={() => moveTaskTo(t.id, g, {endOf: g})}>
                  <span className="truncate">{g}</span>
                </DropdownMenuItem>
              ) : null,
            )}
            {others.some(Boolean) && <DropdownMenuSeparator />}
            <DropdownMenuItem onSelect={() => void moveToNewGroup(t.id)}>
              <FolderPlus /> New group…
            </DropdownMenuItem>
            {group && (
              <DropdownMenuItem onSelect={() => moveTaskTo(t.id, '', {endOf: ''})}>
                <FolderMinus /> Take out of “{group}”
              </DropdownMenuItem>
            )}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem destructive onSelect={() => void deleteTask(t)}>
          <Trash2 /> Delete…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
