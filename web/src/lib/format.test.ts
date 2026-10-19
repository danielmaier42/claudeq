import {describe, expect, it} from 'vitest'
import {baseName, duration, groupByDay, inDateRange, relTime, resumeAt} from './format'
import {matchesWords} from './search'

describe('format', () => {
  it('relTime speaks past and future', () => {
    const now = Date.parse('2026-01-10T12:00:00Z')
    expect(relTime(new Date(now - 30_000), now)).toBe('just now')
    expect(relTime(new Date(now - 5 * 60_000), now)).toBe('5 min ago')
    expect(relTime(new Date(now + 2 * 3600_000), now)).toBe('in 2 h')
  })
  it('duration picks the two largest units', () => {
    expect(duration(4_000)).toBe('4s')
    expect(duration(125_000)).toBe('2m 5s')
    expect(duration(3_726_000)).toBe('1h 2m')
  })
  it('inDateRange treats an empty end as open', () => {
    expect(inDateRange('2026-01-10T12:00:00', '2026-01-10', '')).toBe(true)
    expect(inDateRange('2026-01-10T12:00:00', '', '2026-01-09')).toBe(false)
  })
  it('baseName trims trailing slashes', () => {
    expect(baseName('/a/b/c/')).toBe('c')
  })
  it('resumeAt prefers the later time', () => {
    expect(resumeAt('2026-01-01T10:00:00Z', '2026-01-01T11:00:00Z')?.toISOString()).toBe(
      '2026-01-01T11:00:00.000Z',
    )
    expect(resumeAt(undefined, undefined)).toBeNull()
  })
  it('groupByDay keeps order and splits on day change', () => {
    const g = groupByDay(['2026-01-02T10:00:00', '2026-01-02T09:00:00', '2026-01-01T10:00:00'], (x) => x)
    expect(g.map((s) => s.items.length)).toEqual([2, 1])
  })
  it('matchesWords needs every word', () => {
    expect(matchesWords('adr 0050', 'ADR-0050 review')).toBe(true)
    expect(matchesWords('seo pdf', 'SEO job', 'report.docx')).toBe(false)
  })
})
