import {useQuery} from '@tanstack/react-query'
import {CheckCircle2, Palette, Search, Sparkles, Tag, Unlock} from 'lucide-react'
import {useEffect, useState, type ReactNode} from 'react'
import {Button} from '@/components/ui/button'
import {Checkbox} from '@/components/ui/checkbox'
import {Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader} from '@/components/ui/dialog'
import {Meter} from '@/components/ui/progress'
import {toast} from '@/components/ui/toaster'
import {errorMessage, get, post} from '@/lib/api'
import {invalidate, keys} from '@/lib/queries'
import type {RenameApplyRequest, RenameApplyResult, RenameCheck, WhatsNewStatus} from '@/lib/types'

// WhatsNewDialog introduces DispatchQ 2.0 once, on the first start after the
// update from ClaudeQ. From it the operator can have the prompt reviewer check
// the queue for tasks that still name ClaudeQ and apply the fixes they pick.
export function WhatsNewDialog() {
  const status = useQuery({queryKey: ['whats-new'], queryFn: () => get<WhatsNewStatus>('/api/whats-new')})
  const [closed, setClosed] = useState(false)
  const open = !!status.data?.show && !closed

  const close = async () => {
    setClosed(true)
    try {
      await post('/api/whats-new/dismiss')
    } catch (e) {
      toast.error('Could not save that you have seen this', {description: errorMessage(e)})
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && void close()}>
      <DialogContent size="lg">
        <DialogHeader
          icon={<Sparkles />}
          title="ClaudeQ is now DispatchQ"
          description={`What’s new in DispatchQ ${status.data?.edition ?? ''}`}
        />
        <DialogBody className="space-y-4 text-[13px] leading-relaxed">
          <Item icon={<Tag />} title="A new name">
            ClaudeQ is now DispatchQ, with a new icon. Your tasks, run history and settings moved over as they
            were. The command line tool is now <code>dispatchq</code>, in{' '}
            <code>/Applications/DispatchQ.app/Contents/MacOS</code>.
          </Item>
          <Item icon={<Palette />} title="A complete redesign">
            The app was rebuilt from the ground up: a Dashboard overview, a command palette (⌘K), shortcuts
            for every view (⌘1 to ⌘8), a slide-in task panel, a new log view and artifact viewer, and an
            accent colour that follows macOS or can be pinned.
          </Item>
          <Item icon={<Unlock />} title="Every beta feature is public">
            Codex, opencode, adding your own providers and provider pools are no longer behind the beta
            switch. They are available to everyone, and the switch is gone.
          </Item>
          <RenameCheckPanel />
        </DialogBody>
        <DialogFooter>
          <Button variant="primary" onClick={() => void close()}>
            Got it
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Item({icon, title, children}: {icon: ReactNode; title: string; children: ReactNode}) {
  return (
    <div className="flex gap-3">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent-text [&_svg]:size-3.5">
        {icon}
      </div>
      <div>
        <div className="font-medium">{title}</div>
        <p className="text-muted-foreground">{children}</p>
      </div>
    </div>
  )
}

// RenameCheckPanel runs the check and lists what it found, each fix selected
// until the operator deselects it.
function RenameCheckPanel() {
  const [check, setCheck] = useState<RenameCheck | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [applying, setApplying] = useState(false)

  const running = check?.state === 'running'
  useEffect(() => {
    if (!running) return
    const t = setInterval(async () => {
      try {
        const c = await get<RenameCheck>('/api/rename-check')
        setCheck(c)
        if (c.state === 'done')
          setSelected(new Set(c.findings.filter((f) => f.revised_prompt).map((f) => f.task_id)))
      } catch {
        /* the next poll tries again */
      }
    }, 1000)
    return () => clearInterval(t)
  }, [running])

  const start = async () => {
    try {
      setCheck(await post<RenameCheck>('/api/rename-check'))
    } catch (e) {
      toast.error('Could not start the check', {description: errorMessage(e)})
    }
  }

  const apply = async () => {
    setApplying(true)
    try {
      const r = await post<RenameApplyResult>('/api/rename-check/apply', {
        task_ids: [...selected],
      } satisfies RenameApplyRequest)
      invalidate(keys.tasks)
      setCheck(await get<RenameCheck>('/api/rename-check'))
      setSelected(new Set())
      if (r.changed?.length)
        toast(`${r.applied.length} migrated, ${r.changed.length} skipped`, {
          description: 'Skipped tasks were edited after the check. Check them again.',
        })
      else toast.success(`${r.applied.length} ${r.applied.length === 1 ? 'task' : 'tasks'} migrated`)
    } catch (e) {
      toast.error('Could not migrate the tasks', {description: errorMessage(e)})
    } finally {
      setApplying(false)
    }
  }

  const toggle = (id: string, on: boolean) =>
    setSelected((s) => {
      const n = new Set(s)
      if (on) n.add(id)
      else n.delete(id)
      return n
    })

  return (
    <div className="rounded-lg border border-border bg-surface-2/60 p-3.5">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="font-medium">Check your tasks for the new name</div>
          <p className="text-muted-foreground">
            A task that calls ClaudeQ.app or uses CLAUDEQ_ variables should move to the new names. The prompt
            reviewer checks every task that mentions ClaudeQ; you choose which fixes to apply.
          </p>
        </div>
        {(!check || check.state === 'idle' || check.state === 'unavailable') && (
          <Button onClick={() => void start()}>
            <Search />
            Check tasks
          </Button>
        )}
      </div>

      {check?.state === 'unavailable' && (
        <p className="mt-3 text-muted-foreground">
          No prompt reviewer is available. Choose one in Settings, then check again.
        </p>
      )}

      {running && (
        <div className="mt-3 space-y-1.5">
          <Meter value={check.checked} max={check.total} />
          <p className="text-[12px] text-muted-foreground">
            Checked {check.checked} of {check.total}
          </p>
        </div>
      )}

      {check?.state === 'done' && (
        <div className="mt-3 space-y-2">
          {check.findings.length === 0 ? (
            <p className="flex items-center gap-1.5 text-muted-foreground">
              <CheckCircle2 className="size-4 text-success" />
              {check.total === 0
                ? 'No task mentions ClaudeQ. Nothing to migrate.'
                : 'Nothing left to migrate.'}
            </p>
          ) : (
            <>
              <ul className="space-y-2">
                {check.findings.map((f) => (
                  <li key={f.task_id} className="flex items-start gap-2.5">
                    <Checkbox
                      className="mt-0.5"
                      checked={selected.has(f.task_id)}
                      disabled={!f.revised_prompt}
                      onCheckedChange={(v) => toggle(f.task_id, v === true)}
                      aria-label={f.name}
                    />
                    <div className="min-w-0">
                      <div className="font-medium">{f.name}</div>
                      <p className="text-[12px] text-muted-foreground">
                        {f.message}
                        {!f.revised_prompt && ' Edit this task yourself.'}
                      </p>
                    </div>
                  </li>
                ))}
              </ul>
              <div className="flex justify-end">
                <Button
                  variant="primary"
                  loading={applying}
                  disabled={selected.size === 0}
                  onClick={() => void apply()}
                >
                  Migrate {selected.size} {selected.size === 1 ? 'task' : 'tasks'}
                </Button>
              </div>
            </>
          )}
          {!!check.errors?.length && (
            <p className="text-[12px] text-danger">Could not check: {check.errors.join('; ')}</p>
          )}
        </div>
      )}
    </div>
  )
}
