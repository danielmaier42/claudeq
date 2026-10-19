import {describe, expect, it} from 'vitest'
import {chatTranscript, parseLog} from './parse-log'

const lines = (...evs: unknown[]) =>
  evs.map((e) => (typeof e === 'string' ? e : JSON.stringify(e))).join('\n')

describe('parseLog', () => {
  it('reads a Claude stream: messages, tools with their results, the result', () => {
    const entries = parseLog(
      lines(
        {type: 'system', subtype: 'init', model: 'claude-opus'},
        {type: 'assistant', message: {content: [{type: 'text', text: 'Looking.'}]}},
        {
          type: 'assistant',
          message: {content: [{type: 'tool_use', id: 't1', name: 'Bash', input: {command: 'ls -la'}}]},
        },
        {
          type: 'user',
          message: {
            content: [{type: 'tool_result', tool_use_id: 't1', content: [{type: 'text', text: 'a\nb'}]}],
          },
        },
        {type: 'assistant', message: {content: [{type: 'thinking', thinking: ''}]}},
        {type: 'result', result: 'Done.', is_error: false, total_cost_usd: 0.5, num_turns: 3},
        'not json',
      ),
    )
    expect(entries.map((e) => e.kind)).toEqual(['meta', 'message', 'steps', 'result', 'raw'])
    const steps = entries[2]
    if (steps.kind !== 'steps') throw new Error('expected steps')
    expect(steps.steps).toEqual([
      {
        kind: 'tool',
        id: 't1',
        name: 'Bash',
        summary: 'ls -la',
        input: '{\n  "command": "ls -la"\n}',
        output: 'a\nb',
        error: false,
      },
    ])
    expect(entries[3]).toMatchObject({kind: 'result', costUsd: 0.5, turns: 3})
  })

  it('folds a subagent into the tool activity instead of the conversation', () => {
    const entries = parseLog(
      lines(
        {type: 'user', parent_tool_use_id: 'x', message: {content: [{type: 'text', text: 'Do the thing'}]}},
        {type: 'assistant', parent_tool_use_id: 'x', message: {content: [{type: 'text', text: 'Did it'}]}},
      ),
    )
    expect(entries).toEqual([
      {
        kind: 'steps',
        steps: [
          {kind: 'note', who: 'Subagent prompt', text: 'Do the thing'},
          {kind: 'note', who: 'Subagent', text: 'Did it'},
        ],
      },
    ])
  })

  it('reads dispatchq (and older claudeq) notes, opencode and codex events', () => {
    const entries = parseLog(
      lines(
        {type: 'dispatchq_status', status: 'note', message: 'Pool chose A'},
        {type: 'claudeq_status', status: 'failed', message: 'exit 1'},
        {type: 'text', part: {text: 'hi from opencode'}},
        {type: 'item.completed', item: {type: 'agent_message', text: 'hi from codex'}},
        {type: 'item.completed', item: {type: 'command_execution', command: 'make', aggregated_output: 'ok'}},
        {type: 'turn.failed', error: {message: 'boom'}},
      ),
    )
    expect(entries).toEqual([
      {kind: 'status', label: 'note', text: 'Pool chose A', tone: 'note'},
      {kind: 'status', label: 'failed', text: 'exit 1', tone: 'error'},
      {kind: 'message', role: 'assistant', who: 'opencode', text: 'hi from opencode'},
      {kind: 'message', role: 'assistant', who: 'Codex', text: 'hi from codex'},
      {kind: 'steps', steps: [{kind: 'tool', name: 'bash', summary: 'make', input: 'make', output: 'ok'}]},
      {kind: 'status', label: 'error', text: 'boom', tone: 'error'},
    ])
  })

  it('keeps a result whose call is not in the log', () => {
    const entries = parseLog(
      lines({
        type: 'user',
        message: {content: [{type: 'tool_result', tool_use_id: 'gone', content: 'x', is_error: true}]},
      }),
    )
    expect(entries).toEqual([
      {
        kind: 'steps',
        steps: [{kind: 'tool', name: 'result', summary: 'x', input: '', output: 'x', error: true}],
      },
    ])
  })
})

describe('chatTranscript', () => {
  it('is the conversation without the tool noise', () => {
    const entries = parseLog(
      lines({
        type: 'assistant',
        message: {
          content: [
            {type: 'text', text: 'Hi'},
            {type: 'tool_use', id: 'a', name: 'Read', input: {}},
          ],
        },
      }),
    )
    expect(chatTranscript('Do it', entries)).toBe('You:\nDo it\n\nClaude:\nHi')
  })
})
