import {Check, Copy} from 'lucide-react'
import {useEffect, useState} from 'react'
import {Button} from '@/components/ui/button'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {cn} from '@/lib/utils'

// CopyButton copies a text and shows a check for a moment, so the click is
// confirmed where the user looked.
export function CopyButton({
  text,
  label = 'Copy',
  className,
  children,
}: {
  text: string | (() => string)
  label?: string
  className?: string
  children?: string
}) {
  const [done, setDone] = useState(false)
  useEffect(() => {
    if (!done) return
    const t = setTimeout(() => setDone(false), 1400)
    return () => clearTimeout(t)
  }, [done])

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(typeof text === 'function' ? text() : text)
      setDone(true)
    } catch {
      toast.error('Could not copy to the clipboard')
    }
  }
  const icon = done ? <Check className="text-success" /> : <Copy />
  if (children)
    return (
      <Button variant="ghost" size="sm" className={className} onClick={copy}>
        {icon}
        {done ? 'Copied' : children}
      </Button>
    )
  return (
    <Tooltip content={done ? 'Copied' : label}>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={label}
        className={cn('bg-surface/80', className)}
        onClick={copy}
      >
        {icon}
      </Button>
    </Tooltip>
  )
}
