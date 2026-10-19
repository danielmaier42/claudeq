import {openNotificationTarget} from '@/features/notifications/open-target'
import {openAddTask} from '@/features/task-editor/store'
import {refreshAll} from '@/lib/queries'
import {navigate} from './nav'

// The app window's menu bar drives the page through these, and a click on a
// macOS notification through cqOpenNotificationTarget. This list is exactly
// what cmd/dispatchqapp may call. They are published when this module loads, not
// from a React effect: the window's DOMContentLoaded handler takes a
// notification clicked during launch, and must find its opener already there.
export function installNativeBridge() {
  window.openAdd = () => openAddTask()
  window.select = (v: string) => navigate(v)
  window.cqRefresh = () => void refreshAll()
  window.cqOpenNotificationTarget = openNotificationTarget
}
