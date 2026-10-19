import {ExternalLink, Trash2} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Tooltip} from '@/components/ui/tooltip'
import {openRunLog} from '@/features/log/store'
import {fullTime, relTime} from '@/lib/format'
import type {Artifact} from '@/lib/types'
import {cn} from '@/lib/utils'
import {deleteArtifact, openArtifactExternal} from './actions'
import {extLabel, sourceName} from './logic'

// The pieces a card and a list row share. Each sits above the stretched open
// button (relative z-[1]), so it stays its own click target.

export function ArtifactActions({a, className}: {a: Artifact; className?: string}) {
  return (
    <div
      className={cn(
        'relative z-[1] flex shrink-0 gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100',
        className,
      )}
    >
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
  )
}

export function TypeTag({a}: {a: Artifact}) {
  return (
    <span className="shrink-0 rounded bg-surface-3 px-1.5 py-px text-[10.5px] font-semibold tracking-wide text-muted-foreground">
      {extLabel(a.file_name, a.content_type)}
    </span>
  )
}

// ArtifactSource names who made the artifact; with a run on record it opens
// that run's log.
export function ArtifactSource({a, className}: {a: Artifact; className?: string}) {
  const src = sourceName(a)
  if (!src) return null
  return (
    <Tooltip content={src.chain + (a.run_id ? '\nClick to open the run’s log' : '')}>
      {a.run_id ? (
        <button
          type="button"
          onClick={() => openRunLog(a.run_id!)}
          className={cn(
            'relative z-[1] min-w-0 truncate rounded text-left outline-none hover:text-accent-text hover:underline focus-visible:ring-2 focus-visible:ring-accent/40',
            className,
          )}
        >
          {src.name}
        </button>
      ) : (
        <span className={cn('relative z-[1] min-w-0 truncate', className)}>{src.name}</span>
      )}
    </Tooltip>
  )
}

export function PublishedAt({a}: {a: Artifact}) {
  return (
    <Tooltip content={fullTime(a.published_at)}>
      <span className="relative z-[1] shrink-0 tabular-nums">{relTime(a.published_at)}</span>
    </Tooltip>
  )
}
