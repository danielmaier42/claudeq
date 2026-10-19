import {describe, expect, it} from 'vitest'
import type {Run} from '@/lib/types'
import {
  canContinue,
  canTakeOver,
  cancelMode,
  emptyFilters,
  filterMenus,
  groupWorkflows,
  runMatches,
  staleChoices,
  statusLabel,
} from './runs'

const run = (p: Partial<Run>): Run => ({
  run_id: 'r1',
  task_id: 't1',
  task_name: 'Nightly review',
  started_at: '2026-10-19T08:00:00Z',
  status: 'success',
  exit_code: 0,
  log_path: '',
  unread: false,
  ...p,
})

const task = {
  id: 't1',
  name: 'Nightly review',
  prompt: '',
  working_dir: '/Users/me/src/app',
  group: 'Work',
  trigger: 'manual',
  parallel: false,
  enabled: true,
  permissions: {},
} as unknown as Run['task']

describe('statusLabel', () => {
  it('calls a rate-limited run rescheduled only while its resume is pending', () => {
    expect(statusLabel({status: 'rate_limited_waiting', resume_pending: true})).toBe('rescheduled')
    expect(statusLabel({status: 'rate_limited_waiting'})).toBe('rate limited')
    expect(statusLabel({status: 'auth_error'})).toBe('auth error')
  })
})

describe('runMatches', () => {
  it('searches both spellings of the outcome and the folder', () => {
    const r = run({status: 'rate_limited_waiting', resume_pending: true, task})
    expect(runMatches(r, {...emptyFilters, query: 'rate limited'})).toBe(true)
    expect(runMatches(r, {...emptyFilters, query: 'rescheduled app'})).toBe(true)
    expect(runMatches(r, {...emptyFilters, query: 'failed'})).toBe(false)
  })

  it('applies the segment, the group and the date range', () => {
    const r = run({task})
    expect(runMatches(r, {...emptyFilters, show: ''})).toBe(true)
    expect(runMatches(r, {...emptyFilters, show: 'running'})).toBe(false)
    expect(runMatches(r, {...emptyFilters, group: 'Work'})).toBe(true)
    expect(runMatches(run({}), {...emptyFilters, group: ' none'})).toBe(true)
    expect(runMatches(r, {...emptyFilters, from: '2026-10-20'})).toBe(false)
  })
})

describe('filterMenus', () => {
  it('offers only tasks of the chosen group and drops stale choices', () => {
    const runs = [
      run({run_id: 'a', task}),
      run({run_id: 'b', task_id: 't2', task_name: 'Loose', status: 'failed'}),
    ]
    const m = filterMenus(runs, {...emptyFilters, group: 'Work'})
    expect(m.tasks.map((t) => t.value)).toEqual(['t1'])
    expect(m.groups.map((g) => g.label)).toEqual(['Work', 'No group'])
    expect(staleChoices({...emptyFilters, status: 'canceled', task: 't1'}, m)).toEqual({status: ''})
  })
})

describe('groupWorkflows', () => {
  it('boxes a workflow with several runs, oldest first, and indents a join', () => {
    const runs = [
      run({run_id: 'join', workflow_id: 'w', parent_run_id: 'fan', started_at: '2026-10-19T09:00:00Z'}),
      run({run_id: 'solo', workflow_id: 'solo'}),
      run({run_id: 'fan', workflow_id: 'w', started_at: '2026-10-19T07:00:00Z'}),
    ]
    const out = groupWorkflows(runs)
    expect(out.map((e) => e.kind)).toEqual(['workflow', 'run'])
    const wf = out[0]
    if (wf.kind !== 'workflow') throw new Error('expected a workflow')
    expect(wf.runs.map((w) => [w.run.run_id, w.depth])).toEqual([
      ['fan', 0],
      ['join', 1],
    ])
  })
})

describe('run actions', () => {
  it('offers continue, take over and cancel by state', () => {
    const done = run({session_id: 's', task})
    expect(canContinue(done)).toBe(true)
    expect(canTakeOver(done)).toBe(false)
    const live = run({status: 'running', session_id: 's', task})
    expect(canTakeOver(live)).toBe(true)
    expect(canTakeOver({...live, provider: {kind: 'script'}})).toBe(false)
    expect(cancelMode(live)).toBe('running')
    expect(cancelMode(run({status: 'rate_limited_waiting', resume_pending: true}))).toBe('resume')
    expect(canContinue(run({status: 'rate_limited_waiting', session_id: 's', task}))).toBe(false)
  })
})
