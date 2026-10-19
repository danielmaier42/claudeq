import {ExternalLink, Trash2} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Tooltip} from '@/components/ui/tooltip'
import {openRunLog} from '@/features/log/store'
import {bytes, fullTime, relTime} from '@/lib/format'
import type {Artifact} from '@/lib/types'
import {cn} from '@/lib/utils'
import {deleteArtifact, openArtifactExternal} from './actions'
import {FileIcon} from './FileIcon'
import {extLabel, sourceName} from './logic'

// One artifact as one card: the type tile, the title with its summary, who
// made it and when, the file. The whole card opens the viewer (a stretched
// button under the title, so the source link and the actions stay their own
// targets); the actions show while the pointer or focus is on the card.
export function ArtifactCard({a, onOpen}: {a: Artifact; onOpen: () => void}) {
  const src = sourceName(a)
  return (
    <div
      className={cn(
        'group relative flex min-h-[148px] flex-col rounded-xl border border-border bg-surface p-3.5 shadow-xs transition-[border-color,box-shadow] focus-within:border-accent/60 hover:border-border-strong hover:shadow-sm',
      )}
    >
      <div className="flex items-start gap-3">
        <div className="relative">
          <FileIcon fileName={a.file_name} contentType={a.content_type} />
        </div>
        <div className="min-w-0 flex-1 pt-0.5">
          <button
            type="button"
            onClick={onOpen}
            className="block w-full text-left outline-none after:absolute after:inset-0 after:rounded-xl after:content-[''] focus-visible:after:ring-2 focus-visible:after:ring-accent/40"
          >
            <span className={cn('line-clamp-2 text-[13.5px] leading-[19px]', 'font-medium')}>{a.title}</span>
          </button>
        </div>
        <div className="relative z-[1] -mt-1 -mr-1 flex shrink-0 gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
          <Tooltip content="Open in the default app">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Open externally"
              onClick={() => openArtifactExternal(a)}
            >
              <ExternalLink />
            </Button>
          </Tooltip>
          <Tooltip content="Delete">
            <Button
              variant="danger-ghost"
              size="icon-sm"
              aria-label="Delete"
              onClick={() => void deleteArtifact(a)}
            >
              <Trash2 />
            </Button>
          </Tooltip>
        </div>
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
        <span className="shrink-0 rounded bg-surface-3 px-1.5 py-px text-[10.5px] font-semibold tracking-wide text-muted-foreground">
          {extLabel(a.file_name, a.content_type)}
        </span>
        {src && (
          <Tooltip content={src.chain + (a.run_id ? '\nClick to open the run’s log' : '')}>
            {a.run_id ? (
              <button
                type="button"
                onClick={() => openRunLog(a.run_id!)}
                className="relative z-[1] min-w-0 truncate rounded text-left outline-none hover:text-accent-text hover:underline focus-visible:ring-2 focus-visible:ring-accent/40"
              >
                {src.name}
              </button>
            ) : (
              <span className="relative z-[1] min-w-0 truncate">{src.name}</span>
            )}
          </Tooltip>
        )}
        <span className="flex-1" />
        <Tooltip content={fullTime(a.published_at)}>
          <span className="relative z-[1] shrink-0 tabular-nums">{relTime(a.published_at)}</span>
        </Tooltip>
      </div>
    </div>
  )
}
