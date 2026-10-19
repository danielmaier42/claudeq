import {FileText} from 'lucide-react'
import {Tooltip} from '@/components/ui/tooltip'
import {openArtifact} from '@/features/artifacts/store'
import {bytes} from '@/lib/format'
import {useArtifacts} from '@/lib/queries'

// The artifacts this run published, as chips that open the viewer; ← → in the
// viewer step through just these.
export function RunArtifacts({runId}: {runId: string}) {
  const arts = useArtifacts().data?.filter((a) => a.run_id === runId) ?? []
  if (!arts.length) return null
  const ids = arts.map((a) => a.id)
  return (
    <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
      <span className="mr-0.5 text-[12px] text-muted-foreground">Artifacts</span>
      {arts.map((a) => (
        <Tooltip
          key={a.id}
          content={[a.description, `${a.file_name} · ${bytes(a.size)}`].filter(Boolean).join('\n')}
        >
          <button
            type="button"
            onClick={() => openArtifact(a.id, ids)}
            className="inline-flex max-w-64 items-center gap-1.5 rounded-md border border-border bg-surface-2 px-2 py-1 text-[12px] font-medium outline-none hover:bg-surface-3 focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            {a.unread && <span className="size-1.5 shrink-0 rounded-full bg-accent" />}
            <FileText className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate">{a.title || a.file_name}</span>
          </button>
        </Tooltip>
      ))}
    </div>
  )
}
