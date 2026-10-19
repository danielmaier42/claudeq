import {Eye, EyeOff, KeyRound} from 'lucide-react'
import {useState} from 'react'
import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import {Tooltip} from '@/components/ui/tooltip'

// SecretInput never shows a stored credential: it says that one is set and
// offers to replace or remove it. A new value is typed masked, with an eye to
// check it before saving.
export function SecretInput({
  id,
  stored,
  value,
  onChange,
  placeholder,
  invalid,
}: {
  id: string
  stored: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  invalid?: boolean
}) {
  const [replacing, setReplacing] = useState(false)
  const [shown, setShown] = useState(false)
  const untouched = value === stored

  if (stored && untouched && !replacing) {
    return (
      <div className="flex h-8 items-center gap-2">
        <div className="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md border border-border bg-surface-2 px-2.5 text-[13px] text-muted-foreground">
          <KeyRound className="size-3.5 shrink-0 text-success" />
          <span className="font-medium text-foreground">Set</span>
          <span className="truncate tracking-widest text-subtle-foreground">••••••••</span>
        </div>
        <Button size="sm" onClick={() => setReplacing(true)}>
          Replace
        </Button>
        <Button size="sm" variant="danger-ghost" onClick={() => onChange('')}>
          Remove
        </Button>
      </div>
    )
  }

  const cancel = () => {
    setReplacing(false)
    setShown(false)
    onChange(stored)
  }

  return (
    <div className="flex items-center gap-2">
      <div className="relative min-w-0 flex-1">
        <Input
          id={id}
          type={shown ? 'text' : 'password'}
          autoComplete="off"
          spellCheck={false}
          autoFocus={replacing}
          value={untouched && replacing ? '' : value}
          placeholder={stored && !value ? 'Removed when you save' : placeholder}
          aria-invalid={invalid || undefined}
          onChange={(e) => onChange(e.target.value)}
          className="pr-8 font-mono"
        />
        <Tooltip content={shown ? 'Hide' : 'Show'}>
          <button
            type="button"
            aria-label={shown ? 'Hide value' : 'Show value'}
            onClick={() => setShown((v) => !v)}
            className="absolute top-1/2 right-1.5 flex size-6 -translate-y-1/2 items-center justify-center rounded text-subtle-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40"
          >
            {shown ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
          </button>
        </Tooltip>
      </div>
      {stored && (
        <Button size="sm" variant="ghost" onClick={cancel}>
          Keep current
        </Button>
      )}
    </div>
  )
}
