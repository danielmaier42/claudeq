import {useIsFetching} from '@tanstack/react-query'
import {ExternalLink, Plus, Search} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {CountBadge} from '@/components/ui/badge'
import {Kbd} from '@/components/ui/kbd'
import {Tooltip} from '@/components/ui/tooltip'
import {openAddTask} from '@/features/task-editor/store'
import {useHealth, useSettings, useUpdate} from '@/lib/queries'
import {openExternal} from '@/lib/native'
import {cn} from '@/lib/utils'
import {useBadges} from './badges'
import {navigate, useNav} from './nav'
import {openPalette} from './palette-store'
import {VIEWS, type ViewDef} from './views'

const REPO = 'https://github.com/danielmaier42/claudeq'

export function Sidebar() {
  const view = useNav((s) => s.view)
  const badges = useBadges()
  const main = VIEWS.filter((v) => !v.foot)
  const foot = VIEWS.filter((v) => v.foot)

  return (
    <aside className="flex w-[228px] shrink-0 flex-col border-r border-border bg-sidebar">
      <div className="flex items-center gap-2.5 px-4 pt-4 pb-3">
        <img src="/logo.svg" alt="" className="size-7 rounded-lg shadow-sm" />
        <div className="min-w-0 flex-1">
          <div className="text-[14px] leading-4 font-semibold tracking-[-0.01em]">DispatchQ</div>
          <Version />
        </div>
      </div>

      <div className="flex flex-col gap-1.5 px-3 pb-3">
        <Button variant="primary" className="w-full justify-start" onClick={() => openAddTask()}>
          <Plus /> New task
        </Button>
        <button
          onClick={openPalette}
          className="flex h-8 items-center gap-2 rounded-md border border-border bg-surface px-2.5 text-[12.5px] text-subtle-foreground shadow-xs transition-colors hover:text-foreground"
        >
          <Search className="size-3.5" />
          <span className="flex-1 text-left">Search & commands</span>
          <Kbd>⌘K</Kbd>
        </button>
      </div>

      <nav className="flex flex-col gap-0.5 px-2" aria-label="Views">
        {main.map((v, i) => (
          <NavItem key={v.id} def={v} active={view === v.id} count={badges[v.id] ?? 0} shortcut={i + 1} />
        ))}
      </nav>

      <div className="flex-1" />

      <nav className="flex flex-col gap-0.5 px-2 pb-2" aria-label="More">
        {foot.map((v, i) => (
          <NavItem
            key={v.id}
            def={v}
            active={view === v.id}
            count={badges[v.id] ?? 0}
            shortcut={main.length + i + 1}
          />
        ))}
      </nav>
      <Footer />
    </aside>
  )
}

function NavItem({
  def,
  active,
  count,
  shortcut,
}: {
  def: ViewDef
  active: boolean
  count: number
  shortcut: number
}) {
  const Icon = def.icon
  return (
    <button
      onClick={() => navigate(def.id)}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'group flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium transition-colors',
        active
          ? 'bg-accent-soft text-accent-text'
          : 'text-foreground/80 hover:bg-surface-3/70 hover:text-foreground',
      )}
    >
      <Icon
        className={cn(
          'size-4 shrink-0',
          active ? 'text-accent-text' : 'text-muted-foreground group-hover:text-foreground',
        )}
      />
      <span className="flex-1 truncate text-left">{def.label}</span>
      {count > 0 ? (
        <CountBadge count={count} className={def.id === 'settings' ? 'bg-danger text-white' : undefined} />
      ) : (
        <span className="text-[11px] text-subtle-foreground opacity-0 transition-opacity group-hover:opacity-100">
          ⌘{shortcut}
        </span>
      )}
    </button>
  )
}

function Version() {
  const update = useUpdate().data
  if (!update?.current) return <div className="text-[11.5px] text-subtle-foreground">Task queue</div>
  return (
    <div className="truncate text-[11.5px] text-subtle-foreground">
      {update.supported ? 'v' + update.current : update.current}
    </div>
  )
}

// The footer says whether the daemon answers and whether runs are paused.
function Footer() {
  const health = useHealth()
  const settings = useSettings().data
  const fetching = useIsFetching() > 0
  const down = health.isError
  const paused = settings?.paused
  const state = down
    ? {dot: 'bg-danger', text: 'Daemon not reachable'}
    : paused
      ? {dot: 'bg-warning', text: 'All runs paused'}
      : {dot: 'bg-success', text: 'Connected'}
  return (
    <div className="flex items-center gap-2 border-t border-border px-4 py-2.5 text-[12px] text-muted-foreground">
      <span className={cn('size-2 rounded-full', state.dot, fetching && !down && 'animate-pulse-dot')} />
      <span className="flex-1 truncate">{state.text}</span>
      <Tooltip content="DispatchQ on GitHub">
        <button
          onClick={() => openExternal(REPO)}
          className="rounded p-1 hover:bg-surface-3 hover:text-foreground"
          aria-label="DispatchQ on GitHub"
        >
          <ExternalLink className="size-3.5" />
        </button>
      </Tooltip>
    </div>
  )
}
