import {describe, expect, it} from 'vitest'
import {dateRangeLabel, daysBack} from './date-range'

const now = new Date(2026, 9, 19, 15, 0)
const day = (d: string) =>
  new Date(d + 'T00:00').toLocaleDateString(undefined, {day: 'numeric', month: 'short'})

describe('daysBack', () => {
  it('counts today as the first of the days', () => {
    expect(daysBack(1, now)).toBe('2026-10-19')
    expect(daysBack(7, now)).toBe('2026-10-13')
    expect(daysBack(30, now)).toBe('2026-09-20')
  })
})

describe('dateRangeLabel', () => {
  it('is empty for an open range', () => {
    expect(dateRangeLabel('', '', now)).toBe('')
  })
  it('names one-sided and closed ranges', () => {
    expect(dateRangeLabel('2026-10-01', '', now)).toBe(`Since ${day('2026-10-01')}`)
    expect(dateRangeLabel('', '2026-10-05', now)).toBe(`Until ${day('2026-10-05')}`)
    expect(dateRangeLabel('2026-10-03', '2026-10-03', now)).toBe(`On ${day('2026-10-03')}`)
    expect(dateRangeLabel('2026-10-01', '2026-10-05', now)).toBe(
      `${day('2026-10-01')} – ${day('2026-10-05')}`,
    )
  })
  it('calls a range starting today "Today"', () => {
    expect(dateRangeLabel('2026-10-19', '', now)).toBe('Today')
  })
  it('adds the year for another year', () => {
    expect(dateRangeLabel('2025-12-24', '', now)).toContain('2025')
  })
})
