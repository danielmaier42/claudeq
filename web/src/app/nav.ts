import {create} from 'zustand'

// Which view is open. The hash mirrors it (#/queue), so a reload stays put and
// the browser's back button works in a tab.
export const VIEW_IDS = [
  'dashboard',
  'queue',
  'log',
  'artifacts',
  'notifications',
  'usage',
  'feedback',
  'settings',
] as const
export type ViewId = (typeof VIEW_IDS)[number]

// The native menu and older links still use the view ids of the first UI.
const LEGACY: Record<string, ViewId> = {tasks: 'queue', news: 'log', activity: 'log'}

export function toViewId(v: string): ViewId | null {
  const id = LEGACY[v] ?? v
  return (VIEW_IDS as readonly string[]).includes(id) ? (id as ViewId) : null
}

interface NavState {
  view: ViewId
  // The Settings section to show when Settings opens; the view clears it.
  settingsSection: string
}

function fromHash(): ViewId {
  return toViewId(location.hash.replace(/^#\/?/, '').split('/')[0]) ?? 'dashboard'
}

export const useNav = create<NavState>(() => ({view: fromHash(), settingsSection: ''}))

export function navigate(view: string, settingsSection = '') {
  const id = toViewId(view)
  if (!id) return
  useNav.setState({view: id, settingsSection})
  // A history entry per view, so Back in a browser tab returns to the last one.
  if (fromHash() !== id || !location.hash) history.pushState(null, '', '#/' + id)
}

export const openSettings = (section = '') => navigate('settings', section)

window.addEventListener('hashchange', () => useNav.setState({view: fromHash()}))
