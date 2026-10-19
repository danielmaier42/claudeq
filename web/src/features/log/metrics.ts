import {compact, duration, fullTime, runDuration, usd} from '@/lib/format'
import type {Run} from '@/lib/types'

// What a run cost and how long it took, short for a row and in full for its
// tooltip. A script run records none of the agent figures, so it shows only
// its duration.
export function runMetrics(r: Run, now: number): {short: string[]; detail: string[]} {
  const timed = !!r.finished_at || r.status === 'running'
  const short: string[] = timed ? [runDuration(r, now)] : []
  const detail: string[] = []
  const tokens = (r.input_tokens ?? 0) + (r.output_tokens ?? 0)
  if (r.cost_usd) {
    short.push(usd(r.cost_usd))
    detail.push(`Cost ${usd(r.cost_usd)}`)
  }
  if (tokens) {
    short.push(`${compact(tokens)} tok`)
    detail.push(
      `${(r.input_tokens ?? 0).toLocaleString()} input · ${(r.output_tokens ?? 0).toLocaleString()} output tokens`,
    )
  }
  if (r.num_turns) {
    short.push(`${r.num_turns} turn${r.num_turns === 1 ? '' : 's'}`)
    detail.push(`${r.num_turns} turn${r.num_turns === 1 ? '' : 's'}`)
  }
  if (r.duration_ms) detail.push(`${duration(r.duration_ms)} reported by the agent`)
  return {short, detail}
}

// The exact start and how long the run went on, for the hover on its time.
export function runTimeTip(r: Run, now: number): string {
  const start = `Started ${fullTime(r.started_at)}`
  if (r.finished_at) return `${start}\nRan ${runDuration(r, now)}`
  if (r.status === 'running') return `${start}\nRunning for ${runDuration(r, now)}`
  return start
}
