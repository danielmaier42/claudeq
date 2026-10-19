import {relTime} from '@/lib/format'
import type {Run, RunStatus} from '@/lib/types'

export type OutcomeTone = 'success' | 'danger' | 'warning' | 'neutral'

export interface Outcome {
  runId: string
  tone: OutcomeTone
  // "last: failed 1 h ago"
  text: string
  // A success is the expected case and stays in the background; anything else
  // should catch the eye.
  quiet: boolean
}

const WORDS: Record<Exclude<RunStatus, 'running'>, [string, OutcomeTone]> = {
  success: ['succeeded', 'success'],
  failed: ['failed', 'danger'],
  auth_error: ['login failed', 'danger'],
  rate_limited_waiting: ['rate limited', 'warning'],
  canceled: ['canceled', 'neutral'],
}

// lastOutcome is how the task's most recent run ended, for the row's meta
// line, or null when it never ran. A run still in flight is left to the
// Running badge.
export function lastOutcome(
  r: Pick<Run, 'run_id' | 'status' | 'started_at' | 'finished_at'> | undefined,
  now = Date.now(),
): Outcome | null {
  if (!r || r.status === 'running') return null
  const [word, tone] = WORDS[r.status] ?? [r.status, 'neutral']
  const at = r.finished_at || r.started_at
  return {runId: r.run_id, tone, text: `last: ${word} ${relTime(at, now)}`, quiet: tone === 'success'}
}
