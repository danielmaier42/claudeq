import {describe, expect, it, vi} from 'vitest'
import type {Settings, UpdateStatus} from '@/lib/types'
import {combineEdits, editedSections, fieldError, hasEdits, mergeSettings, pruneEdits} from './draft'
import {choiceOptions, IDLE, idleValue} from './options'
import {pickSection} from './sections'
import {reinstallSummary} from './update'

// update.ts toasts, and the toaster reads the color scheme jsdom does not have.
vi.mock('@/components/ui/toaster', () => ({toast: vi.fn()}))

const base: Settings = {
  default_provider: 'claude',
  heartbeat_minutes: 60,
  pushover: {enabled: false, token: 'tok', user_key: 'usr'},
  ntfy: {enabled: false, server: '', topic: '', token: ''},
  webhook: {enabled: false, url: '', template: ''},
  idle_timeout_minutes: 0,
  max_run_history: 0,
  system_prompt: '',
  backfill_urgency: 0,
  paused: false,
  prompt_review_disabled: false,
  artifact_notify_disabled: false,
  prompt_review_provider: '',
  prompt_review_model: '',
  assistant_provider: '',
  assistant_model: '',
  default_working_dir: '',
  appearance: '',
  accent: '',
}

describe('draft', () => {
  it('lays edits over the latest settings without touching other fields', () => {
    const latest = {...base, appearance: 'dark' as const, paused: true}
    const out = mergeSettings(latest, {system_prompt: 'x', pushover: {enabled: true}})
    expect(out.appearance).toBe('dark')
    expect(out.paused).toBe(true)
    expect(out.system_prompt).toBe('x')
    expect(out.pushover).toEqual({enabled: true, token: 'tok', user_key: 'usr'})
  })

  it('combines channel edits field by field', () => {
    expect(combineEdits({ntfy: {topic: 'a'}}, {ntfy: {server: 'b'}})).toEqual({
      ntfy: {topic: 'a', server: 'b'},
    })
  })

  it('prunes edits that match what is stored', () => {
    const e = pruneEdits(base, {
      system_prompt: '',
      pushover: {token: 'tok', user_key: 'new'},
      ntfy: {topic: ''},
    })
    expect(e).toEqual({pushover: {user_key: 'new'}})
    expect(hasEdits(pruneEdits(base, {heartbeat_minutes: 60}))).toBe(false)
  })

  it('maps edits to their sections', () => {
    expect([...editedSections({system_prompt: 'x', webhook: {url: 'u'}})]).toEqual([
      'general',
      'notifications',
    ])
  })

  it('places API validation errors at their field', () => {
    expect(fieldError('pushover: the API token is required')).toEqual({
      field: 'pushover.token',
      text: 'The API token is required',
    })
    expect(fieldError('ntfy topic: required (the name subscribers listen on)')?.field).toBe('ntfy.topic')
    expect(fieldError('webhook body: the template does not produce valid JSON')?.field).toBe(
      'webhook.template',
    )
    expect(fieldError('webhook URL: required')?.field).toBe('webhook.url')
    expect(fieldError('appearance "x": want ...')).toBeNull()
  })
})

describe('options', () => {
  it('keeps a value set outside the list', () => {
    expect(choiceOptions(IDLE, 45, (m) => `${m} minutes`)[0]).toEqual({value: '45', label: '45 minutes'})
    expect(choiceOptions(IDLE, idleValue(0), String)).toHaveLength(IDLE.length)
  })
})

describe('sections', () => {
  it('prefers the requested section, then the last one, then About while an update waits', () => {
    expect(pickSection({requested: 'system', last: 'general', updatePending: true})).toBe('system')
    expect(pickSection({requested: 'pools', last: 'notifications', updatePending: false})).toBe('pools')
    expect(pickSection({requested: '', last: 'notifications', updatePending: true})).toBe('notifications')
    expect(pickSection({requested: '', last: '', updatePending: true})).toBe('about')
    expect(pickSection({requested: 'nope', last: '', updatePending: false})).toBe('general')
  })
})

describe('reinstall', () => {
  const dev: UpdateStatus = {
    current: 'dev-main',
    supported: false,
    available: false,
    latest: '2.1.0',
    has_installer: true,
    reinstall: true,
    data_floor: '2.0.0',
    checking: false,
    downloading: false,
  }

  it('offers the latest release to a dev build', () => {
    expect(reinstallSummary(dev)?.label).toBe('Reinstall DispatchQ 2.1.0')
  })

  it('names the floor when the release cannot read the data', () => {
    const s = reinstallSummary({...dev, latest: '1.6.2', reinstall: false})
    expect(s?.description).toContain('2.0.0 or later')
  })

  it('says so when the release has no installer', () => {
    expect(reinstallSummary({...dev, reinstall: false, has_installer: false})?.description).toContain(
      'no installer',
    )
  })

  it('offers nothing to a release build or before a check', () => {
    expect(reinstallSummary({...dev, supported: true, current: '2.0.0'})).toBeUndefined()
    expect(reinstallSummary({...dev, latest: ''})).toBeUndefined()
  })
})
