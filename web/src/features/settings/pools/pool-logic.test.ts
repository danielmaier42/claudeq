import {describe, expect, it} from 'vitest'
import type {Pool, Provider} from '@/lib/types'
import {formatWeight, membersBody, nextMember, parseWeight, poolKind, weightPlaceholder} from './pool-logic'

const prov = (id: string, kind: Provider['kind']): Provider => ({id, kind}) as Provider

describe('poolKind', () => {
  const ps = [prov('a', 'claude-code'), prov('b', 'codex')]
  it('takes the first configured member and skips vanished ones', () => {
    expect(poolKind(['gone', 'b', 'a'], ps)).toBe('codex')
    expect(poolKind([], ps)).toBeUndefined()
  })
})

describe('weights', () => {
  it('reads empty and invalid as auto', () => {
    expect(parseWeight('')).toBe(0)
    expect(parseWeight('-2')).toBe(0)
    expect(parseWeight('abc')).toBe(0)
    expect(parseWeight('2.5')).toBe(2.5)
  })
  it('names the reported capacity', () => {
    expect(weightPlaceholder(20)).toBe('auto · 20×')
    expect(weightPlaceholder(undefined)).toBe('auto · 1×')
    expect(formatWeight(1.23456)).toBe('1.23')
  })
  it('keeps list order and drops unchecked members', () => {
    expect(membersBody(['a', 'b', 'c'], new Set(['c', 'a']), {a: '5', c: ''})).toEqual([
      {provider: 'a', weight: 5},
      {provider: 'c', weight: 0},
    ])
  })
})

describe('nextMember', () => {
  it('is the first member that can take work', () => {
    const pool = {
      ranking: [
        {provider: 'x', tier: 3},
        {provider: 'y', tier: 2},
      ],
    } as Pool
    expect(nextMember(pool)?.provider).toBe('y')
    expect(nextMember({ranking: [{provider: 'x', tier: 3}]} as Pool)).toBeUndefined()
  })
})
