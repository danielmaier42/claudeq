import type {PoolRanking, Settings} from '@/lib/types'

// The daemon's default when Settings carries no threshold (0).
export const DEFAULT_BACKFILL_URGENCY = 1.5
export const BACKFILL_MIN = 0.5
export const BACKFILL_MAX = 5

export function backfillThreshold(s: Settings | undefined): number {
  return s?.backfill_urgency || DEFAULT_BACKFILL_URGENCY
}

// A provider above the threshold with room in both windows is one backfill
// tasks may run on (provider.MemberScore.Spare). Settings previews it while
// the slider moves; the daemon's own `backfill` flag wins otherwise.
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
