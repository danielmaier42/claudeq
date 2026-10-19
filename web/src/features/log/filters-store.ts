import {create} from 'zustand'
import {emptyFilters, type LogFilters} from './runs'

// The Log's filters outlive the view: switching to the Queue and back finds
// the list as it was left. `limit` is how many matching runs are shown; any
// change of filter starts again at the first page.
export const PAGE = 25

interface State extends LogFilters {
  limit: number
}

export const useLogFilters = create<State>(() => ({...emptyFilters, limit: PAGE}))

export const setLogFilters = (patch: Partial<LogFilters>) => useLogFilters.setState({...patch, limit: PAGE})
export const clearLogFilters = () => useLogFilters.setState({...emptyFilters, limit: PAGE})
export const showMoreRuns = () => useLogFilters.setState((s) => ({limit: s.limit + PAGE}))

export const filtersOf = ({
  query,
  show,
  group,
  task,
  provider,
  status,
  from,
  to,
}: LogFilters): LogFilters => ({
  query,
  show,
  group,
  task,
  provider,
  status,
  from,
  to,
})
