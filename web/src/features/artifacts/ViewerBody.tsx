import {useQuery} from '@tanstack/react-query'
import {Download, ExternalLink, FileQuestion, FileWarning} from 'lucide-react'
import {type ReactNode, useEffect, useRef} from 'react'
import {Button} from '@/components/ui/button'
import {EmptyState} from '@/components/ui/empty-state'
import {Spinner} from '@/components/ui/spinner'
import {errorMessage, get} from '@/lib/api'
import {bytes} from '@/lib/format'
import {openExternal} from '@/lib/native'
import type {Artifact} from '@/lib/types'
import {downloadArtifact, openArtifactExternal} from './actions'
import {artifactKind, contentURL, frameLink} from './logic'

// ViewerBody shows one artifact the way the window can: web pages and PDFs in
// a frame, images inline, text as text, and for anything else the way out.
export function ViewerBody({a}: {a: Artifact}) {
  const src = contentURL(a.id)
  switch (artifactKind(a.content_type)) {
    case 'html':
      return <HtmlBody a={a} />
    case 'pdf':
      // PDFs render in the engine's built-in viewer. Chromium refuses to show a
      // PDF inside a sandboxed iframe, so this one is not sandboxed — safe
      // because the file is served as application/pdf with
      // X-Content-Type-Options: nosniff, so it can never be interpreted as
      // HTML, and the native PDF viewer does not expose the app's origin.
      return <iframe title={a.title} src={src} className="size-full border-0 bg-white" />
    case 'image':
      return (
        <div className="flex size-full items-center justify-center overflow-auto bg-surface-2 p-6">
          <img
            src={src}
            alt={a.title}
            className="max-h-full max-w-full rounded-md object-contain shadow-sm"
          />
        </div>
      )
    case 'text':
      return <TextBody a={a} />
    default:
      // Archives, binaries and anything else the engine cannot render inline:
      // the viewer still opens, so the file can be handed on from here and the
      // publishing session continued from the footer.
      return (
        <Fallback
          icon={<FileQuestion />}
          title="This file type has no preview"
          description={`${a.file_name} · ${bytes(a.size)} · open it in its app or save a copy.`}
          a={a}
        />
      )
  }
}

// HtmlBody runs the page in an opaque origin (sandbox without
// allow-same-origin) so it cannot reach the loopback API or the parent DOM; the
// content CSP additionally blocks any network access, so it cannot phone home.
// Links cannot open from inside the frame, so the page posts each clicked link
// here and it goes to the browser.
function HtmlBody({a}: {a: Artifact}) {
  const frame = useRef<HTMLIFrameElement>(null)
  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      if (e.source !== frame.current?.contentWindow) return
      const url = frameLink(e.data)
      if (url) openExternal(url)
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [])
  return (
    <iframe
      ref={frame}
      title={a.title}
      src={contentURL(a.id, 'frame')}
      sandbox="allow-scripts"
      className="size-full border-0 bg-white"
    />
  )
}

function TextBody({a}: {a: Artifact}) {
  // The stored file never changes, so it is fetched once per artifact.
  const {data, isPending, error} = useQuery({
    queryKey: ['artifact-content', a.id],
    queryFn: ({signal}) => get<unknown>(contentURL(a.id), signal),
    staleTime: Infinity,
    gcTime: 60_000,
    retry: false,
  })
  if (isPending) {
    return (
      <div className="flex size-full items-center justify-center text-muted-foreground">
        <Spinner />
      </div>
    )
  }
  if (error) {
    return (
      <Fallback
        icon={<FileWarning />}
        title="Could not load the file"
        description={errorMessage(error)}
        a={a}
      />
    )
  }
  return (
    <pre className="size-full overflow-auto bg-surface-2 px-5 py-4 font-mono text-[12.5px] leading-[1.6] break-words whitespace-pre-wrap text-foreground select-text">
      {typeof data === 'string' ? data : JSON.stringify(data, null, 2)}
    </pre>
  )
}

function Fallback({
  icon,
  title,
  description,
  a,
}: {
  icon: ReactNode
  title: string
  description: string
  a: Artifact
}) {
  return (
    <div className="flex size-full items-center justify-center">
      <EmptyState
        icon={icon}
        title={title}
        description={description}
        action={
          <>
            <Button variant="primary" onClick={() => openArtifactExternal(a)}>
              <ExternalLink />
              Open externally
            </Button>
            <Button onClick={() => downloadArtifact(a)}>
              <Download />
              Download
            </Button>
          </>
        }
      />
    </div>
  )
}
