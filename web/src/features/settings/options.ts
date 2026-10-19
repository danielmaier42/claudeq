import type {SelectOption} from '@/components/ui/select'

// The interval and limit choices of the old form. A value set by hand or via
// the CLI that is not one of them is kept and listed first, so opening Settings
// never quietly rewrites it.

type Choice = [number, string]

export const HEARTBEAT: Choice[] = [
  [15, 'Every 15 minutes'],
  [30, 'Every 30 minutes'],
  [60, 'Every hour'],
  [120, 'Every 2 hours'],
  [180, 'Every 3 hours'],
  [360, 'Every 6 hours'],
]

export const IDLE: Choice[] = [
  [-1, 'Off'],
  [15, '15 minutes'],
  [30, '30 minutes'],
  [60, '1 hour'],
  [120, '2 hours'],
  [240, '4 hours'],
]

export const HISTORY: Choice[] = [
  [100, '100 runs'],
  [500, '500 runs'],
  [1000, '1000 runs'],
  [-1, 'Unlimited'],
]

// 0 is "the built-in default" in the store; the form shows what that means.
export const heartbeatValue = (v: number) => v || 60
export const idleValue = (v: number) => (v === 0 ? 30 : v)
export const historyValue = (v: number) => (v === 0 ? 500 : v)

export function choiceOptions(
  choices: Choice[],
  value: number,
  custom: (n: number) => string,
): SelectOption[] {
  const opts = choices.map(([v, label]) => ({value: String(v), label}))
  if (!choices.some(([v]) => v === value)) opts.unshift({value: String(value), label: custom(value)})
  return opts
}
