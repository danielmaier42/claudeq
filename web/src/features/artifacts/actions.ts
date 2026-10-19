import {confirm} from '@/components/ui/confirm'
import {toast} from '@/components/ui/toaster'
import {api, errorMessage} from '@/lib/api'
import {openExternal} from '@/lib/native'
import {invalidate, keys, queryClient} from '@/lib/queries'
import type {Artifact} from '@/lib/types'
import {contentURL} from './logic'

// patch changes the cached list straight away, so a read mark or a delete shows
// before the daemon answers. A poll already in flight is cancelled first: an
// older answer landing last would paint the artifact unread again.
async function patch(fn: (arts: Artifact[]) => Artifact[]) {
  await queryClient.cancelQueries({queryKey: keys.artifacts})
  queryClient.setQueryData<Artifact[]>(keys.artifacts, (old) => (old ? fn(old) : old))
}

export async function markArtifactRead(id: string) {
  await patch((arts) => arts.map((a) => (a.id === id ? {...a, unread: false} : a)))
  try {
    await api('POST', `/api/artifacts/${encodeURIComponent(id)}/read`)
  } catch {
    // The refetch below shows the artifact as the daemon has it.
  }
  await invalidate(keys.artifacts)
}

// deleteArtifact asks first. onGone runs once the daemon has deleted it and
// before the list drops it, so the viewer can step away from it in time.
export async function deleteArtifact(a: Artifact, onGone?: () => void) {
  const ok = await confirm({
    title: `Delete “${a.title}”?`,
    description: 'This removes the stored copy of the file. The task that published it is not affected.',
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  try {
    await api('DELETE', `/api/artifacts/${encodeURIComponent(a.id)}`)
    onGone?.()
    await patch((arts) => arts.filter((x) => x.id !== a.id))
    toast.success('Artifact deleted', {description: a.title})
  } catch (e) {
    toast.error('Could not delete the artifact', {description: errorMessage(e)})
  } finally {
    void invalidate(keys.artifacts)
  }
}

// Opening an artifact — in the viewer or externally — counts as reading it.
export function openArtifactExternal(a: Artifact) {
  openExternal(location.origin + contentURL(a.id))
  if (a.unread) void markArtifactRead(a.id)
}

export function downloadArtifact(a: Artifact) {
  openExternal(location.origin + contentURL(a.id, true))
}
