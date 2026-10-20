import {inDateRange} from '@/lib/format'
import {matchesWords} from '@/lib/search'
import type {Artifact} from '@/lib/types'

// What the viewer can do with a file, from its content type.
export type ArtifactKind = 'pdf' | 'html' | 'image' | 'text' | 'other'

export function artifactKind(contentType: string): ArtifactKind {
  const ct = (contentType || '').toLowerCase()
  if (ct.includes('pdf')) return 'pdf'
  if (ct.startsWith('text/html')) return 'html'
  if (ct.startsWith('image/')) return 'image'
  if (ct.startsWith('text/') || ['json', 'xml', 'javascript', 'csv'].some((t) => ct.includes(t)))
    return 'text'
  return 'other'
}

// isMarkdown tells a Markdown file, which the viewer renders, from other text.
// The daemon usually types it text/plain, so the file name decides as well.
export function isMarkdown(fileName: string, contentType: string): boolean {
  return (contentType || '').toLowerCase().includes('markdown') || /\.(md|markdown)$/i.test(String(fileName))
}

// extLabel is the short type tag on a card: the file's extension, or the kind
// when the name has none.
export function extLabel(fileName: string, contentType: string): string {
  const m = String(fileName).match(/\.([a-z0-9]+)$/i)
  if (m) return m[1].toUpperCase()
  const k = artifactKind(contentType)
  return k === 'other' ? 'FILE' : k.toUpperCase()
}

// contentURL is where an artifact's file is served: as is, as a download, or
// for the viewer's frame, where an HTML page reports link clicks to the window.
export function contentURL(id: string, as?: 'download' | 'frame'): string {
  return `/api/artifacts/${encodeURIComponent(id)}/content${as ? `?${as}=1` : ''}`
}

// frameLink is the http(s) link an HTML artifact in the viewer asks the window
// to open, or null for any other message.
export function frameLink(data: unknown): string | null {
  if (typeof data !== 'object' || data === null) return null
  const {type, url} = data as {type?: unknown; url?: unknown}
  if (type !== 'dispatchq:open-link' || typeof url !== 'string') return null
  try {
    const u = new URL(url)
    return u.protocol === 'http:' || u.protocol === 'https:' ? u.href : null
  } catch {
    return null
  }
}

// NO_GROUP stands for "no group" in the group menu: a real group name never
// starts with a space.
export const NO_GROUP = ' none'
export const groupKey = (a: Artifact) => a.group || NO_GROUP

export interface ArtifactFilters {
  query: string
  group: string
  origin: string
  from: string
  to: string
}

export const NO_FILTERS: ArtifactFilters = {query: '', group: '', origin: '', from: '', to: ''}

export const anyFilter = (f: ArtifactFilters) => !!(f.query.trim() || f.group || f.origin || f.from || f.to)

// Every word of the search has to occur somewhere in the artifact — title,
// description, file name, task, parent, group or type — so "adr 0050" finds
// "ADR-0050 review" and "seo pdf" the PDF of the SEO job.
export function matchesQuery(a: Artifact, query: string): boolean {
  return matchesWords(
    query,
    a.title,
    a.description,
    a.file_name,
    a.task_name,
    a.origin_name,
    a.group,
    extLabel(a.file_name, a.content_type),
  )
}

export function shown(a: Artifact, f: ArtifactFilters): boolean {
  return (
    inDateRange(a.published_at, f.from, f.to) &&
    (!f.group || groupKey(a) === f.group) &&
    (!f.origin || a.origin_id === f.origin) &&
    matchesQuery(a, f.query)
  )
}

export interface Option {
  value: string
  label: string
}

// sourceOptions fills the group and parent menus from the artifacts there are.
// The parent is the job at the root of the chain that published (a watcher behind
// its review job), resolved by the daemon, and the parent menu only offers
// parents in the chosen group. A choice that no longer matches anything falls
// back to "all", so the list can never be filtered to nothing by a stale
// selection.
export function sourceOptions(arts: Artifact[], f: ArtifactFilters) {
  const pool = arts
  const groups = new Map<string, string>()
  for (const a of pool) groups.set(groupKey(a), a.group || 'No group')
  const group = f.group && groups.has(f.group) ? f.group : ''
  const origins = new Map<string, string>()
  for (const a of pool) {
    if (!a.origin_id || origins.has(a.origin_id)) continue
    if (group && groupKey(a) !== group) continue
    origins.set(a.origin_id, a.origin_name || a.origin_id)
  }
  const origin = f.origin && origins.has(f.origin) ? f.origin : ''
  return {groups: byLabel(groups, NO_GROUP), origins: byLabel(origins), group, origin}
}

// byLabel sorts menu entries by label, with the entry for "none" last.
function byLabel(m: Map<string, string>, noneKey?: string): Option[] {
  return [...m]
    .sort((x, y) => Number(x[0] === noneKey) - Number(y[0] === noneKey) || x[1].localeCompare(y[1]))
    .map(([value, label]) => ({value, label}))
}

// sourceName is who made an artifact: the parent job when another job created
// the publisher (the watcher is what one looks for, not the review job it
// filed), else the publisher itself. chain spells out the whole line.
export function sourceName(a: Artifact): {name: string; chain: string} | null {
  if (!a.task_name) return null
  const parent = a.origin_id && a.origin_id !== a.task_id && a.origin_name ? a.origin_name : ''
  return {name: parent || a.task_name, chain: parent ? `${parent} › ${a.task_name}` : a.task_name}
}

// neighbours finds the artifacts before and after `id` in the order the viewer
// steps through.
export function neighbours(ids: string[], id: string) {
  const i = ids.indexOf(id)
  if (i < 0) return {index: -1, prev: null, next: null}
  return {index: i, prev: i > 0 ? ids[i - 1] : null, next: i < ids.length - 1 ? ids[i + 1] : null}
}
