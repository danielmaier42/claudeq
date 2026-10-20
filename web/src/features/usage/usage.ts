import {compact, usd} from '@/lib/format'
import type {StatsBucket, StatsDay} from '@/lib/types'

export type Metric = 'tokens' | 'runs' | 'cost'

export interface MetricDef {
  id: Metric
  label: string
  value: (d: StatsDay) => number
  fmt: (n: number) => string
  integer?: boolean
}

export const METRICS: MetricDef[] = [
  {id: 'tokens', label: 'Tokens', value: (d) => d.tokens, fmt: (n) => fmtTokens(n)},
  {id: 'runs', label: 'Runs', value: (d) => d.runs, fmt: (n) => String(n), integer: true},
  {id: 'cost', label: 'API-equiv. cost', value: (d) => d.cost_usd, fmt: (n) => usd(n)},
]

// emptyText is the chart's line for a period without any of the chosen
// measure, in place of an axis that would run from 0 to 1 over nothing.
const EMPTY: Record<Metric, string> = {
  tokens: 'No tokens recorded in this period',
  runs: 'No runs in this period',
  cost: 'No cost recorded in this period',
}
export const emptyText = (m: Metric) => EMPTY[m]

export const metricDef = (m: Metric) => METRICS.find((x) => x.id === m) ?? METRICS[0]

export function fmtTokens(n: number): string {
  return n < 1000 ? String(n) : compact(n)
}

export const bucketTokens = (b: StatsBucket) => b.input_tokens + b.output_tokens

// bucketValue is a bucket's figure for the chosen measure, as the chart counts it.
export function bucketValue(m: Metric, b: StatsBucket): number {
  return m === 'tokens' ? bucketTokens(b) : m === 'runs' ? b.runs : b.cost_usd
}

export function successRate(b: StatsBucket): number | null {
  const done = b.success + b.failed
  return done ? Math.round((b.success / done) * 100) : null
}

// niceTicks splits 0..max into a few round steps (1, 2 or 5 × 10^k), so the
// axis reads 0 / 50k / 100k rather than 0 / 43.7k / 87.4k. The last tick is the
// chart's top. Counts (runs) never get a fractional step.
export function niceTicks(max: number, integer = false, target = 4): number[] {
  if (!(max > 0)) return [0, 1]
  const raw = max / target
  const mag = 10 ** Math.floor(Math.log10(raw))
  const nice = [1, 2, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? 10 * mag
  const step = integer ? Math.max(1, Math.round(nice)) : nice
  const ticks: number[] = []
  for (let v = 0; v < max + step * 1e-9; v += step) ticks.push(Number(v.toPrecision(12)))
  if (ticks[ticks.length - 1] < max) ticks.push(Number((ticks[ticks.length - 1] + step).toPrecision(12)))
  return ticks
}

export function dayLong(date: string): string {
  const [y, m, d] = date.split('-').map(Number)
  return new Date(y, m - 1, d).toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}
