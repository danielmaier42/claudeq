import type {Pool, PoolMember, PoolRanking, Provider} from '@/lib/types'

// A pool's members are all of one type: a task's model has to mean the same
// thing on every one of them. The kind is the first member's that is still
// configured; an empty selection admits every kind.
export function poolKind(members: string[], providers: Provider[]): string | undefined {
  for (const id of members) {
    const p = providers.find((x) => x.id === id)
    if (p) return p.kind
  }
  return undefined
}

// An empty weight means exactly what the provider's plan reports, so the
// placeholder says that number.
export function weightPlaceholder(capacity: number | undefined): string {
  return capacity ? `auto · ${capacity}×` : 'auto · 1×'
}

// parseWeight reads a weight field: empty, zero, negative or nonsense is 0,
// which the daemon takes as "auto".
export function parseWeight(text: string): number {
  const n = parseFloat(text)
  return Number.isFinite(n) && n > 0 ? n : 0
}

export function formatWeight(n: number): string {
  return String(Math.round(n * 100) / 100)
}

// nextMember is where a run started now would go: the first member that can
// take work at all.
export function nextMember(pool: Pool): PoolRanking | undefined {
  return pool.ranking?.find((r) => r.tier < 3)
}

// membersBody turns the dialog's selection into the payload, in the order the
// providers are listed, so an untouched pool writes back the same members.
export function membersBody(
  order: string[],
  checked: Set<string>,
  weights: Record<string, string>,
): PoolMember[] {
  return order
    .filter((id) => checked.has(id))
    .map((id) => ({provider: id, weight: parseWeight(weights[id] ?? '')}))
}
