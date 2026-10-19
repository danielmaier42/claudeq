import {create} from 'zustand'

// The All | Active filter and the search outlive a switch to another view and
// back, like the first UI's module variable did.
export const useQueueFilter = create<{onlyActive: boolean; query: string}>(() => ({
  onlyActive: false,
  query: '',
}))
export const setOnlyActive = (onlyActive: boolean) => useQueueFilter.setState({onlyActive})
export const setQueueQuery = (query: string) => useQueueFilter.setState({query})
