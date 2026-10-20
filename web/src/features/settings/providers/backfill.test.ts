import {describe, expect, it} from 'vitest'
import type {PoolRanking} from '@/lib/types'
import {backfillThreshold, isSpare, urgencyText} from './backfill'

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

describe('backfill', () => {
  it('isSpare needs room in both windows and an urgency above, not at, the threshold', () => {
    expect(isSpare(score({}), 1.5)).toBe(true)
    expect(isSpare(score({urgency: 1.5}), 1.5)).toBe(false)
    expect(isSpare(score({tier: 2, urgency: 9}), 1.5)).toBe(false)
    expect(isSpare(undefined, 1.5)).toBe(false)
  })
  it('falls back to the daemon default without a stored threshold', () => {
    expect(backfillThreshold(undefined)).toBe(1.5)
  })
  it('words the urgency, or the note of a provider that cannot take work', () => {
    for (const running of [0, 1, 2]) {
      expect(urgencyText(score({running}))).toBe('Urgency 2.0')
    }
    expect(urgencyText(score({tier: 3, note: 'Out until Monday'}))).toBe('Out until Monday')
  })
})
