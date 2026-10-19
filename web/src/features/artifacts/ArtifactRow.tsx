import {bytes} from '@/lib/format'
import type {Artifact} from '@/lib/types'
import {ArtifactActions, ArtifactSource, PublishedAt, TypeTag} from './ArtifactParts'
import {FileIcon} from './FileIcon'

// One artifact as one line of the list layout: the same facts as a card,
// side by side, so many fit on screen. The row opens the viewer the way a card
// does.
export function ArtifactRow({a, onOpen}: {a: Artifact; onOpen: () => void}) {
  return (
    <div className="group relative flex min-h-12 items-center gap-3 px-3 py-2 transition-colors focus-within:bg-surface-2/70 hover:bg-surface-2/70">
      <FileIcon fileName={a.file_name} contentType={a.content_type} size="sm" />
      <div className="min-w-0 flex-1">
        <button
          type="button"
          onClick={onOpen}
          className="block max-w-full truncate text-left text-[13px] font-medium outline-none after:absolute after:inset-0 after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-accent/40 focus-visible:after:ring-inset"
        >
          {a.title}
        </button>
        <p className="truncate text-[12px] text-muted-foreground">
          {a.description || (
            <span className="font-mono text-[11.5px]">
              {a.file_name} · {bytes(a.size)}
            </span>
          )}
        </p>
      </div>
      <div className="hidden w-48 min-w-0 shrink-0 text-[12px] text-subtle-foreground md:flex">
        <ArtifactSource a={a} />
      </div>
      <div className="flex w-14 shrink-0 justify-end">
        <TypeTag a={a} />
      </div>
      <div className="w-20 shrink-0 text-right text-[12px] text-subtle-foreground">
        <PublishedAt a={a} />
      </div>
      <ArtifactActions a={a} />
    </div>
  )
}
