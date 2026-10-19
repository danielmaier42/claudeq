import {toast} from '@/components/ui/toaster'
import {ApiError, errorMessage, get, post, put} from '@/lib/api'
import {invalidate, keys, queryClient} from '@/lib/queries'
import type {PauseState, Settings} from '@/lib/types'
import {editedSections, fieldError, hasEdits, mergeSettings, pruneEdits, type SettingsEdits} from './draft'
import {setSection, useSettingsUi} from './store'

// Two ways a setting is written, and the form says which applies where:
//   - switches and pickers apply on the spot (commitSettings), optimistically:
//     a choice is judged by seeing it take effect;
//   - text is kept as a draft until Save (saveDraft): half a token written on
//     every keystroke would be refused, or worse, accepted.
// Both start from the latest stored settings, never from what the form loaded.

export async function writeOver(patch: SettingsEdits): Promise<Settings> {
  const latest = await get<Settings>('/api/settings')
  const saved = await put<Settings>('/api/settings', mergeSettings(latest, patch))
  queryClient.setQueryData(keys.settings, saved)
  return saved
}

export async function commitSettings(patch: SettingsEdits): Promise<boolean> {
  await queryClient.cancelQueries({queryKey: keys.settings})
  const before = queryClient.getQueryData<Settings>(keys.settings)
  if (before) queryClient.setQueryData(keys.settings, mergeSettings(before, patch))
  try {
    await writeOver(patch)
    return true
  } catch (e) {
    if (before) queryClient.setQueryData(keys.settings, before)
    toast.error('Could not save the setting', {description: errorMessage(e)})
    invalidate(keys.settings)
    return false
  }
}

export async function saveDraft() {
  const {edits, saving} = useSettingsUi.getState()
  const stored = queryClient.getQueryData<Settings>(keys.settings)
  if (saving || !stored) return
  const patch = pruneEdits(stored, edits)
  if (!hasEdits(patch)) {
    useSettingsUi.setState({edits: {}, errors: {}})
    return
  }
  useSettingsUi.setState({saving: true})
  try {
    const saved = await writeOver(patch)
    // Keep what was typed while the save was on its way: only the edits the
    // daemon now holds are done.
    useSettingsUi.setState((s) => ({edits: pruneEdits(saved, s.edits), errors: {}}))
    toast.success('Settings saved')
  } catch (e) {
    const msg = errorMessage(e)
    const hit = e instanceof ApiError && e.status === 400 ? fieldError(msg) : null
    if (hit) {
      // Show the refusal where the field is, opening its section if need be.
      useSettingsUi.setState((s) => ({errors: {...s.errors, [hit.field]: hit.text}}))
      setSection('notifications')
    } else {
      const [first] = editedSections(patch)
      if (first) setSection(first)
    }
    toast.error('Settings not saved', {description: msg})
  } finally {
    useSettingsUi.setState({saving: false})
  }
}

// setPaused flips the global stop switch through its own endpoint: the server
// keeps the stored value on a settings PUT, so it never rides along with Save.
export async function setPaused(paused: boolean) {
  await queryClient.cancelQueries({queryKey: keys.settings})
  const before = queryClient.getQueryData<Settings>(keys.settings)
  if (before) queryClient.setQueryData(keys.settings, {...before, paused})
  try {
    await post<PauseState>('/api/pause', {paused})
    toast.success(paused ? 'All runs paused' : 'Runs resumed')
  } catch (e) {
    // The write failed, so the queue is still in the old state: put the switch
    // back rather than leaving it claiming something that never happened.
    if (before) queryClient.setQueryData(keys.settings, before)
    toast.error(paused ? 'Could not pause runs' : 'Could not resume runs', {description: errorMessage(e)})
  }
  invalidate(keys.settings, keys.tasks, keys.health)
}
