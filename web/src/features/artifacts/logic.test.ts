import {describe, expect, it} from 'vitest'
import type {Artifact} from '@/lib/types'
import {
  artifactKind,
  extLabel,
  frameLink,
  isMarkdown,
  neighbours,
  NO_FILTERS,
  NO_GROUP,
  shown,
  sourceName,
  sourceOptions,
} from './logic'

const art = (p: Partial<Artifact>): Artifact => ({
  id: 'a',
  title: 'Report',
  file_name: 'report.html',
  rel_path: 'a/report.html',
  size: 10,
  content_type: 'text/html; charset=utf-8',
  published_at: '2026-10-19T08:00:00Z',
  unread: false,
  ...p,
})

describe('artifactKind and extLabel', () => {
  it('classifies content types', () => {
    expect(artifactKind('application/pdf')).toBe('pdf')
    expect(artifactKind('text/html; charset=utf-8')).toBe('html')
    expect(artifactKind('image/png')).toBe('image')
    expect(artifactKind('application/json')).toBe('text')
    expect(artifactKind('text/markdown')).toBe('text')
    expect(artifactKind('application/zip')).toBe('other')
  })
  it('prefers the extension, else the kind', () => {
    expect(extLabel('x.tar.gz', 'application/gzip')).toBe('GZ')
    expect(extLabel('README', 'text/plain')).toBe('TEXT')
    expect(extLabel('blob', 'application/octet-stream')).toBe('FILE')
  })
})

describe('shown', () => {
  it('matches every search word across fields, including the type', () => {
    const a = art({
      title: 'ADR-0050 review',
      task_name: 'SEO audit',
      file_name: 'seo.pdf',
      content_type: 'application/pdf',
    })
    expect(shown(a, {...NO_FILTERS, query: 'adr 0050'})).toBe(true)
    expect(shown(a, {...NO_FILTERS, query: 'seo pdf'})).toBe(true)
    expect(shown(a, {...NO_FILTERS, query: 'seo html'})).toBe(false)
  })
  it('filters by group, "no group", parent and date', () => {
    const a = art({group: 'nightly', origin_id: 'w'})
    const b = art({id: 'b'})
    expect(shown(a, {...NO_FILTERS, group: 'nightly'})).toBe(true)
    expect(shown(b, {...NO_FILTERS, group: 'nightly'})).toBe(false)
    expect(shown(b, {...NO_FILTERS, group: NO_GROUP})).toBe(true)
    expect(shown(a, {...NO_FILTERS, origin: 'w'})).toBe(true)
    expect(shown(b, {...NO_FILTERS, origin: 'w'})).toBe(false)
    expect(shown(a, {...NO_FILTERS, from: '2026-10-20'})).toBe(false)
  })
})

describe('sourceOptions', () => {
  const arts = [
    art({id: '1', group: 'b', origin_id: 'w1', origin_name: 'Watcher'}),
    art({id: '2', group: 'a', origin_id: 'w2', origin_name: 'Alpha'}),
    art({id: '3', origin_id: 'w3', origin_name: 'Loose'}),
  ]
  it('sorts groups by label with "No group" last', () => {
    expect(sourceOptions(arts, NO_FILTERS).groups.map((g) => g.label)).toEqual(['a', 'b', 'No group'])
  })
  it('offers only the parents of the chosen group', () => {
    const o = sourceOptions(arts, {...NO_FILTERS, group: 'b'})
    expect(o.origins).toEqual([{value: 'w1', label: 'Watcher'}])
  })
  it('falls back to "all" for a stale choice', () => {
    const o = sourceOptions(arts, {...NO_FILTERS, group: 'gone', origin: 'w9'})
    expect(o.group).toBe('')
    expect(o.origin).toBe('')
  })
})

describe('sourceName', () => {
  it('names the parent when another job published', () => {
    expect(
      sourceName(art({task_id: 't', task_name: 'Review', origin_id: 'w', origin_name: 'Watcher'})),
    ).toEqual({
      name: 'Watcher',
      chain: 'Watcher › Review',
    })
    expect(
      sourceName(art({task_id: 't', task_name: 'Review', origin_id: 't', origin_name: 'Review'}))?.chain,
    ).toBe('Review')
    expect(sourceName(art({}))).toBeNull()
  })
})

describe('neighbours', () => {
  it('finds the artifacts on either side', () => {
    expect(neighbours(['a', 'b', 'c'], 'b')).toEqual({index: 1, prev: 'a', next: 'c'})
    expect(neighbours(['a', 'b'], 'a')).toEqual({index: 0, prev: null, next: 'b'})
    expect(neighbours(['a'], 'x').index).toBe(-1)
  })
})

describe('frameLink', () => {
  it('passes http(s) links the frame reports', () => {
    expect(frameLink({type: 'dispatchq:open-link', url: 'https://dev.azure.com/x?a=1'})).toBe(
      'https://dev.azure.com/x?a=1',
    )
    expect(frameLink({type: 'dispatchq:open-link', url: 'http://example.com'})).toBe('http://example.com/')
  })
  it('ignores other schemes, other messages and junk', () => {
    expect(frameLink({type: 'dispatchq:open-link', url: 'javascript:alert(1)'})).toBeNull()
    expect(frameLink({type: 'dispatchq:open-link', url: 'file:///etc/passwd'})).toBeNull()
    expect(frameLink({type: 'dispatchq:open-link', url: 'not a url'})).toBeNull()
    expect(frameLink({type: 'other', url: 'https://example.com'})).toBeNull()
    expect(frameLink('https://example.com')).toBeNull()
    expect(frameLink(null)).toBeNull()
  })
})

describe('isMarkdown', () => {
  it('knows Markdown by type or by name', () => {
    expect(isMarkdown('report.md', 'text/plain; charset=utf-8')).toBe(true)
    expect(isMarkdown('NOTES.MARKDOWN', '')).toBe(true)
    expect(isMarkdown('report', 'text/markdown')).toBe(true)
  })
  it('leaves other text alone', () => {
    expect(isMarkdown('notes.txt', 'text/plain')).toBe(false)
    expect(isMarkdown('page.html', 'text/html')).toBe(false)
    expect(isMarkdown('md', 'text/plain')).toBe(false)
  })
})
