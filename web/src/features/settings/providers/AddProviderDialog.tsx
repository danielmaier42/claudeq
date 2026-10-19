import {Sparkles} from 'lucide-react'
import {useState, type FormEvent} from 'react'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
} from '@/components/ui/dialog'
import {Field} from '@/components/ui/field'
import {Input} from '@/components/ui/input'
import {Select} from '@/components/ui/select'
import {toast} from '@/components/ui/toaster'
import {errorMessage} from '@/lib/api'
import {useBeta} from '@/lib/providers'
import {useProviderKinds} from '@/lib/queries'
import type {ProviderKind} from '@/lib/types'
import {addProvider} from './actions'

// AddProviderDialog asks for the things that cannot be changed afterwards (the
// id and the kind) plus the two that decide what the new provider is. The rest
// is edited on its card once it exists.
export function AddProviderDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">{open && <AddForm onDone={() => onOpenChange(false)} />}</DialogContent>
    </Dialog>
  )
}

function AddForm({onDone}: {onDone: () => void}) {
  const beta = useBeta()
  const kinds = (useProviderKinds().data ?? []).filter((k) => !k.beta || beta)
  const [id, setId] = useState('')
  const [kind, setKind] = useState<ProviderKind | ''>('')
  const [name, setName] = useState('')
  const [dir, setDir] = useState('')
  const [idError, setIdError] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const chosen = kinds.find((k) => k.kind === kind) ?? kinds[0]

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const trimmed = id.trim()
    if (!trimmed) {
      setIdError('Give the provider an id; tasks name it by that.')
      return
    }
    if (!chosen) return
    setSaving(true)
    setError('')
    try {
      await addProvider({
        id: trimmed,
        kind: chosen.kind,
        name: name.trim() || trimmed,
        config_dir: dir.trim(),
      })
      toast.success('Provider added')
      onDone()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex min-h-0 flex-col">
      <DialogHeader
        icon={<Sparkles />}
        title="Add provider"
        description="Give it its own configuration directory to make it a second account, with its own sessions and its own rate limit."
      />
      <DialogBody className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Id" htmlFor="np-id" error={idError} hint="How tasks name it. Fixed once added.">
            <Input
              id="np-id"
              className="font-mono text-[12.5px]"
              value={id}
              placeholder="claude-work"
              aria-invalid={!!idError}
              onChange={(e) => {
                setId(e.target.value)
                setIdError('')
              }}
              spellCheck={false}
              autoFocus
            />
          </Field>
          <Field label="Type" htmlFor="np-kind" hint="Fixed once added.">
            <Select
              id="np-kind"
              value={chosen?.kind ?? ''}
              onValueChange={(v) => setKind(v as ProviderKind)}
              placeholder="Loading…"
              options={kinds.map((k) => ({
                value: k.kind,
                label: (k.name || k.kind) + (k.beta ? ' (beta)' : ''),
              }))}
            />
          </Field>
        </div>
        <Field label="Name" htmlFor="np-name" hint="Shown in the app and in run messages. Empty uses the id.">
          <Input
            id="np-name"
            value={name}
            placeholder={id.trim() || 'Claude (work)'}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <Field
          label="Configuration directory"
          htmlFor="np-dir"
          hint="Where the CLI keeps its account and sessions. Empty uses the CLI's own default, shown as the placeholder."
        >
          {/* The placeholder is the chosen harness's own location, so an empty
              field says what it falls back to rather than leaving it to be guessed. */}
          <Input
            id="np-dir"
            className="font-mono text-[12.5px]"
            value={dir}
            placeholder={chosen?.default_config_dir || "the CLI's own"}
            onChange={(e) => setDir(e.target.value)}
            spellCheck={false}
          />
        </Field>
        <p className="text-[12px] text-muted-foreground">
          Binary, default model and the fallback for a used-up limit are set on its card afterwards.
        </p>
        {error && (
          <Alert tone="danger" title="Not added">
            {error}
          </Alert>
        )}
      </DialogBody>
      <DialogFooter>
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </DialogClose>
        <Button type="submit" variant="primary" loading={saving} disabled={!chosen}>
          Add provider
        </Button>
      </DialogFooter>
    </form>
  )
}
