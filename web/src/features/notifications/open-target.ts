import {navigate} from '@/app/nav'
import {toast} from '@/components/ui/toaster'
import {openArtifact} from '@/features/artifacts/store'
import {openRunLog} from '@/features/log/store'
import {api, errorMessage} from '@/lib/api'
import {invalidate, keys, queryClient} from '@/lib/queries'
import type {AppNotification} from '@/lib/types'
import type {NotificationTarget} from '@/lib/native'
import {openExternal} from '@/lib/native'

// openTarget opens what a notification is about: the artifact, the run's log,
// or its link. A notification about nothing in particular (a provider's state)
// has nowhere to go. Reports whether there was anything to open.
export function openTarget(t: NotificationTarget): boolean {
  if (t.artifact_id) {
    openArtifact(t.artifact_id)
    return true
  }
  if (t.run_id) {
    openRunLog(t.run_id)
    return true
  }
  if (t.url) {
    openExternal(t.url)
    return true
  }
  return false
}

// patch changes the cached list straight away. A poll already in flight is
// cancelled first: an older answer landing last would paint the notification
// unread again.
async function patch(fn: (n: AppNotification) => AppNotification) {
  await queryClient.cancelQueries({queryKey: keys.notifications})
  queryClient.setQueryData<AppNotification[]>(keys.notifications, (old) => old?.map(fn))
}

export async function markNotificationRead(id: string) {
  await patch((n) => (n.id === id ? {...n, unread: false} : n))
  try {
    await api('POST', `/api/notifications/${encodeURIComponent(id)}/read`)
  } catch {
    // The refetch below shows the notification as the daemon has it.
  }
  await invalidate(keys.notifications)
}

export async function markAllNotificationsRead() {
  await patch((n) => ({...n, unread: false}))
  try {
    await api('POST', '/api/notifications/read-all')
    toast.success('All marked read')
  } catch (e) {
    toast.error('Could not mark all read', {description: errorMessage(e)})
  }
  await invalidate(keys.notifications)
}

// openNotificationTarget is what a click on a macOS notification calls (through
// window.cqOpenNotificationTarget): mark it read, open what it is about, or
// fall back to the Notifications view. The target is not kept waiting for the
// daemon's answer to the read mark.
export function openNotificationTarget(t: NotificationTarget) {
  if (t.id) void markNotificationRead(t.id)
  else void invalidate(keys.notifications)
  if (!openTarget(t)) navigate('notifications')
}
