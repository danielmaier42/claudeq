import {create} from 'zustand'

// The All | Active filter outlives a switch to another view and back, like the
// first UI's module variable did.
export const useQueueFilter = create<{onlyActive: boolean}>(() => ({onlyActive: false}))
export const setOnlyActive = (onlyActive: boolean) => useQueueFilter.setState({onlyActive})
