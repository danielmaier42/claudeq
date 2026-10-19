import {create} from 'zustand'

// How the artifacts are laid out: cards (the default) or a compact list. A
// preference of this window, so it is kept in localStorage, not in the daemon.
export type ArtifactLayout = 'grid' | 'list'

const KEY = 'cq-artifacts-layout'

function stored(): ArtifactLayout {
  try {
    return localStorage.getItem(KEY) === 'list' ? 'list' : 'grid'
  } catch {
    return 'grid'
  }
}

export const useArtifactLayout = create<{layout: ArtifactLayout}>(() => ({layout: stored()}))

export function setArtifactLayout(layout: ArtifactLayout) {
  useArtifactLayout.setState({layout})
  try {
    localStorage.setItem(KEY, layout)
  } catch {
    /* private mode: the choice lasts for this session */
  }
}
