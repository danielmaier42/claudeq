import type {LucideIcon} from 'lucide-react'
import type {ReactNode} from 'react'
import {Row} from '@/components/layout/settings-row'
import {Switch} from '@/components/ui/switch'
import {cn} from '@/lib/utils'

// EditorSection is one step of the editor (What, Where, When, How it runs): a
// small heading with an icon, then its fields.
export function EditorSection({
  icon: Icon,
  title,
  aside,
  className,
  children,
}: {
  icon: LucideIcon
  title: string
  aside?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <section className={cn('flex flex-col gap-4', className)} aria-label={title}>
      <div className="flex items-center gap-2">
        <span className="grid size-6 place-items-center rounded-md bg-accent-soft text-accent-text">
          <Icon className="size-3.5" aria-hidden />
        </span>
        <h3 className="text-[12px] font-semibold tracking-wide text-muted-foreground uppercase">{title}</h3>
        {aside && <div className="ml-auto">{aside}</div>}
      </div>
      {children}
    </section>
  )
}

export function ToggleRow({
  id,
  label,
  description,
  checked,
  onCheckedChange,
}: {
  id: string
  label: ReactNode
  description: ReactNode
  checked: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <Row
      htmlFor={id}
      label={label}
      description={description}
      control={<Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />}
    />
  )
}

// Hint is a line under a field that matters more than a plain hint: an import's
// missing folder, or a provider nobody here matches.
export function Hint({tone, children}: {tone: 'warning' | 'danger'; children: ReactNode}) {
  return (
    <p
      className={cn(
        'text-[12px]',
        tone === 'danger' ? 'text-danger' : 'text-[color-mix(in_oklab,var(--warning)_75%,var(--foreground))]',
      )}
    >
      {children}
    </p>
  )
}
