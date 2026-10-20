import type {ComponentProps} from 'react'
import Markdown, {type Components} from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {openExternal} from '@/lib/native'

// MarkdownBody renders a Markdown artifact as a readable page. It builds React
// elements and ignores raw HTML in the file, so nothing in it can inject markup
// into the window; web links go to the browser, images show their alt text
// because the window loads nothing from outside.
export function MarkdownBody({text}: {text: string}) {
  return (
    <div className="size-full overflow-auto bg-surface select-text">
      <article className="mx-auto max-w-[860px] space-y-3 px-8 py-6 text-[14px] leading-relaxed break-words text-foreground">
        <Markdown remarkPlugins={[remarkGfm]} components={components}>
          {text}
        </Markdown>
      </article>
    </div>
  )
}

function Link({href, children}: ComponentProps<'a'>) {
  const web = !!href && /^https?:\/\//i.test(href)
  if (!web) return <span title={href}>{children}</span>
  return (
    <a
      href={href}
      onClick={(e) => {
        e.preventDefault()
        openExternal(href)
      }}
      className="text-accent-text underline-offset-2 hover:underline"
    >
      {children}
    </a>
  )
}

const components: Components = {
  h1: ({children}) => <h1 className="pt-1 text-[22px] leading-tight font-semibold">{children}</h1>,
  h2: ({children}) => (
    <h2 className="border-b border-border pt-3 pb-1 text-[18px] leading-tight font-semibold">{children}</h2>
  ),
  h3: ({children}) => <h3 className="pt-2 text-[15.5px] font-semibold">{children}</h3>,
  h4: ({children}) => <h4 className="pt-1 font-semibold">{children}</h4>,
  h5: ({children}) => <h5 className="pt-1 font-semibold">{children}</h5>,
  h6: ({children}) => <h6 className="pt-1 font-semibold text-muted-foreground">{children}</h6>,
  p: ({children}) => <p>{children}</p>,
  a: Link,
  img: ({alt}) => (alt ? <span className="text-muted-foreground">[{alt}]</span> : null),
  ul: ({children, className}) => (
    <ul className={className?.includes('contains-task-list') ? 'space-y-1 pl-1' : 'list-disc space-y-1 pl-5'}>
      {children}
    </ul>
  ),
  ol: ({children, start}) => (
    <ol start={start} className="list-decimal space-y-1 pl-5">
      {children}
    </ol>
  ),
  li: ({children}) => <li className="marker:text-subtle-foreground [&>ol]:mt-1 [&>ul]:mt-1">{children}</li>,
  input: ({checked}) => (
    <input type="checkbox" checked={!!checked} readOnly disabled className="mr-1.5 align-middle" />
  ),
  blockquote: ({children}) => (
    <blockquote className="space-y-2 border-l-2 border-border-strong pl-3 text-muted-foreground">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="border-border" />,
  pre: ({children}) => (
    <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2 px-3 py-2.5 font-mono text-[12.5px] leading-normal [&_code]:bg-transparent [&_code]:p-0">
      {children}
    </pre>
  ),
  code: ({children}) => (
    <code className="rounded bg-surface-3 px-1 py-px font-mono text-[0.88em]">{children}</code>
  ),
  table: ({children}) => (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13px]">{children}</table>
    </div>
  ),
  th: ({children, style}) => (
    <th
      style={style}
      className="border border-border bg-surface-2 px-2.5 py-1.5 text-left align-top font-semibold"
    >
      {children}
    </th>
  ),
  td: ({children, style}) => (
    <td style={style} className="border border-border px-2.5 py-1.5 align-top">
      {children}
    </td>
  ),
}
