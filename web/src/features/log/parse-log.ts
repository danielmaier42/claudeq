// parseLog turns a run's log into what the chat view shows. The log is JSONL:
// Claude Code's stream-json, or the verbatim event streams of opencode and
// Codex (their adapters write them unchanged), plus dispatchq's own status lines.
// A line that is not JSON is kept as it is.
//
// Tool activity between two messages folds into one "steps" entry, so a run
// with thousands of tool calls still reads as the conversation it was. A
// subagent's chatter (events carrying a parent_tool_use_id) goes there too:
// it is part of the work, not a turn of the conversation.

export interface ToolStep {
  kind: 'tool'
  id?: string
  name: string
  // One line naming what the call was about: the command, the file, the pattern.
  summary: string
  input: string
  output?: string
  error?: boolean
}

export interface NoteStep {
  kind: 'note'
  who: string
  text: string
}

export type Step = ToolStep | NoteStep

export type LogEntry =
  | {kind: 'message'; role: 'user' | 'assistant'; who: string; text: string}
  | {kind: 'steps'; steps: Step[]}
  | {kind: 'result'; text: string; error: boolean; costUsd?: number; turns?: number; durationMs?: number}
  | {kind: 'status'; label: string; text: string; tone: 'note' | 'error'}
  | {kind: 'meta'; text: string}
  | {kind: 'raw'; text: string}

type Json = Record<string, unknown>

const isObj = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v)
const str = (v: unknown): string => (typeof v === 'string' ? v : '')

