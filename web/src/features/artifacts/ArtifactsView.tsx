import {Eye, FolderOpen, SearchX} from 'lucide-react'
import {useMemo, useState} from 'react'
import {DayHeading, ViewLayout} from '@/components/layout/view-layout'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {EmptyState} from '@/components/ui/empty-state'
import {Skeleton} from '@/components/ui/skeleton'
import {errorMessage} from '@/lib/api'
import {groupByDay} from '@/lib/format'
import {useArtifacts} from '@/lib/queries'
import {markAllArtifactsRead} from './actions'
import {ArtifactCard} from './ArtifactCard'
import {ArtifactsToolbar} from './ArtifactsToolbar'
import {anyFilter, shown, sourceOptions} from './logic'
import {openArtifact} from './store'
import {clearArtifactFilters, useArtifactFilters} from './use-filters'

// Cards come in batches: a store with hundreds of artifacts should not render
// them all before the first one shows.
const BATCH = 48

export function ArtifactsView() {
  const {data: arts, isPending, error} = useArtifacts()
  const filters = useArtifactFilters()
  // A new filter starts again from the first batch.
  const [paging, setPaging] = useState({filters, limit: BATCH})
  const limit = paging.filters === filters ? paging.limit : BATCH
  const [marking, setMarking] = useState(false)

  const {groups, origins, group, origin} = useMemo(() => sourceOptions(arts ?? [], filters), [arts, filters])
  const effective = useMemo(() => ({...filters, group, origin}), [filters, group, origin])
  const filtered = useMemo(() => (arts ?? []).filter((a) => shown(a, effective)), [arts, effective])
  const visible = filtered.slice(0, limit)
  const days = groupByDay(visible, (a) => a.published_at)
  const unread = arts?.filter((a) => a.unread).length ?? 0
  // The viewer steps through the list as it is filtered here.
  const open = (id: string) =>
    openArtifact(
      id,
      filtered.map((a) => a.id),
    )

  const markAll = async () => {
    setMarking(true)
    await markAllArtifactsRead()
    setMarking(false)
  }

  const subtitle = arts?.length
    ? `${arts.length} artifact${arts.length === 1 ? '' : 's'}${unread ? ` · ${unread} unread` : ''}`
    : 'Files your tasks publish'

  return (
    <ViewLayout
      title="Artifacts"
      subtitle={subtitle}
      wide
      actions={
        <Button onClick={markAll} loading={marking} disabled={!unread}>
          {!marking && <Eye />}
          Mark all read
        </Button>
      }
      toolbar={
        arts?.length ? (
          <ArtifactsToolbar
            filters={filters}
            groups={groups}
            origins={origins}
            group={group}
            origin={origin}
          />
        ) : undefined
      }
    >
      {isPending ? (
        <CardSkeletons />
      ) : error && !arts ? (
        <Alert tone="danger" title="Could not load artifacts">
          {errorMessage(error)}
        </Alert>
      ) : !arts?.length ? (
        <EmptyState
          icon={<FolderOpen />}
          title="No artifacts yet"
          description="Files your tasks publish with “claudeq publish” appear here — reports, exports, HTML pages, PDFs."
        />
      ) : !filtered.length ? (
        <EmptyState
          icon={<SearchX />}
          title="No artifacts match"
          description={
            filters.query.trim()
              ? `Nothing matches “${filters.query.trim()}”. Try fewer or different words.`
              : filters.unread
                ? 'Everything here has been read. Switch to All to see the rest.'
                : 'Adjust the date, group or parent filter to see artifacts.'
          }
          action={
            <Button variant="secondary" onClick={clearArtifactFilters}>
              Clear filters
            </Button>
          }
        />
      ) : (
        <>
          {days.map((d) => (
            <section key={d.day} className="mb-5 last:mb-0">
              <DayHeading>{d.label}</DayHeading>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
                {d.items.map((a) => (
                  <ArtifactCard key={a.id} a={a} onOpen={() => open(a.id)} />
                ))}
              </div>
            </section>
          ))}
          <div className="mt-6 flex items-center justify-between gap-3 border-t border-border pt-3 text-[12.5px] text-muted-foreground">
            <span>
              {filtered.length} artifact{filtered.length === 1 ? '' : 's'}
              {anyFilter(filters) ? ' matching' : ''}
              {visible.length < filtered.length ? ` · showing ${visible.length}` : ''}
            </span>
            {visible.length < filtered.length && (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setPaging({filters, limit: limit + BATCH})}
              >
                Show {Math.min(BATCH, filtered.length - visible.length)} more
              </Button>
            )}
          </div>
        </>
      )}
    </ViewLayout>
  )
}

function CardSkeletons() {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3 pt-7">
      {Array.from({length: 8}, (_, i) => (
        <div key={i} className="flex h-[148px] flex-col rounded-xl border border-border bg-surface p-3.5">
          <div className="flex gap-3">
            <Skeleton className="size-9 rounded-lg" />
            <div className="flex-1 space-y-2 pt-1">
              <Skeleton className="h-3.5 w-4/5" />
              <Skeleton className="h-3.5 w-1/2" />
            </div>
          </div>
          <Skeleton className="mt-3 h-3 w-full" />
          <Skeleton className="mt-auto h-3 w-2/3" />
        </div>
      ))}
    </div>
  )
}
