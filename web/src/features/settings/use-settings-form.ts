import {useMemo} from 'react'
import {useSettings} from '@/lib/queries'
import {hasEdits, mergeSettings, pruneEdits} from './draft'
import {useSettingsUi} from './store'

// useSettingsForm is what the sections read: the stored settings with the
// unsaved edits laid over them, and which of those edits really differ.
export function useSettingsForm() {
  const query = useSettings()
  const edits = useSettingsUi((s) => s.edits)
  const stored = query.data
  return useMemo(() => {
    const pending = stored ? pruneEdits(stored, edits) : {}
    return {
      stored,
      value: stored ? mergeSettings(stored, edits) : undefined,
      pending,
      dirty: hasEdits(pending),
      isLoading: query.isLoading,
      error: query.error,
    }
  }, [stored, edits, query.isLoading, query.error])
}
