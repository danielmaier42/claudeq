import {relTime} from '@/lib/format'
import type {LimitWindow, PoolRanking, ProviderLimits, Settings} from '@/lib/types'

// The daemon's default when Settings carries no threshold (0).
export const DEFAULT_BACKFILL_URGENCY = 1.5
export const BACKFILL_MIN = 0.5
export const BACKFILL_MAX = 5

export function backfillThreshold(s: Settings | undefined): number {
  return s?.backfill_urgency || DEFAULT_BACKFILL_URGENCY
}

// A window shows what is left of it, so the bar empties as it is used. It is
// coloured before it runs dry: at 0% left the provider's tasks already wait,
// and the point is to see it coming.
export function windowLeft(w: LimitWindow): number {
  return Math.min(100, Math.max(0, 100 - w.used_percent))
}

export type Level = 'ok' | 'warning' | 'danger'

export function leftLevel(left: number): Level {
  return left <= 10 ? 'danger' : left <= 25 ? 'warning' : 'ok'
}

export function resetText(w: LimitWindow, now = Date.now()): string {
  if (!w.resets_at) return ''
  const at = new Date(w.resets_at)
  if (at.getTime() <= now) return 'Resets now'
  const day = at.toLocaleDateString(undefined, {weekday: 'short'})
  const time = at.toLocaleTimeString(undefined, {hour: '2-digit', minute: '2-digit'})
  return `Resets ${relTime(at, now)} · ${day} ${time}`
}

// The week sits in the same column on every provider, also where an account has
// no five-hour window, so the cards can be read down a column.
export function windowSlots(windows: LimitWindow[] = []): (LimitWindow | null)[] {
  const five = windows.find((w) => w.id === 'five_hour') ?? null
  const week = windows.find((w) => w.id === 'week') ?? null
  const rest = windows.filter((w) => w !== five && w !== week)
  if (!five && !week) return rest
  return [five, week, ...rest]
}

// limitStatusText says when the figures are from, or why there are none.
export function limitStatusText(l: ProviderLimits, now = Date.now()): string {
  switch (l.state) {
    case 'ok':
      return l.updated_at ? `Updated ${relTime(l.updated_at, now)}` : 'Updated'
    case 'unsupported':
      return l.reason || 'No limit'
    case 'disabled':
      return 'Switched off'
  }
  const when = l.updated_at ? ` Showing the reading from ${relTime(l.updated_at, now)}.` : ''
  return (l.reason || 'Could not be read.') + when
}

// A provider above the threshold with room in both windows is one backfill
// tasks may run on (provider.MemberScore.Spare). The Dashboard previews it
// while the slider moves; the daemon's own `backfill` flag wins otherwise.
export function isSpare(u: PoolRanking | undefined, threshold: number): boolean {
  return !!u && u.tier === 0 && u.urgency > threshold
}

// urgencyText is a provider's urgency in words: a tier above 0 has a note
// instead of a usable number.
export function urgencyText(u: PoolRanking | undefined): string {
  if (!u) return ''
  if (u.tier > 0) return u.note
  const shared = u.running ? ` · shared by ${u.running} run${u.running > 1 ? 's' : ''}` : ''
  return `Urgency ${u.urgency.toFixed(1)}${shared}`
}

export const fmtWeight = (w: number) => `${Math.round(w * 100) / 100}×`
