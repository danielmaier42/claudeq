import {create} from 'zustand'
import {NO_FILTERS, type NotificationFilters} from './logic'

// The filters live outside the view so they are still set after a trip to
// another view, as they were in the first UI.
export const useNotificationFilters = create<NotificationFilters>(() => NO_FILTERS)

export const setNotificationFilters = (patch: Partial<NotificationFilters>) =>
  useNotificationFilters.setState(patch)
export const clearNotificationFilters = () => useNotificationFilters.setState(NO_FILTERS)
