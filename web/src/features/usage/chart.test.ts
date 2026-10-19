import {describe, expect, it} from 'vitest'
import {localDate} from '@/lib/format'
import type {StatsDay} from '@/lib/types'
import {
  axisLabels,
  bucketTitle,
  byWeek,
  chartBuckets,
  narrowSkips,
  periodLabel,
  WEEKLY_AFTER,
  weekSpan,
} from './chart'

// days builds n consecutive days ending on the given one, each with one run
// and 10 tokens per day of the month, so sums are easy to check.
function days(n: number, last: string): StatsDay[] {
  const [y, m, d] = last.split('-').map(Number)
  return Array.from({length: n}, (_, i) => {
    const date = localDate(new Date(y, m - 1, d - (n - 1 - i)))
    return {date, runs: 1, cost_usd: 0.5, tokens: 10 * Number(date.slice(8))}
  })
}

describe('chart', () => {
  it('periodLabel names the window the response covers', () => {
    expect(periodLabel({days: 14})).toBe('Last 14 days')
    expect(periodLabel({days: 1})).toBe('Today')
    expect(periodLabel({days: 213, all: true})).toBe('All time')
  })

  it('chartBuckets keeps days up to the threshold and switches to weeks after it', () => {
    const short = chartBuckets(days(WEEKLY_AFTER, '2026-10-19'))
    expect(short.unit).toBe('day')
    expect(short.buckets).toHaveLength(WEEKLY_AFTER)
    expect(short.buckets[0].end).toBe(short.buckets[0].date)
    const long = chartBuckets(days(WEEKLY_AFTER + 1, '2026-10-19'))
    expect(long.unit).toBe('week')
  })

  it('byWeek sums Monday-to-Sunday weeks and keeps partial weeks at both ends', () => {
    // Thu 2026-10-01 .. Mon 2026-10-19: Thu–Sun, two full weeks, Monday alone.
    const weeks = byWeek(days(19, '2026-10-19'))
    expect(weeks.map((w) => [w.date, w.end])).toEqual([
      ['2026-10-01', '2026-10-04'],
      ['2026-10-05', '2026-10-11'],
      ['2026-10-12', '2026-10-18'],
      ['2026-10-19', '2026-10-19'],
    ])
    expect(weeks.map((w) => w.runs)).toEqual([4, 7, 7, 1])
    expect(weeks[0].tokens).toBe(10 * (1 + 2 + 3 + 4))
    expect(weeks[1].cost_usd).toBeCloseTo(3.5)
    // Every day lands in exactly one week.
    expect(weeks.reduce((n, w) => n + w.runs, 0)).toBe(19)
  })

  it('byWeek groups across a month and a year boundary', () => {
    const weeks = byWeek(days(7, '2027-01-03'))
    expect(weeks).toHaveLength(1)
    expect([weeks[0].date, weeks[0].end]).toEqual(['2026-12-28', '2027-01-03'])
  })

  it('weekSpan names the days a week holds', () => {
    const w = (date: string, end: string) => ({date, end, runs: 0, cost_usd: 0, tokens: 0})
    const today = '2026-10-19'
    expect(weekSpan(w('2026-10-05', '2026-10-11'), today)).toMatch(/Oct.*5 – 11$/)
    expect(weekSpan(w('2026-09-28', '2026-10-04'), today)).toMatch(/Sep.*28 – Oct.*4$/)
    expect(weekSpan(w('2026-10-19', '2026-10-19'), today)).toBe('Today')
    expect(weekSpan(w('2026-10-12', '2026-10-19'), today)).toMatch(/Oct.*12 – today$/)
    expect(weekSpan(w('2025-10-06', '2025-10-12'), today)).toContain('2025')
    expect(bucketTitle(w('2026-10-05', '2026-10-11'), 'week', today)).toMatch(/^Week · /)
  })

  it('axisLabels labels every day of a short range with the month where it starts', () => {
    const d = chartBuckets(days(4, '2026-10-02')).buckets
    const labels = axisLabels(d, 'day', '2026-10-02')
    expect(labels[0]).toMatch(/Sep/)
    expect(labels[1]).toBe('30')
    expect(labels[2]).toMatch(/Oct/)
    expect(labels[3]).toBe('Today')
  })

  it('axisLabels thins a long range from the newest column', () => {
    const d = chartBuckets(days(30, '2026-10-19')).buckets
    const labels = axisLabels(d, 'day', '2026-10-19')
    expect(labels.filter(Boolean).length).toBeLessThanOrEqual(16)
    expect(labels[29]).toBe('Today')
    expect(labels[28]).toBe('')
    // The first shown label carries the month even though column 0 is skipped.
    expect(labels.find(Boolean)).toMatch(/Sep/)
  })

  it('axisLabels calls the running week this week', () => {
    const {unit, buckets} = chartBuckets(days(90, '2026-10-19'))
    expect(axisLabels(buckets, unit, '2026-10-19').at(-1)).toBe('This week')
  })

  it('narrowSkips drops every second shown label but keeps the newest', () => {
    expect(narrowSkips(['a', '', 'b', '', 'c', 'd'])).toEqual([true, false, false, false, true, false])
  })
})
