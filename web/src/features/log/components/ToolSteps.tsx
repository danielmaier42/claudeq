import {Bot, ChevronRight, Wrench} from 'lucide-react'
import {memo, useState} from 'react'
import {Badge} from '@/components/ui/badge'
import {Spinner} from '@/components/ui/spinner'
import {cn} from '@/lib/utils'
import type {NoteStep, Step, ToolStep} from '../parse-log'
import {Expandable, isLong} from './Expandable'
import {OutputBlock} from './OutputBlock'
import {RichText} from './RichText'

// A few steps are shown as they are; a long stretch of tool work folds into
// one line until it is opened, so the conversation stays readable.
const OPEN_UP_TO = 3

// ToolSteps is one stretch of tool activity between two messages. Its parts
// are rendered only while it is open: a run can make thousands of calls.
export const ToolSteps = memo(function ToolSteps({steps, active}: {steps: Step[]; active: boolean}) {
  const [open, setOpen] = useState(steps.length <= OPEN_UP_TO)
  const tools = steps.filter((s): s is ToolStep => s.kind === 'tool')
  const failed = tools.filter((t) => t.error).length
  const names = [...new Set(tools.map((t) => t.name))]

  return (
    <div className="rounded-lg border border-border bg-surface-2/60">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-[12.5px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        <ChevronRight className={cn('size-3.5 shrink-0 transition-transform', open && 'rotate-90')} />
        {active ? <Spinner className="size-3.5" /> : <Wrench className="size-3.5 shrink-0" />}
        <span className="shrink-0 font-medium text-foreground">{stepsTitle(steps.length, tools.length)}</span>
        <span className="min-w-0 truncate font-mono text-[11.5px]">{names.slice(0, 6).join(', ')}</span>
        {failed > 0 && (
          <Badge tone="danger" className="ml-auto">
            {failed} failed
          </Badge>
        )}
      </button>
      {open && (
        <div className="space-y-1 border-t border-border px-2 py-2">
          {steps.map((s, i) =>
            s.kind === 'tool' ? <ToolRow key={i} step={s} /> : <NoteRow key={i} step={s} />,
          )}
        </div>
      )}
    </div>
  )
})

function stepsTitle(steps: number, tools: number): string {
  if (tools === steps) return tools === 1 ? '1 tool call' : `${tools} tool calls`
  return `${steps} steps`
}

function ToolRow({step}: {step: ToolStep}) {
  const [open, setOpen] = useState(false)
  const showInput = step.input.trim() && step.input.trim() !== step.summary
  return (
    <div className={cn('rounded-md', open && 'bg-surface-3/50')}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-[12px] outline-none hover:bg-surface-3/60 focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        <ChevronRight
          className={cn('size-3 shrink-0 text-subtle-foreground transition-transform', open && 'rotate-90')}
        />
        <span
          className={cn('shrink-0 font-mono font-semibold', step.error ? 'text-danger' : 'text-accent-text')}
        >
          {step.name}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-[11.5px] text-muted-foreground">
          {step.summary}
        </span>
        {step.error && <Badge tone="danger">error</Badge>}
      </button>
      {open && (
        <div className="space-y-2 px-2 pt-1 pb-2 pl-7">
          {showInput && <OutputBlock label="Input" text={step.input} />}
          {step.output !== undefined && (
            <OutputBlock
              label="Output"
              text={step.output || '(no output)'}
              tone={step.error ? 'error' : undefined}
            />
          )}
          {!showInput && step.output === undefined && (
            <p className="text-[12px] text-subtle-foreground">No input or output recorded yet.</p>
          )}
        </div>
      )}
    </div>
  )
}

function NoteRow({step}: {step: NoteStep}) {
  return (
    <div className="flex gap-2 rounded-md px-2 py-1.5 text-[12.5px]">
      <Bot className="mt-0.5 size-3.5 shrink-0 text-ai" />
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-semibold tracking-wide text-subtle-foreground uppercase">
          {step.who}
        </div>
        <Expandable long={isLong(step.text)}>
          <RichText text={step.text} className="text-[12.5px] text-muted-foreground" />
        </Expandable>
      </div>
    </div>
  )
}
