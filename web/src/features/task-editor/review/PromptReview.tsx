import {Sparkles} from 'lucide-react'
import {useEffect, useState, useSyncExternalStore, type ReactNode} from 'react'
import {Button} from '@/components/ui/button'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {cn} from '@/lib/utils'
import {ReviewController} from './controller'

// PromptReview is the purple "DispatchQ suggests:" banner under a prompt box. It
// renders nothing until there is something to say. Mounting it shows only what
// is already known about the text (no Claude usage); every later change of the
// prompt, kind or folder is reviewed after a pause in typing. `fresh` asks for
// a real review straight away — for text new to this Mac, like an import.
// Unmounting retires the review in flight.
export function PromptReview({
  kind,
  prompt,
  workingDir,
  onApply,
  className,
  fresh = false,
}: {
  kind: 'task' | 'system' | 'script'
  prompt: string
  workingDir: string
  onApply: (revised: string) => void
  className?: string
  fresh?: boolean
}) {
  const [ctl] = useState(() => new ReviewController())
  const view = useSyncExternalStore(ctl.subscribe, ctl.getSnapshot)

  useEffect(() => {
    ctl.update({kind, prompt, workingDir}, fresh)
  }, [ctl, kind, prompt, workingDir, fresh])
  useEffect(() => () => ctl.dispose(), [ctl])

  if (view.state === 'hidden') return null

  if (view.state === 'busy')
    return (
      <Banner className={cn('text-muted-foreground', className)} busy>
        <span className="font-semibold text-ai">DispatchQ is checking this {view.noun}…</span>
      </Banner>
    )

  const {res, remembered, noun} = view
  const apply = (revised: string) => {
    ctl.markApplied(revised)
    onApply(revised)
    toast.success(noun === 'script' ? 'Script rewritten' : 'Prompt rewritten')
  }
  return (
    <Banner className={className}>
      <div className="font-semibold text-ai">
        {remembered ? 'DispatchQ suggested earlier:' : 'DispatchQ suggests:'}
      </div>
      <div className="mt-0.5 break-words whitespace-pre-wrap text-foreground/90">{res.message}</div>
      <div className="mt-2.5 flex flex-wrap gap-2">
        {res.revised_prompt && (
          <Tooltip
            content={`Rewrites the ${noun} as suggested. The result lands in the box above, where you can still change it.`}
          >
            <Button size="sm" variant="ai" onClick={() => apply(res.revised_prompt ?? '')}>
              Apply
            </Button>
          </Tooltip>
        )}
        <Button size="sm" variant="ghost" onClick={() => ctl.dismiss()}>
          Dismiss
        </Button>
        {remembered && (
          // The prompt may be fine by now without the text having changed: the
          // missing file was created, the folder exists.
          <Tooltip content={`Asks Claude about this ${noun} again. Costs a little usage.`}>
            <Button size="sm" variant="ghost" onClick={() => ctl.checkAgain()}>
              Check again
            </Button>
          </Tooltip>
        )}
      </div>
    </Banner>
  )
}

function Banner({busy, className, children}: {busy?: boolean; className?: string; children: ReactNode}) {
  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        'flex animate-in gap-2.5 rounded-lg border border-ai/30 bg-ai-soft px-3 py-2.5 text-[12.5px] leading-relaxed',
        className,
      )}
    >
      <Sparkles className={cn('mt-0.5 size-4 shrink-0 text-ai', busy && 'animate-pulse-dot')} aria-hidden />
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}
