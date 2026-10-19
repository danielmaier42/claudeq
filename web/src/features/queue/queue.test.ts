import {describe, expect, it} from 'vitest'
import type {QueueTask} from '@/lib/types'
import {buildBlocks, moveGroup, placeTask, sameOrder, waitedFor} from './queue-model'
import {describeCron} from './schedule'

const task = (id: string, group = '', enabled = true) =>
  ({
    id,
    name: id,
    group,
    enabled,
    trigger: 'asap',
    prompt: '',
    working_dir: '',
    parallel: false,
    permissions: 'default',
    running: false,
  }) as QueueTask

const ids = (ts: QueueTask[]) => ts.map((t) => `${t.id}${t.group ? '@' + t.group : ''}`).join(' ')

describe('buildBlocks', () => {
  it('groups in first-appearance order and filters paused tasks', () => {
    const ts = [task('a'), task('b', 'x'), task('c', '', false), task('d', 'x')]
    expect(buildBlocks(ts, false).map((b) => [b.group, b.tasks.length])).toEqual([
      ['', 2],
      ['x', 2],
    ])
    expect(buildBlocks(ts, true)[0].tasks.map((t) => t.id)).toEqual(['a'])
  })
})

describe('placeTask', () => {
  const ts = [task('a'), task('b'), task('c', 'x'), task('d', 'x')]
  it('moves above and below a task in another group', () => {
    expect(ids(placeTask(ts, 'a', 'x', {before: 'd'}))).toBe('b c@x a@x d@x')
    expect(ids(placeTask(ts, 'd', '', {after: 'a'}))).toBe('a d b c@x')
  })
  it('appends to the end of a group, or the list for a new one', () => {
    expect(ids(placeTask(ts, 'a', 'x', {endOf: 'x'}))).toBe('b c@x d@x a@x')
    expect(ids(placeTask(ts, 'c', 'new', {endOf: 'new'}))).toBe('a b d@x c@new')
  })
  it('keeps groups together like the daemon does', () => {
    expect(ids(placeTask(ts, 'c', '', {after: 'd'}))).toBe('a b c d@x')
  })
})

describe('moveGroup', () => {
  const ts = [task('a'), task('b', 'x'), task('c', 'y')]
  it('moves a section in front of another or to the end', () => {
    expect(ids(moveGroup(ts, 'y', ''))).toBe('c@y a b@x')
    expect(ids(moveGroup(ts, 'x', null))).toBe('a c@y b@x')
  })
})

describe('sameOrder', () => {
  it('notices a group change without a position change', () => {
    expect(sameOrder([task('a')], [task('a', 'x')])).toBe(false)
    expect(sameOrder([task('a')], [task('a')])).toBe(true)
  })
})

describe('waitedFor', () => {
  it('is empty for the first minute, then minutes, then hours', () => {
    const now = Date.parse('2026-01-01T12:00:00Z')
    expect(waitedFor('2026-01-01T11:59:30Z', now)).toBe('')
    expect(waitedFor('2026-01-01T11:48:00Z', now)).toBe('12 min')
    expect(waitedFor('2026-01-01T09:30:00Z', now)).toBe('2 h')
  })
})

describe('describeCron', () => {
  it.each([
    ['* * * * *', 'Every minute'],
    ['*/15 * * * *', 'Every 15 minutes'],
    ['5 * * * *', 'Every hour at :05'],
    ['0 */4 * * *', 'Every 4 hours'],
    ['0 3 * * *', 'Every day at 03:00'],
    ['30 9 * * 1-5', 'Weekdays at 09:30'],
    ['0 10 * * 6,0', 'Weekends at 10:00'],
    ['0 8 * * MON,wed', 'Mondays, Wednesdays at 08:00'],
    ['0 9,17 * * *', 'Every day at 09:00 and 17:00'],
    ['0 6 1 * *', 'Monthly on the 1st at 06:00'],
    ['0 6 22 * *', 'Monthly on the 22nd at 06:00'],
  ])('%s', (expr, want) => expect(describeCron(expr)).toBe(want))

  it('gives up on what it cannot say simply', () => {
    expect(describeCron('0 3 1 1 *')).toBeNull()
    expect(describeCron('*/5 9-17 * * 1-5')).toBeNull()
    expect(describeCron('bogus')).toBeNull()
  })
})
