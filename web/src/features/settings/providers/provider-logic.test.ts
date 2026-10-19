import {describe, expect, it} from 'vitest'
import type {Provider, ProviderLimits} from '@/lib/types'
import {
  fallbackOptions,
  groupByKind,
  hasLimitsRow,
  healthInfo,
  limitStatusText,
  resetText,
  windowLeft,
  windowTone,
} from './provider-logic'

const prov = (id: string, enabled = true): Provider =>
  ({id, name: id.toUpperCase(), enabled, kind: 'claude-code'}) as Provider

describe('healthInfo', () => {
  it('maps known states and falls back for unknown ones', () => {
    expect(healthInfo('ready')).toEqual({tone: 'success', label: 'Ready'})
    expect(healthInfo('not_authenticated').tone).toBe('danger')
    expect(healthInfo('weird')).toEqual({tone: 'warning', label: 'weird'})
  })
})

describe('fallbackOptions', () => {
  it('offers the others, marks switched-off ones, and keeps a vanished stored id', () => {
    const opts = fallbackOptions('a', [prov('a'), prov('b'), prov('c', false)], 'gone')
    expect(opts.map((o) => o.value)).toEqual(['', 'b', 'c', 'gone'])
    expect(opts[2].label).toBe('C (switched off)')
    expect(opts[3].label).toBe('gone (unavailable)')
  })
  it('adds nothing when the stored id still exists', () => {
    expect(fallbackOptions('a', [prov('a'), prov('b')], 'b')).toHaveLength(2)
  })
})

describe('limits', () => {
  it('clamps the share left and colours it before it runs dry', () => {
    expect(windowLeft({id: 'w', label: 'W', used_percent: 120})).toBe(0)
    expect(windowTone(8)).toBe('danger')
    expect(windowTone(20)).toBe('warning')
    expect(windowTone(60)).toBe('accent')
  })
  it('says a past reset is now', () => {
    expect(resetText({id: 'w', label: 'W', used_percent: 1, resets_at: '2000-01-01T00:00:00Z'})).toBe(
      'Resets now',
    )
    expect(resetText({id: 'w', label: 'W', used_percent: 1})).toBe('')
  })
  it('explains a failed read with the age of the last one', () => {
    const now = Date.parse('2026-01-01T12:00:00Z')
    const l: ProviderLimits = {
      state: 'unavailable',
      reason: 'Token expired.',
      updated_at: '2026-01-01T10:00:00Z',
      checked_at: '',
    }
    expect(limitStatusText(l, now)).toBe('Token expired. Showing the reading from 2 h ago.')
    expect(limitStatusText({state: 'unavailable', checked_at: ''}, now)).toBe('Could not be read.')
  })
  it('shows a row only with windows or a failed read', () => {
    expect(hasLimitsRow(undefined)).toBe(false)
    expect(hasLimitsRow({state: 'unsupported', checked_at: ''})).toBe(false)
    expect(hasLimitsRow({state: 'unavailable', checked_at: ''})).toBe(true)
    expect(
      hasLimitsRow({state: 'ok', windows: [{id: 'w', label: 'W', used_percent: 1}], checked_at: ''}),
    ).toBe(true)
  })
})

describe('groupByKind', () => {
  it('puts the default first, its group first, the rest by name', () => {
    const p = (id: string, kind: string, dflt = false) =>
      ({id, name: id, kind, type_name: kind, default: dflt, enabled: true}) as Provider
    const groups = groupByKind([
      p('alpha', 'claude-code'),
      p('beta', 'codex'),
      p('zed', 'codex', true),
      p('aaa', 'codex'),
    ])
    expect(groups.map((g) => g.kind)).toEqual(['codex', 'claude-code'])
    expect(groups[0].providers.map((x) => x.id)).toEqual(['zed', 'aaa', 'beta'])
  })
})
