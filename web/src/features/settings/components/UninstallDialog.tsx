import {Trash2} from 'lucide-react'
import {useState} from 'react'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader} from '@/components/ui/dialog'
import {Field} from '@/components/ui/field'
import {Input} from '@/components/ui/input'
import {toast} from '@/components/ui/toaster'
import {errorMessage, post} from '@/lib/api'
import type {UninstallRequest} from '@/lib/types'

const WORD = 'uninstall'

// The uninstaller is a package the macOS Installer runs: it asks once more,
// offers the data under Customize, and closes this window when it removes the
// app. Removing the app is still the one thing here that cannot be undone from
// ClaudeQ, so the word has to be typed before it is opened.
export function UninstallDialog({open, onOpenChange}: {open: boolean; onOpenChange: (o: boolean) => void}) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const ok = typed.trim().toLowerCase() === WORD

  const change = (o: boolean) => {
    if (busy) return
    if (!o) {
      setTyped('')
      setError('')
    }
    onOpenChange(o)
  }

  const run = async () => {
    if (!ok) return
    setBusy(true)
    setError('')
    try {
      await post<undefined>('/api/uninstall', {} satisfies UninstallRequest)
      toast.success('The uninstaller is open', {description: 'Follow the macOS Installer to finish.'})
      setBusy(false)
      change(false)
    } catch (e) {
      setError(errorMessage(e))
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent size="sm">
        <DialogHeader
          icon={<Trash2 className="text-danger" />}
          title="Uninstall ClaudeQ?"
          description="Opens the uninstaller in the macOS Installer."
          className="[&>div:first-child]:bg-danger-soft"
        />
        <DialogBody className="space-y-4">
          <ul className="list-disc space-y-1 pl-5 text-[13px] text-foreground/85">
            <li>Deletes the app and stops its background service</li>
            <li>Removes the permission to wake this Mac</li>
            <li>Keeps your tasks and run history unless you select them under Customize</li>
          </ul>
          <Field label={`Type “${WORD}” to continue`} htmlFor="s-uninstall-confirm">
            <Input
              id="s-uninstall-confirm"
              autoFocus
              autoComplete="off"
              spellCheck={false}
              value={typed}
              placeholder={WORD}
              onChange={(e) => setTyped(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void run()
              }}
            />
          </Field>
          {error && (
            <Alert tone="danger" title="The uninstaller could not be opened">
              {error}
            </Alert>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => change(false)} disabled={busy}>
            Cancel
          </Button>
          <Button variant="danger" disabled={!ok} loading={busy} onClick={() => void run()}>
            Open uninstaller
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
