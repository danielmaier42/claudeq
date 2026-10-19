import {create} from 'zustand'
import {DEFAULT_STATS_RANGE} from '@/lib/queries'
import type {StatsRange} from '@/lib/types'

// The chosen period outlives a switch to another view and back, for the rest
// of the session; a fresh window starts on the default again.
export const useUsageRange = create<{range: StatsRange}>(() => ({range: DEFAULT_STATS_RANGE}))
export const setUsageRange = (range: StatsRange) => useUsageRange.setState({range})
