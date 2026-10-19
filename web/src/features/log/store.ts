import {create} from 'zustand'

// The run log sheet: open it for one run from anywhere (Log, Queue, a
// notification click). LogSheet.tsx is mounted once by the shell.
export const useRunLog = create<{runId: string | null}>(() => ({runId: null}))

export const openRunLog = (runId: string) => useRunLog.setState({runId})
export const closeRunLog = () => useRunLog.setState({runId: null})
