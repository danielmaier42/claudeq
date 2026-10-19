import type {PromptReviewKind, PromptReviewResponse} from '@/lib/types'

// Answers are remembered on disk, keyed by a hash of exactly what was asked, so
// reopening the same task — or the app itself — shows the last finding instead
// of paying for the same review again. Entries are dropped after a day because
// the machine they judge may have moved on since.
const STORE_KEY = 'cq.prompt-review'
const TTL = 86_400_000
const MAX = 40

interface Entry {
  at: number
  res: PromptReviewResponse
}

// reviewKey hashes the question rather than keeping a copy of it: prompts are
// long, and the cache only ever has to tell one question apart from another
// (FNV-1a). Who reviews is part of the question — "the default provider"
// resolves to another account and model as Settings change.
export function reviewKey(kind: PromptReviewKind, prompt: string, dir: string, reviewer: string): string {
  const s = JSON.stringify([kind, prompt, dir, reviewer])
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return `${kind}:${s.length}:${(h >>> 0).toString(36)}`
}

// Storage can be unavailable or hold something else's data; a cache is never
// worth an exception on the way to writing a task.
export function createReviewCache(
  storage: Pick<Storage, 'getItem' | 'setItem'> | null,
  now = () => Date.now(),
) {
  let map: Map<string, Entry> | null = null

  const load = () => {
    if (map) return map
    map = new Map()
    try {
      const raw: unknown = JSON.parse(storage?.getItem(STORE_KEY) ?? '[]')
      if (Array.isArray(raw))
        for (const e of raw as {k?: string; at?: number; res?: PromptReviewResponse}[])
          if (e?.k && e.at && e.res) map.set(e.k, {at: e.at, res: e.res})
    } catch {
      /* unreadable cache: start empty */
    }
    return map
  }
  const persist = () => {
    try {
      storage?.setItem(STORE_KEY, JSON.stringify([...load()].map(([k, v]) => ({k, at: v.at, res: v.res}))))
    } catch {
      /* full or unavailable: the cache only saves usage */
    }
  }

  return {
    get(key: string): PromptReviewResponse | null {
      const m = load()
      const hit = m.get(key)
      if (!hit) return null
      if (now() - hit.at > TTL) {
        m.delete(key)
        persist()
        return null
      }
      return hit.res
    },
    set(key: string, res: PromptReviewResponse) {
      const m = load()
      m.delete(key)
      m.set(key, {at: now(), res})
      while (m.size > MAX) m.delete(m.keys().next().value as string)
      persist()
    },
    forget(key: string) {
      if (load().delete(key)) persist()
    },
  }
}

const storage = (() => {
  try {
    return window.localStorage
  } catch {
    return null
  }
})()

export const reviewCache = createReviewCache(storage)
