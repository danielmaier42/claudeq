import {useRef, type KeyboardEvent} from 'react'
import {Tooltip} from '@/components/ui/tooltip'
import {cn} from '@/lib/utils'
import type {SectionDef, SectionId} from '../sections'

export type NavMark = 'update' | 'unsaved'

const MARK_TIP: Record<NavMark, string> = {
  update: 'An update is ready',
  unsaved: 'Unsaved changes',
}

// SettingsNav is the section list on the left. It is a vertical tab list:
// ↑/↓ (and Home/End) move between sections, as in System Settings.
export function SettingsNav({
  sections,
  current,
  marks,
  onSelect,
}: {
  sections: SectionDef[]
  current: SectionId
  marks: Partial<Record<SectionId, NavMark>>
  onSelect: (id: SectionId) => void
}) {
  const ref = useRef<HTMLDivElement>(null)

  const onKeyDown = (e: KeyboardEvent) => {
    const i = sections.findIndex((s) => s.id === current)
    const next =
      e.key === 'ArrowDown' || e.key === 'ArrowRight'
        ? (i + 1) % sections.length
        : e.key === 'ArrowUp' || e.key === 'ArrowLeft'
          ? (i - 1 + sections.length) % sections.length
          : e.key === 'Home'
            ? 0
            : e.key === 'End'
              ? sections.length - 1
              : -1
    if (next < 0) return
    e.preventDefault()
    onSelect(sections[next].id)
    ref.current?.querySelectorAll<HTMLButtonElement>('[role=tab]')[next]?.focus()
  }

  return (
    <div
      ref={ref}
      role="tablist"
      aria-orientation="vertical"
      aria-label="Settings sections"
      onKeyDown={onKeyDown}
      className="-mx-1 flex gap-0.5 overflow-x-auto px-1 pb-1 md:sticky md:top-0 md:mx-0 md:flex-col md:overflow-visible md:px-0 md:pb-0"
    >
      {sections.map((s) => {
        const active = s.id === current
        const mark = marks[s.id]
        const Icon = s.icon
        return (
          <button
            key={s.id}
            role="tab"
            type="button"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            onClick={() => onSelect(s.id)}
            className={cn(
              'flex h-8 shrink-0 items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-2 focus-visible:ring-accent/40',
              active
                ? 'bg-surface-3/80 text-foreground dark:bg-surface-3'
                : 'text-muted-foreground hover:bg-surface-3/50 hover:text-foreground',
            )}
          >
            <Icon className={cn('size-4 shrink-0', active ? 'text-accent-text' : 'text-subtle-foreground')} />
            <span className="flex-1">{s.label}</span>
            {mark && (
              <Tooltip content={MARK_TIP[mark]} side="right">
                <span
                  aria-label={MARK_TIP[mark]}
                  className={cn(
                    'size-2 shrink-0 rounded-full',
                    mark === 'update' ? 'bg-danger' : 'bg-accent',
                  )}
                />
              </Tooltip>
            )}
          </button>
        )
      })}
    </div>
  )
}
