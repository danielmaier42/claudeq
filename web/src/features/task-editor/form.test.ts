import {describe, expect, it} from 'vitest'
import type {Task} from '@/lib/types'
import {
  buildPayload,
  carriedFields,
  fieldForError,
  formFromTask,
  validate,
  withKind,
  withTarget,
  withTrigger,
} from './form'

const task: Task = {
  id: 'nightly-1a2b3c',
  name: 'Nightly',
  prompt: 'Build it',
  working_dir: '/tmp/x',
  group: 'Ops',
  trigger: 'cron',
  cron: '0 20 * * *',
  parallel: true,
  enabled: false,
  pool: 'team',
  model: 'opus',
  reasoning_effort: 'high',
  permissions: 'skip',
  depends_on: ['a'],
  include_results: true,
}

describe('formFromTask', () => {
  it('folds a pool into the target and drops fields of other triggers', () => {
    const f = formFromTask({...task, fixed_at: '2026-01-01T10:00:00Z'})
    expect(f.target).toBe('pool:team')
    expect(f.fixedAt).toBe('')
    expect(f.cron).toBe('0 20 * * *')
    expect(f.skipPermissions).toBe(true)
  })
  it('defaults an empty task to an asap agent job', () => {
    const f = formFromTask({})
    expect(f.kind).toBe('agent')
    expect(f.trigger).toBe('asap')
  })
})

describe('withKind / withTarget', () => {
  it('clears what a script cannot have', () => {
    const f = withKind({...formFromTask(task), backfill: true}, 'script')
    expect(f.skipPermissions).toBe(false)
    expect(f.backfill).toBe(false)
  })
  it('does not carry a model to another provider', () => {
    const f = withTarget(formFromTask(task), 'codex')
    expect(f.model).toBe('')
    expect(f.effort).toBe('')
  })
})

describe('withTrigger', () => {
  it('prefills an empty schedule with the nightly one', () => {
    expect(withTrigger(formFromTask({}), 'cron').cron).toBe('0 3 * * *')
  })
  it('keeps a schedule already typed', () => {
    const f = {...formFromTask({}), cron: '0 9 * * 1'}
    expect(withTrigger(withTrigger(f, 'asap'), 'cron').cron).toBe('0 9 * * 1')
  })
})

describe('validate', () => {
  it('names every missing field', () => {
    const e = validate({...formFromTask({}), trigger: 'fixed'})
    expect(Object.keys(e).sort()).toEqual(['fixedAt', 'prompt', 'workingDir'])
  })
  it('refuses a group name longer than the daemon keeps', () => {
    expect(validate({...formFromTask({}), group: 'x'.repeat(61)}).group).toMatch(/60/)
  })
  it('says script for a script', () => {
    expect(validate({...formFromTask({kind: 'script'})}).prompt).toMatch(/script/)
  })
})

describe('buildPayload', () => {
  it('keeps what the editor does not show on an edit', () => {
    const p = buildPayload(formFromTask(task), {takesEffort: true, carried: carriedFields(task)})
    expect(p).toMatchObject({
      group: 'Ops',
      depends_on: ['a'],
      include_results: true,
      enabled: false,
      id: task.id,
    })
    expect(p.pool).toBe('team')
    expect(p.provider).toBeUndefined()
    expect(p.reasoning_effort).toBe('high')
    expect(p.fixed_at).toBeUndefined()
  })
  it('leaves agent-only fields off a script', () => {
    const f = {...withKind(formFromTask(task), 'script'), backfill: true}
    const p = buildPayload(f, {takesEffort: true})
    expect(p.kind).toBe('script')
    expect(p.model).toBeUndefined()
    expect(p.pool).toBeUndefined()
    expect(p.reasoning_effort).toBeUndefined()
    expect(p.backfill).toBe(false)
    expect(p.permissions).toBe('default')
  })
  it('drops the effort where the harness takes none', () => {
    expect(buildPayload(formFromTask(task), {takesEffort: false}).reasoning_effort).toBeUndefined()
  })
  it('sends the group on an add, empty for none', () => {
    expect(buildPayload({...formFromTask(task), group: ' Night '}, {takesEffort: false}).group).toBe('Night')
    expect(buildPayload(formFromTask({}), {takesEffort: false}).group).toBe('')
  })
  it('sends an edited group only when it was changed', () => {
    const f = formFromTask(task)
    expect(buildPayload(f, {takesEffort: false, keptGroup: 'Ops'}).group).toBeUndefined()
    expect(buildPayload({...f, group: 'Night'}, {takesEffort: false, keptGroup: 'Ops'}).group).toBe('Night')
    // Clearing it is a change too: the task leaves its group.
    expect(buildPayload({...f, group: ''}, {takesEffort: false, keptGroup: 'Ops'}).group).toBe('')
  })
  it('sends a fixed time as an ISO instant', () => {
    const p = buildPayload(
      {...formFromTask({}), trigger: 'fixed', fixedAt: '2026-10-19T20:00'},
      {takesEffort: false},
    )
    expect(p.fixed_at).toBe(new Date('2026-10-19T20:00').toISOString())
  })
})

describe('fieldForError', () => {
  it.each([
    ['invalid task: missing working_dir', 'workingDir'],
    ['invalid task: missing script', 'prompt'],
    ['invalid task: trigger "fixed" requires fixed_at', 'fixedAt'],
    ['invalid task: the hour field "99" is not valid (hour accepts 0-23)', 'cron'],
    ['provider "x" is not ready', 'target'],
    ['invalid group: at most 60 characters', 'group'],
    ['disk full', null],
  ])('%s', (msg, field) => expect(fieldForError(msg)).toBe(field))
})
