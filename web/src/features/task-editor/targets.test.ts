import {describe, expect, it} from 'vitest'
import type {Pool, Provider} from '@/lib/types'
import {effortOptions, modelProvider, takesEffort, targetOptions} from './targets'

const prov = (id: string, over: Partial<Provider> = {}): Provider =>
  ({
    id,
    name: id.toUpperCase(),
    kind: 'claude-code',
    enabled: true,
    default: false,
    reasoning_effort: false,
    health: {state: 'ready', checked_at: ''},
    ...over,
  }) as Provider

const providers = [
  prov('b', {default: true}),
  prov('a', {reasoning_effort: true}),
  prov('down', {health: {state: 'not_authenticated', checked_at: ''}}),
  prov('labs', {beta: true}),
]
const pools: Pool[] = [{id: 'team', name: 'Team', members: [{provider: 'a', weight: 1}], ranking: []}]

describe('targetOptions', () => {
  it('offers ready providers sorted, with the default first', () => {
    const t = targetOptions(providers, pools, false, '')
    expect(t.providers.map((o) => o.value)).toEqual(['', 'a', 'b'])
    expect(t.providers[0].label).toBe('Default (B)')
    expect(t.pools).toEqual([])
  })
  it('keeps a chosen provider that cannot run, marked', () => {
    const o = targetOptions(providers, pools, false, 'down').providers.find((x) => x.value === 'down')
    expect(o).toMatchObject({broken: true, note: 'not authenticated'})
  })
  it('marks an unknown provider and an unknown pool as not configured', () => {
    expect(targetOptions(providers, pools, false, 'gone').providers.at(-1)).toMatchObject({
      value: 'gone',
      note: 'not configured',
    })
    expect(targetOptions(providers, pools, false, 'pool:old').pools).toEqual([
      {value: 'pool:old', label: 'old', broken: true, note: 'not configured'},
    ])
  })
  it('offers beta providers and pools only with beta on, but keeps an existing pool', () => {
    expect(targetOptions(providers, pools, true, '').providers.map((o) => o.value)).toContain('labs')
    expect(targetOptions(providers, pools, true, '').pools).toHaveLength(1)
    expect(targetOptions(providers, pools, false, 'pool:team').pools).toHaveLength(1)
  })
})

describe('model provider and effort', () => {
  it('lets a pool speak through its first member', () => {
    expect(modelProvider('pool:team', pools)).toBe('a')
    expect(takesEffort('pool:team', providers, pools)).toBe(true)
  })
  it('reads the default provider for ""', () => {
    expect(takesEffort('', providers, pools)).toBe(false)
  })
  it('keeps a custom effort', () => {
    expect(effortOptions('max').at(-1)).toEqual({value: 'max', label: 'max (custom)'})
  })
})
