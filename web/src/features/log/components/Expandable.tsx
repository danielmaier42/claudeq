import {useState, type ReactNode} from 'react'
import {cn} from '@/lib/utils'

// Expandable shows a long block clipped to a few lines with a fade and a
// "Show all" toggle, so one huge prompt or note does not push the
// conversation off the screen.
export function Expandable({
  long,
  children,
  className,
}: {
  long: boolean
  children: ReactNode
  className?: string
}) {
  const [open, setOpen] = useState(false)
  if (!long) return <>{children}</>
  return (
    <div className={className}>
      <div
        className={cn(
          'relative',
          !open && 'max-h-56 overflow-hidden [mask-image:linear-gradient(to_bottom,black_70%,transparent)]',
        )}
      >
        {children}
      </div>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="mt-1 rounded-sm text-[12px] font-medium text-accent-text outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent/40"
      >
        {open ? 'Show less' : 'Show all'}
      </button>
    </div>
  )
}

export const isLong = (text: string) => text.length > 1400 || text.split('\n').length > 16
