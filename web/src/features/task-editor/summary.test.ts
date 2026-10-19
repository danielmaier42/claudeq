import {describe, expect, it} from 'vitest'
import {triggerSummary} from './summary'

const now = new Date(2026, 9, 19, 12, 0)
const agent = {kind: 'agent', trigger: 'asap', cron: '', fixedAt: ''} as const

describe('triggerSummary', () => {
  it.each([
    [
      {...agent, trigger: 'cron', cron: '0 7 * * 1-5'},
      'Claude Code · Opus',
      'Runs every weekday at 07:00 on Claude Code · Opus',
    ],
    [agent, 'Pool Claude', 'Runs as soon as possible on pool Claude'],
    [
      {...agent, kind: 'script', trigger: 'fixed', fixedAt: '2026-10-21T09:00'},
      '',
      'Runs once on 21 Oct, 09:00 as a script',
    ],
    [
      {...agent, trigger: 'fixed', fixedAt: '2027-01-02T08:30'},
      'Codex',
      'Runs once on 2 Jan 2027, 08:30 on Codex',
    ],
    [{...agent, trigger: 'fixed'}, 'Codex', 'Runs once (no time chosen yet) on Codex'],
    [{...agent, trigger: 'cron', cron: '0 3 * * *'}, 'Codex', 'Runs every day at 03:00 on Codex'],
    [{...agent, trigger: 'cron', cron: '0 8 * * 1'}, 'Codex', 'Runs on Mondays at 08:00 on Codex'],
    [{...agent, trigger: 'cron', cron: '0 4 1 * *'}, 'Codex', 'Runs monthly on the 1st at 04:00 on Codex'],
    [
      {...agent, trigger: 'cron', cron: '*/5 9-17 * * 1-5'},
      'Codex',
      'Runs on the schedule */5 9-17 * * 1-5 on Codex',
    ],
    [{...agent, trigger: 'cron'}, 'Codex', 'Runs repeatedly (no schedule yet) on Codex'],
  ] as const)('%o', (form, runsOn, want) => expect(triggerSummary(form, runsOn, now)).toBe(want))
})
