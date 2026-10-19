import {describe, expect, it} from 'vitest'
import {niceTicks, successRate} from './usage'

describe('usage', () => {
  it('niceTicks rounds to 1/2/5 steps and covers the maximum', () => {
    expect(niceTicks(87_400)).toEqual([0, 50_000, 100_000])
    expect(niceTicks(1, true)).toEqual([0, 1])
    expect(niceTicks(3)).toEqual([0, 1, 2, 3])
    expect(niceTicks(0.42)).toEqual([0, 0.2, 0.4, 0.6])
    expect(niceTicks(0)).toEqual([0, 1])
  })
  it('successRate ignores runs that are neither', () => {
    const b = {runs: 5, success: 3, failed: 1, cost_usd: 0, input_tokens: 0, output_tokens: 0, duration_ms: 0}
    expect(successRate(b)).toBe(75)
    expect(successRate({...b, success: 0, failed: 0})).toBeNull()
  })
})
