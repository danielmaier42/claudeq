import {AlertDialog as A} from 'radix-ui'
import {useRef, useState, type ReactNode} from 'react'
import {create} from 'zustand'
import {Button} from './button'
import {overlayClass} from './dialog'
import {Input} from './input'

// confirm() and promptText() ask one question and resolve with the answer, so a
// handler reads top to bottom: `if (!(await confirm({...}))) return`. One host
// (<ConfirmHost/>, mounted by the shell) draws whichever question is open.

interface ConfirmOptions {
  title: ReactNode
  description?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}

interface PromptOptions {
  title: ReactNode
  description?: ReactNode
  placeholder?: string
  value?: string
  confirmLabel?: string
  maxLength?: number
}

type Pending =
  | ({kind: 'confirm'; resolve: (v: boolean) => void} & ConfirmOptions)
  | ({kind: 'prompt'; resolve: (v: string | null) => void} & PromptOptions)

const usePending = create<{pending: Pending | null}>(() => ({pending: null}))

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => usePending.setState({pending: {kind: 'confirm', resolve, ...opts}}))
}

export function promptText(opts: PromptOptions): Promise<string | null> {
  return new Promise((resolve) => usePending.setState({pending: {kind: 'prompt', resolve, ...opts}}))
}

export function ConfirmHost() {
  const pending = usePending((s) => s.pending)
  const [text, setText] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  // A new question starts from its own prefilled value.
  const [asked, setAsked] = useState<Pending | null>(null)
  if (asked !== pending) {
    setAsked(pending)
    if (pending?.kind === 'prompt') setText(pending.value ?? '')
  }

  const finish = (answer: boolean) => {
    if (!pending) return
    if (pending.kind === 'confirm') pending.resolve(answer)
    else pending.resolve(answer && text.trim() ? text.trim() : null)
    usePending.setState({pending: null})
  }

  return (
    <A.Root open={!!pending} onOpenChange={(o) => !o && finish(false)}>
      <A.Portal>
        <A.Overlay className={overlayClass} />
        <A.Content
          onOpenAutoFocus={(e) => {
            if (pending?.kind === 'prompt') {
              e.preventDefault()
              inputRef.current?.focus()
            }
          }}
          className="fixed top-1/2 left-1/2 z-50 w-[calc(100vw-48px)] max-w-[400px] -translate-x-1/2 -translate-y-1/2 animate-in rounded-xl border border-border bg-surface p-5 shadow-lg outline-none data-[state=closed]:animate-out"
        >
          {pending && (
            <>
              <A.Title className="text-[15px] font-semibold">{pending.title}</A.Title>
              <A.Description
                className={pending.description ? 'mt-1.5 text-[13px] text-muted-foreground' : 'sr-only'}
              >
                {pending.description ?? pending.title}
              </A.Description>
              {pending.kind === 'prompt' && (
                <Input
                  ref={inputRef}
                  className="mt-4"
                  value={text}
                  placeholder={pending.placeholder}
                  maxLength={pending.maxLength ?? 60}
                  onChange={(e) => setText(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      finish(true)
                    }
                  }}
                />
              )}
              <div className="mt-5 flex justify-end gap-2">
                <A.Cancel asChild>
                  <Button variant="secondary">
                    {(pending.kind === 'confirm' && pending.cancelLabel) || 'Cancel'}
                  </Button>
                </A.Cancel>
                <Button
                  variant={pending.kind === 'confirm' && pending.destructive ? 'danger' : 'primary'}
                  disabled={pending.kind === 'prompt' && !text.trim()}
                  onClick={() => finish(true)}
                >
                  {pending.confirmLabel ?? (pending.kind === 'prompt' ? 'Create' : 'Confirm')}
                </Button>
              </div>
            </>
          )}
        </A.Content>
      </A.Portal>
    </A.Root>
  )
}
