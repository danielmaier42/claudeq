import {AlertTriangle, CircleCheck, CircleX, Info, Sparkles, User} from 'lucide-react'
import type {ReactNode} from 'react'
import {duration, usd} from '@/lib/format'
import {cn} from '@/lib/utils'
import type {LogEntry} from '../parse-log'
import {CopyButton} from './CopyButton'
import {Expandable, isLong} from './Expandable'
import {RichText} from './RichText'
import {ToolSteps} from './ToolSteps'

// ChatLog shows a run as the conversation it was: the prompt, the agent's
// messages, folded tool activity, dispatchq's own remarks and the final result.
export function ChatLog({
  prompt,
  entries,
  live,
  promptTitle = 'Prompt',
}: {
  prompt: string
  entries: LogEntry[]
  live: boolean
  // A script run's "prompt" is the script it ran.
  promptTitle?: string
}) {
  const last = entries.length - 1
  return (
    <div className="mx-auto flex max-w-[820px] flex-col gap-3">
      {prompt.trim() && <UserMessage who="You" text={prompt} title={promptTitle} />}
      {entries.map((e, i) => {
        switch (e.kind) {
          case 'message':
            return e.role === 'user' ? (
              <UserMessage key={i} who={e.who} text={e.text} />
            ) : (
              <AssistantMessage key={i} who={e.who} text={e.text} />
            )
          case 'steps':
            return <ToolSteps key={i} steps={e.steps} active={live && i === last} />
          case 'result':
            return <ResultCard key={i} entry={e} />
          case 'status':
            return <StatusNote key={i} label={e.label} text={e.text} tone={e.tone} />
          case 'meta':
            return (
              <div key={i} className="flex items-center gap-3 py-0.5 text-[11.5px] text-subtle-foreground">
                <span className="h-px flex-1 bg-border" />
                {e.text}
                <span className="h-px flex-1 bg-border" />
              </div>
            )
          default:
            return (
              <pre
                key={i}
                className="font-mono text-[11.5px] break-words whitespace-pre-wrap text-subtle-foreground"
              >
                {e.text}
              </pre>
            )
        }
      })}
    </div>
  )
}

function Who({icon, children, className}: {icon: ReactNode; children: ReactNode; className?: string}) {
  return (
    <div
      className={cn(
        'mb-1 flex items-center gap-1.5 text-[11px] font-semibold tracking-wide text-subtle-foreground uppercase [&_svg]:size-3.5',
        className,
      )}
    >
      {icon}
      {children}
    </div>
  )
}

function UserMessage({who, text, title}: {who: string; text: string; title?: string}) {
  return (
    <div className="group/msg relative ml-auto max-w-[88%] rounded-xl rounded-tr-sm border border-accent/15 bg-accent-soft px-3.5 py-2.5">
      <Who icon={<User />}>{title ?? who}</Who>
      <Expandable long={isLong(text)}>
        <RichText text={text} />
      </Expandable>
      <CopyButton
        text={text}
        label="Copy"
        className="absolute top-1.5 right-1.5 opacity-0 group-hover/msg:opacity-100 focus-visible:opacity-100"
      />
    </div>
  )
}

function AssistantMessage({who, text}: {who: string; text: string}) {
  return (
    <div className="group/msg relative max-w-full pr-8">
      <Who icon={<Sparkles className="text-ai" />}>{who}</Who>
      <RichText text={text} />
      <CopyButton
        text={text}
        label="Copy"
        className="absolute top-0 right-0 opacity-0 group-hover/msg:opacity-100 focus-visible:opacity-100"
      />
    </div>
  )
}

function ResultCard({entry}: {entry: Extract<LogEntry, {kind: 'result'}>}) {
  const facts = [
    entry.durationMs ? duration(entry.durationMs) : '',
    entry.turns ? `${entry.turns} turn${entry.turns === 1 ? '' : 's'}` : '',
    entry.costUsd ? usd(entry.costUsd) : '',
  ].filter(Boolean)
  return (
    <div
      className={cn(
        'group/msg relative rounded-xl border px-3.5 py-3',
        entry.error ? 'border-danger/30 bg-danger-soft' : 'border-success/25 bg-success-soft',
      )}
    >
      <Who
        icon={entry.error ? <CircleX className="text-danger" /> : <CircleCheck className="text-success" />}
        className="text-foreground/70"
      >
        {entry.error ? 'Result · error' : 'Result'}
        {facts.length > 0 && (
          <span className="font-normal tracking-normal normal-case">· {facts.join(' · ')}</span>
        )}
      </Who>
      <Expandable long={isLong(entry.text)}>
        <RichText text={entry.text} />
      </Expandable>
      <CopyButton
        text={entry.text}
        label="Copy result"
        className="absolute top-2 right-2 opacity-0 group-hover/msg:opacity-100 focus-visible:opacity-100"
      />
    </div>
  )
}

// A dispatchq remark about the run itself (a dropped session, a pool choice) is
// not a failure, so it does not borrow the failure colour.
function StatusNote({label, text, tone}: {label: string; text: string; tone: 'note' | 'error'}) {
  const error = tone === 'error'
  return (
    <div
      className={cn(
        'flex gap-2.5 rounded-lg border px-3 py-2 text-[12.5px]',
        error ? 'border-danger/30 bg-danger-soft' : 'border-border bg-surface-2/60 text-muted-foreground',
      )}
    >
      {error ? (
        <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-danger" />
      ) : (
        <Info className="mt-0.5 size-3.5 shrink-0" />
      )}
      <div className="min-w-0">
        <span className={cn('mr-1.5 font-semibold capitalize', error ? 'text-danger' : 'text-foreground/80')}>
          {label}
        </span>
        <span className="break-words whitespace-pre-wrap">{text}</span>
      </div>
    </div>
  )
}
