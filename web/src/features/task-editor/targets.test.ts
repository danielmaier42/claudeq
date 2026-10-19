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
  prov('codex', {kind: 'codex'}),
]
const pools: Pool[] = [{id: 'team', name: 'Team', members: [{provider: 'a', weight: 1}], ranking: []}]

describe('targetOptions', () => {
  it('offers ready providers sorted, with the default first', () => {
    const t = targetOptions(providers, pools, '')
    expect(t.providers.map((o) => o.value)).toEqual(['', 'a', 'b', 'codex'])
    expect(t.providers[0].label).toBe('Default (B)')
  })
  it('keeps a chosen provider that cannot run, marked', () => {
    const o = targetOptions(providers, pools, 'down').providers.find((x) => x.value === 'down')
    expect(o).toMatchObject({broken: true, note: 'not authenticated'})
  })
  it('marks an unknown provider and an unknown pool as not configured', () => {
    expect(targetOptions(providers, pools, 'gone').providers.at(-1)).toMatchObject({
      value: 'gone',
      note: 'not configured',
    })
    expect(targetOptions(providers, pools, 'pool:old').pools.at(-1)).toEqual({
      value: 'pool:old',
      label: 'old',
      broken: true,
      note: 'not configured',
    })
  })
  it('offers every pool next to the providers', () => {
    expect(targetOptions(providers, pools, '').pools).toEqual([{value: 'pool:team', label: 'Team'}])
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
