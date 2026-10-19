import {create} from 'zustand'
import type {ImportResult, QueueTask, Run, TaskInput} from '@/lib/types'

// How the task editor is opened from anywhere in the app. The editor itself
// (TaskEditor.tsx) is mounted once by the shell and reads this.
export type TaskEditorRequest =
  | {mode: 'add'; prefill?: Partial<TaskInput>}
  | {mode: 'edit'; task: QueueTask}
  // Schedule a past run's task again: a new task prefilled from the run's snapshot.
  | {mode: 'replay'; run: Run}
  // A .dispatchq file read by POST /api/tasks/import, reviewed before it is created.
  | {mode: 'import'; result: ImportResult}

export const useTaskEditor = create<{request: TaskEditorRequest | null}>(() => ({request: null}))

export const openTaskEditor = (request: TaskEditorRequest) => useTaskEditor.setState({request})
export const openAddTask = (prefill?: Partial<TaskInput>) => openTaskEditor({mode: 'add', prefill})
export const closeTaskEditor = () => useTaskEditor.setState({request: null})
