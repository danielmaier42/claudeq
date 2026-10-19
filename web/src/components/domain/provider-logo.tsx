import {Server} from 'lucide-react'
import {cn} from '@/lib/utils'

// ProviderLogo is the mark of the harness a provider runs: Claude's burst in its
// orange, OpenAI's knot for Codex in the text colour, a prompt for opencode.
// Simplified marks, drawn here so the app needs no image assets. An unknown
// kind gets a neutral server icon.
export function ProviderLogo({kind, className}: {kind: string; className?: string}) {
  const cls = cn('size-5 shrink-0', className)
  switch (kind) {
    case 'claude-code':
      return (
        <svg viewBox="0 0 24 24" className={cls} aria-hidden>
          <g stroke="#d97757" strokeWidth="2.6" strokeLinecap="round">
            {Array.from({length: 12}, (_, i) => {
              const a = (i * Math.PI) / 6
              const inner = 2.2
              const outer = i % 2 ? 8.6 : 10.4
              return (
                <line
                  key={i}
                  x1={12 + inner * Math.cos(a)}
                  y1={12 + inner * Math.sin(a)}
                  x2={12 + outer * Math.cos(a)}
                  y2={12 + outer * Math.sin(a)}
                />
              )
            })}
          </g>
        </svg>
      )
    case 'codex':
      return (
        <svg viewBox="0 0 24 24" className={cn(cls, 'text-foreground')} aria-hidden>
          <g fill="none" stroke="currentColor" strokeWidth="1.7">
            {Array.from({length: 6}, (_, i) => (
              <rect
                key={i}
                x="9.2"
                y="2.6"
                width="5.6"
                height="11"
                rx="2.8"
                transform={`rotate(${i * 60} 12 12)`}
              />
            ))}
          </g>
        </svg>
      )
    case 'opencode':
      return (
        <svg viewBox="0 0 24 24" className={cn(cls, 'text-foreground')} aria-hidden>
          <rect
            x="2.5"
            y="3.5"
            width="19"
            height="17"
            rx="3.5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
          />
          <path
            d="M7 9.5l3 2.5-3 2.5M12.5 15h4.5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )
    default:
      return <Server className={cn(cls, 'text-muted-foreground')} aria-hidden />
  }
}

// ProviderLogoTile puts the mark on a soft tile, for lists and headers.
export function ProviderLogoTile({kind, className}: {kind: string; className?: string}) {
  return (
    <span
      className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-2',
        className,
      )}
    >
      <ProviderLogo kind={kind} className="size-[18px]" />
    </span>
  )
}
