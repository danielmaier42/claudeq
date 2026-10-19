import {
  Bell,
  ChartNoAxesColumn,
  FileText,
  History,
  LayoutDashboard,
  ListOrdered,
  MessageSquareText,
  Settings as SettingsIcon,
  type LucideIcon,
} from 'lucide-react'
import type {ComponentType} from 'react'
import {ArtifactsView} from '@/features/artifacts/ArtifactsView'
import {DashboardView} from '@/features/dashboard/DashboardView'
import {FeedbackView} from '@/features/feedback/FeedbackView'
import {LogView} from '@/features/log/LogView'
import {NotificationsView} from '@/features/notifications/NotificationsView'
import {QueueView} from '@/features/queue/QueueView'
import {SettingsView} from '@/features/settings/SettingsView'
import {UsageView} from '@/features/usage/UsageView'
import type {ViewId} from './nav'

export interface ViewDef {
  id: ViewId
  label: string
  icon: LucideIcon
  component: ComponentType
  // foot views sit at the bottom of the sidebar.
  foot?: boolean
}

// Every view the sidebar reaches, in sidebar order. ⌘1…⌘8 follow this order.
export const VIEWS: ViewDef[] = [
  {id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard, component: DashboardView},
  {id: 'queue', label: 'Queue', icon: ListOrdered, component: QueueView},
  {id: 'log', label: 'Log', icon: History, component: LogView},
  {id: 'artifacts', label: 'Artifacts', icon: FileText, component: ArtifactsView},
  {id: 'notifications', label: 'Notifications', icon: Bell, component: NotificationsView},
  {id: 'usage', label: 'Usage', icon: ChartNoAxesColumn, component: UsageView},
  {id: 'feedback', label: 'Feedback', icon: MessageSquareText, component: FeedbackView, foot: true},
  {id: 'settings', label: 'Settings', icon: SettingsIcon, component: SettingsView, foot: true},
]

export const viewDef = (id: ViewId) => VIEWS.find((v) => v.id === id) ?? VIEWS[0]
