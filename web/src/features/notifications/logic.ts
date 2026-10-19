import {matchesWords} from '@/lib/search'
import type {AppNotification} from '@/lib/types'

const KIND_LABELS: Record<string, string> = {
  success: 'Run succeeded',
  failure: 'Run failed',
  artifact: 'New artifact',
  provider: 'Provider',
  task: 'From a task',
}
export const kindLabel = (k: string) => KIND_LABELS[k] ?? 'Other'

// Inside the DispatchQ window the sender's name is noise, and the glyph the
// daemon puts in a title is already in the row's icon: "DispatchQ ✗ nightly
// failed" reads as "nightly failed".
export const shortTitle = (t: string) =>
  String(t || '')
    .replace(/^DispatchQ\s*:?\s*/, '')
    .replace(/^(?:✓|✗|📄)\s*/u, '')

// The daemon attributes a message to its task for the channels outside the
// app ("Report · from nightly", or a last line "· from nightly"). The row
// shows the task's name beside the title, so in here that is said twice.
export function shortMessage(message: string, taskName?: string): string {
  if (!taskName) return message
  const from = `·\\s*from\\s+${taskName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`
  return message
    .replace(new RegExp(`(?:^|\\n)\\s*${from}\\s*$`), '')
    .replace(new RegExp(`\\s+${from}(?=\\n|$)`), '')
    .trim()
}

export interface NotificationFilters {
  query: string
  unread: boolean
  kind: string
}

export const NO_FILTERS: NotificationFilters = {query: '', unread: false, kind: ''}

export const anyFilter = (f: NotificationFilters) => !!(f.query.trim() || f.unread || f.kind)

export function shown(n: AppNotification, f: NotificationFilters): boolean {
  return (
    (!f.unread || n.unread) &&
    (!f.kind || n.kind === f.kind) &&
    matchesWords(f.query, n.title, n.message, n.task_name, kindLabel(n.kind))
  )
}

// kindOptions offers the kinds there are (with "Unread", the kinds with
// something unread); a choice that no longer matches anything falls back to
// "all".
export function kindOptions(notes: AppNotification[], f: NotificationFilters) {
  const kinds = new Map<string, string>()
  for (const n of f.unread ? notes.filter((x) => x.unread) : notes) kinds.set(n.kind || '', kindLabel(n.kind))
  const options = [...kinds]
    .filter(([v]) => v)
    .sort((x, y) => x[1].localeCompare(y[1]))
    .map(([value, label]) => ({value, label}))
  return {options, kind: f.kind && kinds.has(f.kind) ? f.kind : ''}
}

// What a click on the row opens, for the hint at its end.
export function targetLabel(n: AppNotification): string {
  if (n.artifact_id) return 'Opens the artifact'
  if (n.run_id) return 'Opens the run’s log'
  if (n.url) return 'Opens the link'
  return ''
}
