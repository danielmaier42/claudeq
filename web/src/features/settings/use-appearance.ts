import {useCallback} from 'react'
import {useTheme, validAccent, type Appearance} from '@/lib/theme'
import type {Settings} from '@/lib/types'
import {commitSettings} from './actions'

// saveLook applies a new appearance and/or accent at once and stores it like any
// other instant setting; a refused save puts the old look back.
async function saveLook(patch: Partial<Pick<Settings, 'appearance' | 'accent'>>) {
  const before = useTheme.getState()
  if (patch.appearance !== undefined) before.setAppearance(patch.appearance)
  if (patch.accent !== undefined) before.setAccent(patch.accent)
  if (!(await commitSettings(patch)))
    useTheme.setState({appearance: before.appearance, accent: before.accent})
}

export function useSaveAppearance() {
  return useCallback((appearance: Appearance) => saveLook({appearance}), [])
}

export function useSaveAccent() {
  return useCallback((accent: string) => saveLook({accent: validAccent(accent)}), [])
}
