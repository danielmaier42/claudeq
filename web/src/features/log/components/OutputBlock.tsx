import {useState} from 'react'
import {cn} from '@/lib/utils'
import {CopyButton} from './CopyButton'

const CLIP = 6_000

// OutputBlock is a tool's input or output: monospace, scrolling inside a
// bounded box, and clipped at a few thousand characters until asked for all
// of it (a file dump can be megabytes).
export function OutputBlock({label, text, tone}: {label: string; text: string; tone?: 'error'}) {
  const [all, setAll] = useState(false)
  const clipped = !all && text.length > CLIP
  return (
    <div className="group/out relative">
      <div className="mb-1 text-[11px] font-semibold tracking-wide text-subtle-foreground uppercase">
        {label}
      </div>
      <pre
        className={cn(
          'max-h-80 overflow-auto rounded-md border border-border bg-surface px-2.5 py-2 font-mono text-[11.5px] leading-normal break-words whitespace-pre-wrap text-muted-foreground',
          tone === 'error' && 'border-danger/30 text-danger',
        )}
      >
        {clipped ? text.slice(0, CLIP) : text}
        {clipped && (
          <button
            type="button"
            onClick={() => setAll(true)}
            className="mt-2 block font-sans text-[12px] font-medium text-accent-text hover:underline"
          >
            Show all ({text.length.toLocaleString()} characters)
          </button>
        )}
      </pre>
      <CopyButton
        text={text}
        label={`Copy ${label.toLowerCase()}`}
        className="absolute top-6 right-1.5 opacity-0 group-hover/out:opacity-100 focus-visible:opacity-100"
      />
    </div>
  )
}
