import type {SelectOption} from '@/components/ui/select'
import {relTime} from '@/lib/format'
import type {HealthState, LimitWindow, Provider, ProviderLimits} from '@/lib/types'

export type HealthTone = 'success' | 'warning' | 'danger'

// Readiness comes from the daemon, which probes the CLI itself; the card only
// says what it was told and never guesses.
const HEALTH: Record<HealthState, [HealthTone, string]> = {
  ready: ['success', 'Ready'],
  not_installed: ['danger', 'Not installed'],
  not_authenticated: ['danger', 'Not logged in'],
  invalid_configuration: ['danger', 'Invalid configuration'],
  disabled: ['warning', 'Switched off'],
  check_failed: ['warning', 'Could not be checked'],
}

export function healthInfo(state: string): {tone: HealthTone; label: string} {
  const known = HEALTH[state as HealthState]
  if (known) return {tone: known[0], label: known[1]}
  return {tone: 'warning', label: state || 'Unknown'}
}

export const byName = (a: Provider, b: Provider) => (a.name || a.id).localeCompare(b.name || b.id)

// fallbackOptions offers every other configured provider as the one that takes
// over while this one is rate-limited. A stored id that has since gone keeps
// its place, marked, so saving does not quietly drop the fallback the operator
// chose.
export function fallbackOptions(self: string, providers: Provider[], stored: string): SelectOption[] {
  const others = providers.filter((p) => p.id !== self)
  const opts: SelectOption[] = [
    {value: '', label: 'Wait for the limit'},
    ...others.map((p) => ({value: p.id, label: (p.name || p.id) + (p.enabled ? '' : ' (switched off)')})),
  ]
  if (stored && !others.some((p) => p.id === stored))
    opts.push({value: stored, label: `${stored} (unavailable)`})
  return opts
}

// A window shows what is left of it, so the bar empties as it is used. It is
// coloured before it runs dry: at 0% left the provider's tasks already wait,
// and the point is to see it coming.
export const windowLeft = (w: LimitWindow) => Math.min(100, Math.max(0, 100 - w.used_percent))

export function windowTone(left: number): 'danger' | 'warning' | 'accent' {
  return left <= 10 ? 'danger' : left <= 25 ? 'warning' : 'accent'
}

export function resetText(w: LimitWindow, now = Date.now()): string {
  if (!w.resets_at) return ''
  const at = new Date(w.resets_at)
  if (at.getTime() <= now) return 'Resets now'
  const day = at.toLocaleDateString(undefined, {weekday: 'short'})
  const time = at.toLocaleTimeString(undefined, {hour: '2-digit', minute: '2-digit'})
  return `Resets ${relTime(at, now)} · ${day} ${time}`
}

// limitStatusText says when the figures are from, or why there are none.
export function limitStatusText(l: ProviderLimits, now = Date.now()): string {
  switch (l.state) {
    case 'ok':
      return l.updated_at ? 'Updated ' + relTime(l.updated_at, now) : 'Updated'
    case 'unsupported':
      return l.reason || 'No limit'
    case 'disabled':
      return 'Switched off'
  }
  const when = l.updated_at ? ' Showing the reading from ' + relTime(l.updated_at, now) + '.' : ''
  return (l.reason || 'Could not be read.') + when
}

// A provider with nothing to report (switched off, or a harness without an
// allowance) gets no Limits row rather than a row saying so.
export function hasLimitsRow(l: ProviderLimits | undefined): l is ProviderLimits {
  return !!l && ((l.windows?.length ?? 0) > 0 || l.state === 'unavailable')
}
