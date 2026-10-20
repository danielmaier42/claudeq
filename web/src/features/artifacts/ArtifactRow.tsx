import {ChevronRight} from 'lucide-react'
import {bytes} from '@/lib/format'
import type {Artifact} from '@/lib/types'
import {ArtifactActions, ArtifactSource, PublishedAt, TypeTag} from './ArtifactParts'
import {FileIcon} from './FileIcon'

// One artifact as one line of the list layout: the same facts as a card, laid
// out like a notification row so the two lists line up when switching between
// them. The row opens the viewer the way a card does.
export function ArtifactRow({a, onOpen}: {a: Artifact; onOpen: () => void}) {
  return (
    <div className="group relative flex items-start gap-3 px-3 py-2.5 transition-colors focus-within:bg-surface-2 hover:bg-surface-2">
      <FileIcon fileName={a.file_name} contentType={a.content_type} size="sm" className="mt-px" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <button
            type="button"
            onClick={onOpen}
            className="min-w-0 truncate text-left text-[13px] leading-5 font-medium outline-none after:absolute after:inset-0 after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-accent/40 focus-visible:after:ring-inset"
          >
            {a.title}
          </button>
          <ArtifactSource a={a} className="shrink text-[12px] text-subtle-foreground" />
        </div>
        <p className="truncate text-[12.5px] leading-[18px] text-muted-foreground">
          {a.description || (
            <span className="font-mono text-[11.5px]">
              {a.file_name} · {bytes(a.size)}
            </span>
          )}
        </p>
      </div>
      <div className="flex h-7 shrink-0 items-center gap-1">
        <ArtifactActions a={a} />
        <TypeTag a={a} />
        <span className="w-16 text-right text-[12px] text-subtle-foreground">
          <PublishedAt a={a} />
        </span>
        <span
          aria-hidden
          className="flex size-5 items-center justify-center text-subtle-foreground opacity-50 transition-opacity group-hover:opacity-100"
        >
          <ChevronRight className="size-4" />
        </span>
      </div>
    </div>
  )
}
