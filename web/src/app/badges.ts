import {useNotifications, useUpdate} from '@/lib/queries'
import type {ViewId} from './nav'

// useBadges counts what is waiting behind a sidebar entry. Only Notifications
// keeps a read state worth counting: a run or an artifact that matters arrives
// there too, so the Log and Artifacts stay quiet lists. Settings shows a ready
// update.
export function useBadges(): Partial<Record<ViewId, number>> {
  const notes = useNotifications().data
  const update = useUpdate().data
  return {
    notifications: notes?.filter((n) => n.unread).length ?? 0,
    settings: update && (update.available || update.restart_required) ? 1 : 0,
  }
}
