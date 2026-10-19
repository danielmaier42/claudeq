import {ArrowUpRight, CircleAlert, Search, Sparkles} from 'lucide-react'
import {RichText} from '@/features/log/components/RichText'
import {cn} from '@/lib/utils'
import type {ChatEntry} from '../store'
import {ActionCard} from './ActionCard'

// Entry is one message: the operator's on the right, the assistant's on the
// left with what it looked up, where it took the app, and its proposals.
export function Entry({entry}: {entry: ChatEntry}) {
  if (entry.role === 'user')
    return (
      <div className="flex animate-in justify-end">
        <div className="max-w-[85%] rounded-2xl rounded-br-md bg-accent px-3 py-1.5 text-[13px] whitespace-pre-wrap text-accent-foreground shadow-xs">
          {entry.text}
        </div>
      </div>
    )

  const error = entry.role === 'error'
  return (
    <div className="flex animate-in items-start gap-2">
      <Avatar error={error} />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        {!!entry.steps?.length && (
          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-[11.5px] text-subtle-foreground">
            {entry.steps.map((s, i) => (
              <span key={i} className="inline-flex items-center gap-1">
                <Search className="size-3" />
                {s}
              </span>
            ))}
          </div>
        )}
        {entry.text && (
          <div
            className={cn(
              'w-fit max-w-full rounded-2xl rounded-tl-md border px-3 py-1.5 shadow-xs',
              error ? 'border-danger/25 bg-danger-soft' : 'border-border bg-surface-2/60',
            )}
          >
            {error ? (
              <p className="text-[13px] whitespace-pre-wrap">{entry.text}</p>
            ) : (
              <RichText text={entry.text} className="space-y-1.5 text-[13px]" />
            )}
          </div>
        )}
        {entry.moved?.filter(Boolean).map((m, i) => (
          <div key={i} className="inline-flex items-center gap-1 text-[11.5px] text-accent-text">
            <ArrowUpRight className="size-3" />
            {m}
          </div>
        ))}
        {entry.cards?.map((c, i) => (
          <ActionCard key={i} entryId={entry.id} index={i} card={c} />
        ))}
      </div>
    </div>
  )
}

function Avatar({error}: {error?: boolean}) {
  return (
    <div
      className={cn(
        'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full [&_svg]:size-3',
        error ? 'bg-danger-soft text-danger' : 'bg-ai-soft text-ai',
      )}
    >
      {error ? <CircleAlert /> : <Sparkles />}
    </div>
  )
}

export function Typing() {
  return (
    <div className="flex animate-in items-center gap-2" role="status" aria-label="The assistant is working">
      <Avatar />
      <div className="flex h-8 items-center gap-1 rounded-2xl rounded-tl-md border border-border bg-surface-2/60 px-3 shadow-xs">
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
