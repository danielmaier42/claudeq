import {X} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {DateRange} from '@/components/ui/date-range'
import {SearchInput} from '@/components/ui/search-input'
import {Segmented} from '@/components/ui/segmented'
import {Select} from '@/components/ui/select'
import {clearLogFilters, setLogFilters} from '../filters-store'
import {anyFilter, type FilterMenus, type LogFilters, type MenuOption, type Show} from '../runs'

// The filter row under the Log's title: search and the All | Unread | Running
// switch first, then the menus (filled from the runs there are) and the days.
// A menu with a single entry has nothing to choose and stays out of the way.
// `f` is what the list applies: a stale menu choice already fell back to all.
export function LogToolbar({f, menus}: {f: LogFilters; menus: FilterMenus}) {
  return (
    <>
      <SearchInput
        value={f.query}
        onChange={(query) => setLogFilters({query})}
        placeholder="Search runs"
        hotkey
        className="w-60"
      />
      <Segmented<Show>
        aria-label="Show"
        value={f.show}
        onValueChange={(show) => setLogFilters({show})}
        options={[
          {value: '', label: 'All', tip: 'Show every run'},
          {value: 'unread', label: 'Unread', tip: 'Show only the runs you have not looked at yet'},
          {value: 'running', label: 'Running', tip: 'Show only the runs that are executing right now'},
        ]}
      />
      <Menu
        label="All groups"
        tip="Runs of one queue group"
        value={f.group}
        options={menus.groups}
        set={(group) => setLogFilters({group, task: ''})}
      />
      <Menu
        label="All tasks"
        tip="Runs of one task"
        value={f.task}
        options={menus.tasks}
        set={(task) => setLogFilters({task})}
      />
      <Menu
        label="All providers"
        tip="Runs on one provider"
        value={f.provider}
        options={menus.providers}
        set={(provider) => setLogFilters({provider})}
      />
      <Menu
        label="All outcomes"
        tip="Runs with one outcome"
        value={f.status}
        options={menus.statuses}
        set={(status) => setLogFilters({status})}
      />
      <div className="flex-1" />
      <DateRange
        from={f.from}
        to={f.to}
        onFrom={(from) => setLogFilters({from})}
        onTo={(to) => setLogFilters({to})}
      />
      {anyFilter(f) && (
        <Button variant="ghost" size="sm" onClick={clearLogFilters}>
          <X />
          Clear
        </Button>
      )}
    </>
  )
}

function Menu({
  label,
  tip,
  value,
  options,
  set,
}: {
  label: string
  tip: string
  value: string
  options: MenuOption[]
  set: (v: string) => void
}) {
  if (options.length < 2 && !value) return null
  return (
    <Select
      aria-label={tip}
      value={value}
      onValueChange={set}
      options={[{value: '', label}, ...options]}
      className="w-auto max-w-48"
    />
  )
}
