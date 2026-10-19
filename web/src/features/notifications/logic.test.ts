import {describe, expect, it} from 'vitest'
import type {AppNotification} from '@/lib/types'
import {kindOptions, NO_FILTERS, shortTitle, shown, targetLabel} from './logic'

const note = (p: Partial<AppNotification>): AppNotification => ({
  id: 'n',
  kind: 'failure',
  title: 'ClaudeQ ✗ nightly failed',
  message: 'exit 1',
  sent_at: '2026-10-19T08:00:00Z',
  unread: false,
  ...p,
})

describe('shortTitle', () => {
  it('drops the sender and the glyph', () => {
    expect(shortTitle('ClaudeQ ✗ nightly failed')).toBe('nightly failed')
    expect(shortTitle('ClaudeQ: 📄 Report')).toBe('Report')
    expect(shortTitle('Provider down')).toBe('Provider down')
  })
})

describe('shown', () => {
  it('searches title, message, task and kind label', () => {
    const n = note({task_name: 'Backup'})
    expect(shown(n, {...NO_FILTERS, query: 'backup failed'})).toBe(true)
    expect(shown(n, {...NO_FILTERS, query: 'run failed exit'})).toBe(true)
    expect(shown(n, {...NO_FILTERS, query: 'succeeded'})).toBe(false)
  })
  it('filters by unread and kind', () => {
    expect(shown(note({}), {...NO_FILTERS, unread: true})).toBe(false)
    expect(shown(note({}), {...NO_FILTERS, kind: 'success'})).toBe(false)
  })
})

describe('kindOptions', () => {
  const notes = [note({id: '1'}), note({id: '2', kind: 'artifact', unread: true})]
  it('offers the kinds there are and drops a stale choice', () => {
    expect(kindOptions(notes, NO_FILTERS).options.map((o) => o.label)).toEqual(['New artifact', 'Run failed'])
    expect(kindOptions(notes, {...NO_FILTERS, kind: 'provider'}).kind).toBe('')
    expect(kindOptions(notes, {...NO_FILTERS, unread: true}).options.map((o) => o.value)).toEqual([
      'artifact',
    ])
  })
})

describe('targetLabel', () => {
  it('names what a click opens', () => {
    expect(targetLabel(note({artifact_id: 'a', run_id: 'r'}))).toBe('Opens the artifact')
    expect(targetLabel(note({run_id: 'r'}))).toBe('Opens the run’s log')
    expect(targetLabel(note({}))).toBe('')
  })
})
