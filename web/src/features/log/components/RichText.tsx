import {memo, type ReactNode} from 'react'
import {openExternal} from '@/lib/native'
import {cn} from '@/lib/utils'
import {parseBlocks, parseInline} from '../rich-text'
import {CopyButton} from './CopyButton'

// RichText renders an agent's message readably: paragraphs, headings, lists,
// quotes and code blocks (each with its own copy button). Everything is built
// as React elements from plain text, so nothing in a log can inject markup.
export const RichText = memo(function RichText({text, className}: {text: string; className?: string}) {
  return (
    <div className={cn('space-y-2.5 text-[13.5px] leading-relaxed break-words', className)}>
      {parseBlocks(text).map((b, i) => {
        switch (b.kind) {
          case 'code':
            return (
              <div key={i} className="group/code relative">
                <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2 px-3 py-2.5 font-mono text-[12px] leading-normal">
                  {b.text}
                </pre>
                <CopyButton
                  text={b.text}
                  label="Copy code"
                  className="absolute top-1.5 right-1.5 opacity-0 group-hover/code:opacity-100 focus-visible:opacity-100"
                />
              </div>
            )
          case 'heading':
            return (
              <p key={i} className="pt-1 font-semibold">
                <Inline text={b.text} />
              </p>
            )
          case 'list': {
            const List = b.ordered ? 'ol' : 'ul'
            return (
              <List key={i} className={cn('space-y-1 pl-5', b.ordered ? 'list-decimal' : 'list-disc')}>
                {b.items.map((it, j) => (
                  <li key={j} className="whitespace-pre-wrap marker:text-subtle-foreground">
                    <Inline text={it} />
                  </li>
                ))}
              </List>
            )
          }
          case 'quote':
            return (
              <blockquote
                key={i}
                className="border-l-2 border-border-strong pl-3 whitespace-pre-wrap text-muted-foreground"
              >
                <Inline text={b.text} />
              </blockquote>
            )
          default:
            return (
              <p key={i} className="whitespace-pre-wrap">
                <Inline text={b.text} />
              </p>
            )
        }
      })}
    </div>
  )
})

function Inline({text}: {text: string}) {
  return (
    <>
      {parseInline(text).map((p, i): ReactNode => {
        switch (p.kind) {
          case 'code':
            return (
              <code key={i} className="rounded bg-surface-3 px-1 py-px font-mono text-[0.88em]">
                {p.text}
              </code>
            )
          case 'bold':
            return <strong key={i}>{p.text}</strong>
          case 'link':
            return (
              <a
                key={i}
                href={p.href}
                onClick={(e) => {
                  e.preventDefault()
                  openExternal(p.href)
                }}
                className="text-accent-text underline-offset-2 hover:underline"
              >
                {p.text}
              </a>
            )
          default:
            return p.text
        }
      })}
    </>
  )
}
