import {X} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {DateRange} from '@/components/ui/date-range'
import {SearchInput} from '@/components/ui/search-input'
import {Segmented} from '@/components/ui/segmented'
import {Select} from '@/components/ui/select'
import {Tooltip} from '@/components/ui/tooltip'
import {anyFilter, type ArtifactFilters, type Option} from './logic'
import {clearArtifactFilters, setArtifactFilters} from './use-filters'

// The filter bar sits above the list, not in the view's header: the header
// keeps the one action that applies to everything, the filters stay with what
// they filter.
export function ArtifactsToolbar({
  filters,
  groups,
  origins,
  group,
  origin,
}: {
  filters: ArtifactFilters
  groups: Option[]
  origins: Option[]
  // The menus' effective choices: a stale one already fell back to "all".
  group: string
  origin: string
}) {
  return (
    <>
      <SearchInput
        hotkey
        value={filters.query}
        onChange={(query) => setArtifactFilters({query})}
        placeholder="Search artifacts"
        className="w-60"
      />
      <Segmented
        aria-label="Read state"
        value={filters.unread ? 'unread' : 'all'}
        onValueChange={(v) => setArtifactFilters({unread: v === 'unread'})}
        options={[
          {value: 'all', label: 'All', tip: 'Show every artifact'},
          {value: 'unread', label: 'Unread', tip: 'Show only the artifacts you have not opened yet'},
        ]}
      />
      <Tooltip content="Show the artifacts of one queue group">
        <div className="w-40">
          <Select
            aria-label="Group"
            value={group}
            onValueChange={(v) => setArtifactFilters({group: v, origin: ''})}
            options={[{value: '', label: 'All groups'}, ...groups]}
          />
        </div>
      </Tooltip>
      <Tooltip content="Show the artifacts one job produced, directly or through the jobs it created">
        <div className="w-48">
          <Select
            aria-label="Parent job"
            value={origin}
            onValueChange={(v) => setArtifactFilters({origin: v})}
            options={[{value: '', label: 'All parents'}, ...origins]}
          />
        </div>
      </Tooltip>
      <span className="flex-1" />
      <DateRange
        from={filters.from}
        to={filters.to}
        onFrom={(from) => setArtifactFilters({from})}
        onTo={(to) => setArtifactFilters({to})}
      />
      {anyFilter(filters) && (
        <Button variant="ghost" size="sm" onClick={clearArtifactFilters}>
          <X />
          Clear
        </Button>
      )}
    </>
  )
}
