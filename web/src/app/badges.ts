import {useArtifacts, useNotifications, useRuns, useUpdate} from '@/lib/queries'
import type {ViewId} from './nav'

// useBadges counts what is waiting behind each sidebar entry: unread runs,
// artifacts and notifications, and an update that is ready.
export function useBadges(): Partial<Record<ViewId, number>> {
  const runs = useRuns().data
  const artifacts = useArtifacts().data
  const notes = useNotifications().data
  const update = useUpdate().data
  const unread = (xs?: {unread: boolean}[]) => xs?.filter((x) => x.unread).length ?? 0
  return {
    log: unread(runs),
    artifacts: unread(artifacts),
    notifications: unread(notes),
    settings: update && (update.available || update.restart_required) ? 1 : 0,
  }
}
