import {describe, expect, it} from 'vitest'
import type {QueueTask, RunStatus} from '@/lib/types'
import {buildBlocks, moveGroup, placeTask, sameOrder, waitedFor} from './queue-model'
import {matchesWords} from '@/lib/search'
import {lastOutcome} from './outcome'
import {searchTexts} from './TaskChips'
import type {QueueContext} from './use-queue-context'
import {describeCron, nextRunText} from './schedule'

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
    ['0 4 1 * *', 'Monthly on the 1st at 04:00'],
    ['0 0 1 1 *', 'Yearly on 1 Jan at 00:00'],
    ['30 7 24 DEC *', 'Yearly on 24 Dec at 07:30'],
    ['0 0 31 2 *', null],
    ['0 0 29 2 *', 'Yearly on 29 Feb at 00:00'],
  ])('%s', (expr, want) => expect(describeCron(expr)).toBe(want))

  it('gives up on what it cannot say simply', () => {
    expect(describeCron('0 3 * 1 *')).toBeNull()
    expect(describeCron('0 3 1 1-3 *')).toBeNull()
    expect(describeCron('0 3 1 13 *')).toBeNull()
    expect(describeCron('0 3 1 1 1')).toBeNull()
    expect(describeCron('*/5 9-17 * * 1-5')).toBeNull()
    expect(describeCron('bogus')).toBeNull()
  })
})

describe('nextRunText', () => {
  const now = new Date(2026, 9, 19, 12, 0).getTime()
  it('is relative within the week', () => {
    expect(nextRunText(new Date(2026, 9, 20, 12, 0).toISOString(), now)).toBe('in 1 d')
  })
  it('spells out a far date as day month year', () => {
    expect(nextRunText(new Date(2027, 0, 1, 0, 0).toISOString(), now)).toBe('1 Jan 2027')
  })
})

describe('lastOutcome', () => {
  const now = Date.parse('2026-10-19T12:00:00Z')
  it('says how the last run ended and when', () => {
    const run = (status: RunStatus, finished?: string) => ({
      run_id: 'r1',
      status,
      started_at: '2026-10-19T10:30:00Z',
      finished_at: finished,
    })
    expect(lastOutcome(run('failed', '2026-10-19T11:00:00Z'), now)).toEqual({
      runId: 'r1',
      tone: 'danger',
      text: 'last: failed 1 h ago',
      quiet: false,
    })
    expect(lastOutcome(run('success'), now)).toEqual({
      runId: 'r1',
      tone: 'success',
      text: 'last: succeeded 1 h ago',
      quiet: true,
    })
  })
  it('stays out of the way for a task that never ran or is running', () => {
    expect(lastOutcome(undefined, now)).toBeNull()
    expect(lastOutcome({run_id: 'r3', status: 'running', started_at: '2026-10-19T11:59:00Z'}, now)).toBeNull()
  })
})

describe('searchTexts', () => {
  const ctx = {
    runsOn: () => 'Claude Code · Opus',
    beta: () => null,
    liveRun: () => undefined,
    lastRun: () => undefined,
    limitedUntil: undefined,
  } satisfies QueueContext
  it('covers name, group, provider, schedule and chips', () => {
    const t = {
      ...task('nightly', 'Ops'),
      trigger: 'cron',
      cron: '0 3 * * *',
      parallel: true,
      enabled: false,
    } as QueueTask
    const texts = searchTexts(t, ctx)
    for (const q of ['nightly', 'ops', 'opus', 'every day at 03', 'parallel', 'paused'])
      expect(matchesWords(q, ...texts), q).toBe(true)
    expect(matchesWords('script', ...texts)).toBe(false)
  })
})
