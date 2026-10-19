import {create} from 'zustand'
import {NO_FILTERS, type ArtifactFilters} from './logic'

// The filters live outside the view so they are still set after a trip to
// another view, as they were in the first UI.
export const useArtifactFilters = create<ArtifactFilters>(() => NO_FILTERS)

export const setArtifactFilters = (patch: Partial<ArtifactFilters>) => useArtifactFilters.setState(patch)
export const clearArtifactFilters = () => useArtifactFilters.setState(NO_FILTERS)