export function pretty(v: unknown): string {
  if (typeof v === 'string') return v
  if (v === undefined) return ''
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

function resultText(c: unknown): string {
  if (typeof c === 'string') return c
  if (Array.isArray(c))
    return c
      .map((x) => (isObj(x) && typeof x.text === 'string' ? x.text : typeof x === 'string' ? x : pretty(x)))
      .join('\n')
  return c == null ? '' : pretty(c)
}

const SUMMARY_FIELDS = [
  'command',
  'file_path',
  'path',
  'pattern',
  'url',
  'query',
  'description',
  'prompt',
  'filePath',
]

export function toolSummary(input: unknown): string {
  if (typeof input === 'string') return firstLine(input)
  if (!isObj(input)) return ''
  for (const k of SUMMARY_FIELDS) {
    const v = input[k]
    if (typeof v === 'string' && v.trim()) return firstLine(v)
  }
  return ''
}

const firstLine = (s: string) => {
  const line = s.trim().split('\n')[0]
  return line.length > 160 ? line.slice(0, 160) + '…' : line
}

class Builder {
  entries: LogEntry[] = []
  private tools = new Map<string, ToolStep>()

  push(e: LogEntry) {
    this.entries.push(e)
  }

  step(s: Step) {
    const last = this.entries[this.entries.length - 1]
    if (last?.kind === 'steps') last.steps.push(s)
    else this.entries.push({kind: 'steps', steps: [s]})
    if (s.kind === 'tool' && s.id) this.tools.set(s.id, s)
  }

  // A result lands on the call it answers; one whose call is not in the log
  // (it began before a resume, say) stands on its own.
  result(id: string, output: string, error: boolean) {
    const t = id ? this.tools.get(id) : undefined
    if (t) {
      t.output = output
      t.error = error
    } else this.step({kind: 'tool', name: 'result', summary: firstLine(output), input: '', output, error})
  }
}

export function parseLog(raw: string): LogEntry[] {
  const b = new Builder()
  for (const rawLine of raw.split('\n')) {
    const line = rawLine.trim()
    if (!line) continue
    let ev: unknown
    try {
      ev = JSON.parse(line)
    } catch {
      b.push({kind: 'raw', text: line})
      continue
    }
    if (isObj(ev)) addEvent(b, ev)
    else b.push({kind: 'raw', text: line})
  }
  return b.entries
}

function addEvent(b: Builder, ev: Json) {
  const type = str(ev.type)
  const msg = isObj(ev.message) ? ev.message : undefined
  const sub = !!ev.parent_tool_use_id

  if (type === 'assistant' && Array.isArray(msg?.content)) {
    for (const c of msg.content) {
      if (!isObj(c)) continue
      const text = str(c.text)
      if (c.type === 'text' && text.trim()) {
        if (sub) b.step({kind: 'note', who: 'Subagent', text})
        else b.push({kind: 'message', role: 'assistant', who: 'Claude', text})
      } else if (c.type === 'tool_use') {
        const name = str(c.name) || 'tool'
        b.step({kind: 'tool', id: str(c.id), name, summary: toolSummary(c.input), input: pretty(c.input)})
      }
    }
    return
  }
  if (type === 'user' && Array.isArray(msg?.content)) {
    for (const c of msg.content) {
      if (!isObj(c)) continue
      const text = str(c.text)
      if (c.type === 'tool_result') b.result(str(c.tool_use_id), resultText(c.content), c.is_error === true)
      else if (c.type === 'text' && text.trim()) {
        if (sub) b.step({kind: 'note', who: 'Subagent prompt', text})
        else b.push({kind: 'message', role: 'user', who: 'You', text})
      }
    }
    return
  }
  if (type === 'user' && typeof msg?.content === 'string' && msg.content.trim()) {
    if (sub) b.step({kind: 'note', who: 'Subagent prompt', text: msg.content})
    else b.push({kind: 'message', role: 'user', who: 'You', text: msg.content})
    return
  }
  if (type === 'result') {
    b.push({
      kind: 'result',
      text: str(ev.result) || '(no text)',
      error: ev.is_error === true,
      costUsd: typeof ev.total_cost_usd === 'number' ? ev.total_cost_usd : undefined,
      turns: typeof ev.num_turns === 'number' ? ev.num_turns : undefined,
      durationMs: typeof ev.duration_ms === 'number' ? ev.duration_ms : undefined,
    })
    return
  }
  // A dispatchq remark about the run itself (a dropped session, a pool choice)
  // is not a failure, so it does not borrow the failure colour. Logs written
  // before the rename say claudeq_status.
  if (type === 'dispatchq_status' || type === 'claudeq_status') {
    const status = str(ev.status) || 'error'
    b.push({
      kind: 'status',
      label: status.replace(/_/g, ' '),
      text: str(ev.message),
      tone: status === 'note' ? 'note' : 'error',
    })
    return
  }
  if (type === 'system' && ev.subtype === 'init') {
    b.push({kind: 'meta', text: 'Session started' + (ev.model ? ` · ${str(ev.model)}` : '')})
    return
  }

  // opencode's own JSONL shape.
  const part = isObj(ev.part) ? ev.part : undefined
  if (type === 'text' && part) {
    const t = str(part.text).trim()
    if (t) b.push({kind: 'message', role: 'assistant', who: 'opencode', text: t})
    return
  }
  if (type === 'tool_use' && part) {
    const st = isObj(part.state) ? part.state : {}
    b.step({
      kind: 'tool',
      name: str(part.tool) || 'tool',
      summary: toolSummary(st.input),
      input: pretty(st.input),
      output: st.output != null ? String(st.output) : undefined,
    })
    return
  }
  if (type === 'error' && isObj(ev.error)) {
    const data = isObj(ev.error.data) ? ev.error.data : {}
    b.push({kind: 'status', label: 'error', text: str(data.message) || str(ev.error.name), tone: 'error'})
    return
  }

  // Codex's own JSONL shape.
  if (type === 'thread.started') {
    b.push({kind: 'meta', text: 'Session started'})
    return
  }
  const item = isObj(ev.item) ? ev.item : undefined
  if (type === 'item.completed' && item) {
    if (item.type === 'agent_message') {
      const t = str(item.text).trim()
      if (t) b.push({kind: 'message', role: 'assistant', who: 'Codex', text: t})
    } else if (item.type === 'error') {
      b.push({kind: 'status', label: 'error', text: str(item.message), tone: 'error'})
    } else if (item.type === 'command_execution') {
      const cmd = str(item.command)
      b.step({
        kind: 'tool',
        name: 'bash',
        summary: firstLine(cmd),
        input: cmd,
        output: str(item.aggregated_output) || undefined,
      })
    } else {
      b.step({kind: 'tool', name: str(item.type) || 'item', summary: '', input: pretty(item)})
    }
    return
  }
  if (type === 'turn.failed') {
    const err = isObj(ev.error) ? str(ev.error.message) : ''
    b.push({kind: 'status', label: 'error', text: err || 'turn failed', tone: 'error'})
  }
  // Everything else (progress, thinking, step and turn boundaries, rate limit
  // pings) is bookkeeping the chat does not show; Raw has it.
}

// The text a "Copy" of the whole chat gives: the conversation without the
// tool noise.
export function chatTranscript(prompt: string, entries: LogEntry[]): string {
  const parts: string[] = []
  if (prompt.trim()) parts.push(`You:\n${prompt.trim()}`)
  for (const e of entries) {
    if (e.kind === 'message') parts.push(`${e.who}:\n${e.text.trim()}`)
    else if (e.kind === 'result') parts.push(`Result${e.error ? ' (error)' : ''}:\n${e.text.trim()}`)
    else if (e.kind === 'status') parts.push(`${e.label}: ${e.text}`)
  }
  return parts.join('\n\n')
}
