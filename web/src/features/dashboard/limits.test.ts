import {describe, expect, it} from 'vitest'
import type {PoolRanking} from '@/lib/types'
import {isSpare, leftLevel, limitStatusText, windowSlots} from './limits'

const score = (over: Partial<PoolRanking>): PoolRanking => ({
  provider: 'p',
  name: 'p',
  tier: 0,
  urgency: 2,
  score: 0,
  weight: 1,
  week_free: 50,
  running: 0,
  note: '',
  ...over,
})

describe('limits', () => {
  it('colours a window before it runs dry', () => {
    expect(leftLevel(60)).toBe('ok')
    expect(leftLevel(25)).toBe('warning')
    expect(leftLevel(10)).toBe('danger')
  })
  it('keeps the week in the second column without a five-hour window', () => {
    const week = {id: 'week', label: 'Week', used_percent: 10}
    expect(windowSlots([week])).toEqual([null, week])
  })
  it('isSpare needs room in both windows and an urgency above, not at, the threshold', () => {
    expect(isSpare(score({}), 1.5)).toBe(true)
    expect(isSpare(score({urgency: 1.5}), 1.5)).toBe(false)
    expect(isSpare(score({tier: 2, urgency: 9}), 1.5)).toBe(false)
    expect(isSpare(undefined, 1.5)).toBe(false)
  })
  it('a failed read names its reason and the age of what is shown', () => {
    const t = Date.parse('2026-10-19T12:00:00Z')
    const l = {
      state: 'unavailable' as const,
      reason: 'Token expired.',
      updated_at: '2026-10-19T11:50:00Z',
      checked_at: '',
    }
    expect(limitStatusText(l, t)).toBe('Token expired. Showing the reading from 10 min ago.')
  })
})
