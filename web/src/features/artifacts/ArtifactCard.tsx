import {bytes} from '@/lib/format'
import type {Artifact} from '@/lib/types'
import {ArtifactActions, ArtifactSource, PublishedAt, TypeTag} from './ArtifactParts'
import {FileIcon} from './FileIcon'

// One artifact as one card: the type tile, the title with its summary, who
// made it and when, the file. The whole card opens the viewer (a stretched
// button under the title, so the source link and the actions stay their own
// targets); the actions show while the pointer or focus is on the card.
export function ArtifactCard({a, onOpen}: {a: Artifact; onOpen: () => void}) {
  return (
    <div className="group relative flex min-h-[148px] flex-col rounded-xl border border-border bg-surface p-3.5 shadow-xs transition-[border-color,box-shadow] focus-within:border-accent/60 hover:border-border-strong hover:shadow-sm">
      <div className="flex items-start gap-3">
        <FileIcon fileName={a.file_name} contentType={a.content_type} />
        <div className="min-w-0 flex-1 pt-0.5">
          <button
            type="button"
            onClick={onOpen}
            className="block w-full text-left outline-none after:absolute after:inset-0 after:rounded-xl after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-accent/40"
          >
            <span className="line-clamp-2 text-[13.5px] leading-[19px] font-medium">{a.title}</span>
          </button>
        </div>
        <ArtifactActions a={a} className="-mt-1 -mr-1" />
      </div>

      {a.description && (
        <p className="mt-2 line-clamp-2 text-[12.5px] leading-[18px] text-muted-foreground">
          {a.description}
        </p>
      )}
      <p className="mt-1.5 truncate font-mono text-[11.5px] text-subtle-foreground">
        {a.file_name} · {bytes(a.size)}
      </p>

      <div className="mt-auto flex items-center gap-2 pt-3 text-[12px] text-subtle-foreground">
        <TypeTag a={a} />
        <ArtifactSource a={a} />
        <span className="flex-1" />
        <PublishedAt a={a} />
      </div>
    </div>
  )
}
