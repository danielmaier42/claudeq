import {DateRange, dateRangeLabel} from '@/components/ui/date-range'
import {Field} from '@/components/ui/field'
import {FilterChip} from '@/components/ui/filter-chip'
import {FilterPopover, FilterSelect, optionLabel} from '@/components/ui/filter-popover'
import {SearchInput} from '@/components/ui/search-input'
import type {ArtifactFilters, Option} from './logic'
import {setArtifactFilters} from './use-filters'

// The filter bar sits above the list, not in the view's header: the header
// keeps the one action that applies to everything, the filters stay with what
// they filter. Search stays in sight; group, parent and days sit behind the
// Filter button and show as chips once set.
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
  const days = dateRangeLabel(filters.from, filters.to)
  const active = (group ? 1 : 0) + (origin ? 1 : 0) + (days ? 1 : 0)

  return (
    <>
      <SearchInput
        hotkey
        value={filters.query}
        onChange={(query) => setArtifactFilters({query})}
        placeholder="Search artifacts"
        className="w-60"
      />
      <FilterPopover
        active={active}
        onReset={() => setArtifactFilters({group: '', origin: '', from: '', to: ''})}
      >
        <FilterSelect
          label="Group"
          hint="The artifacts of one queue group"
          all="All groups"
          value={group}
          options={groups}
          onChange={(v) => setArtifactFilters({group: v, origin: ''})}
        />
        <FilterSelect
          label="Parent job"
          hint="What one job produced, directly or through the jobs it created"
          all="All parents"
          value={origin}
          options={origins}
          onChange={(v) => setArtifactFilters({origin: v})}
        />
        <Field label="Published">
          <DateRange
            from={filters.from}
            to={filters.to}
            onFrom={(from) => setArtifactFilters({from})}
            onTo={(to) => setArtifactFilters({to})}
          />
        </Field>
      </FilterPopover>
      {group && (
        <FilterChip
          label="Group"
          value={optionLabel(groups, group)}
          onRemove={() => setArtifactFilters({group: ''})}
        />
      )}
      {origin && (
        <FilterChip
          label="Parent"
          value={optionLabel(origins, origin)}
          onRemove={() => setArtifactFilters({origin: ''})}
        />
      )}
      {days && <FilterChip value={days} onRemove={() => setArtifactFilters({from: '', to: ''})} />}
    </>
  )
}
