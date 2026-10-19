import {Check, ChevronDown} from 'lucide-react'
import {useId, useMemo, useRef, useState, type KeyboardEvent} from 'react'
import {Input} from '@/components/ui/input'
import {useGroups, useTasks} from '@/lib/queries'
import {cn} from '@/lib/utils'
import {MAX_GROUP_LEN} from '../form'

// GroupCombobox is a text field that suggests the queue's groups: picking one
// files the task there, typing a new name starts a group, and an empty field
// leaves it ungrouped.
export function GroupCombobox({
  id,
  value,
  invalid,
  onChange,
}: {
  id: string
  value: string
  invalid?: boolean
  onChange: (v: string) => void
}) {
  const groupsQ = useGroups().data
  const tasks = useTasks().data
  // The groups list is the daemon's own; the tasks fill in a group the
  // groups poll has not seen yet.
  const names = useMemo(() => {
    const all = [...(groupsQ ?? []).map((g) => g.name), ...(tasks ?? []).map((t) => t.group ?? '')]
    return [...new Set(all.filter(Boolean))]
  }, [groupsQ, tasks])

  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const listId = useId()
  const input = useRef<HTMLInputElement>(null)

  const q = value.trim().toLowerCase()
  // An exact name lists every group again, so the field can switch between them.
  const shown = names.some((n) => n.toLowerCase() === q)
    ? names
    : names.filter((n) => n.toLowerCase().includes(q))
  const visible = open && shown.length > 0

  const pick = (name: string) => {
    onChange(name)
    setOpen(false)
    setActive(-1)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (!open) return setOpen(true)
      const step = e.key === 'ArrowDown' ? 1 : -1
      setActive((i) => (shown.length ? (i + step + shown.length) % shown.length : -1))
    } else if (e.key === 'Escape' && visible) {
      e.preventDefault()
      setOpen(false)
    } else if (e.key === 'Enter' && visible && active >= 0 && !(e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      pick(shown[active])
    }
  }

  return (
    <div className="relative">
      <Input
        ref={input}
        id={id}
        value={value}
        role="combobox"
        aria-expanded={visible}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={visible && active >= 0 ? `${listId}-${active}` : undefined}
        aria-invalid={invalid}
        placeholder="No group"
        maxLength={MAX_GROUP_LEN}
        autoComplete="off"
        spellCheck={false}
        className="pr-7"
        onChange={(e) => {
          onChange(e.target.value)
          setOpen(true)
          setActive(-1)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
      />
      <button
        type="button"
        tabIndex={-1}
        aria-label="Show groups"
        // Keep the focus in the field, so the list does not close and reopen.
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (document.activeElement === input.current) setOpen((o) => !o)
          else input.current?.focus()
        }}
        className="absolute top-1/2 right-1 grid size-6 -translate-y-1/2 place-items-center rounded text-subtle-foreground hover:text-foreground"
      >
        <ChevronDown className="size-3.5" />
      </button>
      {visible && (
        <ul
          id={listId}
          role="listbox"
          className="absolute top-full right-0 left-0 z-50 mt-1 max-h-56 animate-in overflow-y-auto rounded-lg border border-border bg-surface p-1 shadow-md"
        >
          {shown.map((name, i) => (
            <li
              key={name}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              onMouseDown={(e) => e.preventDefault()}
              onMouseEnter={() => setActive(i)}
              onClick={() => pick(name)}
              className={cn(
                'flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-[13px]',
                i === active && 'bg-surface-2',
              )}
            >
              <span className="flex-1 truncate">{name}</span>
              {name === value.trim() && <Check className="size-3.5 text-accent-text" />}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
