import {useEffect} from 'react'
import {ConfirmHost} from '@/components/ui/confirm'
import {Toaster} from '@/components/ui/toaster'
import {TooltipProvider} from '@/components/ui/tooltip'
import {ArtifactViewer} from '@/features/artifacts/ArtifactViewer'
import {LogSheet} from '@/features/log/LogSheet'
import {StatusBanners} from '@/features/status/StatusBanners'
import {TaskEditor} from '@/features/task-editor/TaskEditor'
import {WhatsNewDialog} from '@/features/whats-new/WhatsNewDialog'
import {useSettings} from '@/lib/queries'
import {pinnedAppearance, useTheme, useThemeEffects, validAccent} from '@/lib/theme'
import {CommandPalette} from './CommandPalette'
import {navigate, useNav} from './nav'
import {Sidebar} from './Sidebar'
import {VIEWS, viewDef} from './views'

export function App() {
  useThemeEffects()
  useSettingsTheme()
  useViewShortcuts()
  const view = useNav((s) => s.view)
  const View = viewDef(view).component

  return (
    <TooltipProvider delayDuration={350} skipDelayDuration={150}>
      <div className="flex h-full">
        <Sidebar />
        <main className="flex min-w-0 flex-1 flex-col">
          <StatusBanners />
          <div className="min-h-0 flex-1">
            <View key={view} />
          </div>
        </main>
      </div>
      <TaskEditor />
      <LogSheet />
      <ArtifactViewer />
      <CommandPalette />
      <WhatsNewDialog />
      <ConfirmHost />
      <Toaster />
    </TooltipProvider>
  )
}

// The daemon holds the appearance and accent; whatever it says (changed in
// another window, or by hand in config.toml) is what the page shows.
function useSettingsTheme() {
  const settings = useSettings().data
  useEffect(() => {
    if (!settings) return
    useTheme.setState({
      appearance: pinnedAppearance(settings.appearance),
      accent: validAccent(settings.accent),
    })
  }, [settings?.appearance, settings?.accent]) // eslint-disable-line react-hooks/exhaustive-deps
}

// ⌘1…⌘8 switch views in sidebar order.
function useViewShortcuts() {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return
      const n = Number(e.key)
      if (!Number.isInteger(n) || n < 1 || n > VIEWS.length) return
      if (document.querySelector('[role=dialog]')) return
      e.preventDefault()
      navigate(VIEWS[n - 1].id)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])
}
