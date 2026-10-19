import {describe, expect, it} from 'vitest'
import type {LimitsEntry, ProviderLimits} from '@/lib/types'
import {
  leftLevel,
  limitStatusText,
  newestReading,
  noLimits,
  readFailed,
  readText,
  rowAge,
  windowSlots,
} from './limits'

const entry = (limits: ProviderLimits): LimitsEntry => ({
  id: 'p',
  name: 'p',
  type_name: 'Claude',
  default: false,
  limits,
  backfill: false,
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
  it('a failed read names its reason and the age of what is shown', () => {
    const t = Date.parse('2026-10-19T12:00:00Z')
    const l = {
      state: 'unavailable' as const,
      reason: 'Token expired.',
      updated_at: '2026-10-19T11:50:00Z',
      checked_at: '',
    }
    expect(limitStatusText(l, t)).toBe('Token expired. Showing the reading from 10 min ago.')
    expect(readFailed(l)).toBe(true)
  })
  it('an API-key account has no plan limits, which is no failure', () => {
    const l: ProviderLimits = {
      state: 'unsupported',
      reason: 'No plan limits for API-key accounts',
      checked_at: '',
    }
    expect(noLimits(l)).toBe(true)
    expect(readFailed(l)).toBe(false)
    expect(limitStatusText(l)).toBe('No plan limits for API-key accounts')
  })
  it('heads the strip with the newest reading and lets only lagging rows name their own', () => {
    const t = Date.parse('2026-10-19T12:00:00Z')
    const fresh = {state: 'ok' as const, updated_at: '2026-10-19T11:57:00Z', checked_at: ''}
    const old = {state: 'unavailable' as const, updated_at: '2026-10-19T09:00:00Z', checked_at: ''}
    const newest = newestReading([entry(old), entry(fresh), entry({state: 'unsupported', checked_at: ''})])
    expect(newest).toBe(fresh.updated_at)
    expect(readText(newest, t)).toBe('Read 3 min ago · refreshes every 15 min')
    expect(readText(undefined, t)).toBe('Refreshes every 15 min')
    expect(readText('2026-10-19T12:00:20Z', t)).toBe('Read just now · refreshes every 15 min')
    expect(rowAge(fresh, newest)).toBeUndefined()
    expect(rowAge(old, newest)).toBe(old.updated_at)
  })
})
