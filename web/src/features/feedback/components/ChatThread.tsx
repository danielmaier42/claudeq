import {CircleAlert, FileCheck2, Sparkles} from 'lucide-react'
import {useEffect, useRef} from 'react'
import {cn} from '@/lib/utils'
import type {ChatMessage} from '../store'

// ChatThread is the conversation: the user's messages on the right, Claude's
// on the left, and three bouncing dots while a turn runs.
export function ChatThread({messages, typing}: {messages: ChatMessage[]; typing: boolean}) {
  const end = useRef<HTMLDivElement>(null)
  useEffect(() => {
    end.current?.scrollIntoView({behavior: 'smooth', block: 'end'})
  }, [messages.length, typing])

  return (
    <div className="flex flex-col gap-3" aria-live="polite">
      {messages.map((m) =>
        m.role === 'user' ? <UserBubble key={m.id} text={m.text} /> : <ClaudeBubble key={m.id} message={m} />,
      )}
      {typing && <TypingIndicator />}
      <div ref={end} />
    </div>
  )
}

function UserBubble({text}: {text: string}) {
  return (
    <div className="flex animate-in justify-end">
      <div className="max-w-[80%] rounded-2xl rounded-br-md bg-accent px-3.5 py-2 whitespace-pre-wrap text-accent-foreground shadow-xs">
        {text}
      </div>
    </div>
  )
}

function Avatar({error}: {error?: boolean}) {
  return (
    <div
      className={cn(
        'flex size-7 shrink-0 items-center justify-center rounded-full [&_svg]:size-3.5',
        error ? 'bg-danger-soft text-danger' : 'bg-ai-soft text-ai',
      )}
    >
      {error ? <CircleAlert /> : <Sparkles />}
    </div>
  )
}

function ClaudeBubble({message}: {message: ChatMessage}) {
  const error = message.role === 'error'
  return (
    <div className="flex animate-in items-end gap-2">
      <Avatar error={error} />
      <div
        className={cn(
          'max-w-[80%] rounded-2xl rounded-bl-md border px-3.5 py-2 whitespace-pre-wrap shadow-xs',
          error ? 'border-danger/25 bg-danger-soft' : 'border-border bg-surface',
        )}
      >
        {error && (
          <div className="mb-0.5 text-[12px] font-semibold text-danger">
            The assistant could not draft the issue
          </div>
        )}
        {message.role === 'result' && (
          <FileCheck2 className="mr-1.5 inline size-3.5 align-[-2px] text-success" />
        )}
        {message.text}
      </div>
    </div>
  )
}

function TypingIndicator() {
  return (
    <div className="flex animate-in items-end gap-2" role="status" aria-label="Claude is writing">
      <Avatar />
      <div className="flex h-9 items-center gap-1 rounded-2xl rounded-bl-md border border-border bg-surface px-3.5 shadow-xs">
        {[0, 150, 300].map((d) => (
          <span
            key={d}
            className="size-1.5 animate-bounce rounded-full bg-muted-foreground/70"
            style={{animationDelay: `${d}ms`}}
          />
        ))}
      </div>
    </div>
  )
}
