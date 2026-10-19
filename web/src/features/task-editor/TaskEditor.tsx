import {useCallback, useEffect, useRef, useState} from 'react'
import {confirm} from '@/components/ui/confirm'
import {Sheet, SheetBody, SheetContent, SheetHeader} from '@/components/ui/sheet'
import {Skeleton} from '@/components/ui/skeleton'
import {EditorForm} from './components/EditorForm'
import {resolveSession, type EditorSession} from './session'
import {closeTaskEditor, useTaskEditor, type TaskEditorRequest} from './store'

// TaskEditor is the one sheet that adds, edits, replays or imports a task. The
// shell mounts it once; openTaskEditor() decides what it shows.
export function TaskEditor() {
  const request = useTaskEditor((s) => s.request)
  const [resolved, setResolved] = useState<{
    req: TaskEditorRequest
    session: EditorSession
    id: number
  } | null>(null)
  const dirty = useRef(false)
  const opened = useRef(0)

  useEffect(() => {
    if (!request) return
    let live = true
    void resolveSession(request).then((session) => {
      if (!live) return
      // A fresh form for every opening, so nothing typed into the last one leaks in.
      if (session) setResolved({req: request, session, id: ++opened.current})
      else closeTaskEditor()
    })
    return () => {
      live = false
    }
  }, [request])

  const current = resolved && resolved.req === request ? resolved : null
  const onDirty = useCallback((d: boolean) => {
    dirty.current = d
  }, [])

  // Leaving with unsaved changes asks first — Esc, a click outside, the close
  // button and Cancel all come through here.
  const requestClose = async () => {
    if (
      dirty.current &&
      !(await confirm({
        title: 'Discard changes?',
        description: 'What you changed in this task has not been saved.',
        confirmLabel: 'Discard',
        destructive: true,
      }))
    )
      return
    dirty.current = false
    closeTaskEditor()
  }

  return (
    <Sheet open={!!request} onOpenChange={(open) => !open && void requestClose()}>
      <SheetContent
        width="lg"
        aria-busy={!current}
        onEscapeKeyDown={(e) => {
          e.preventDefault()
          void requestClose()
        }}
        onPointerDownOutside={(e) => {
          e.preventDefault()
          // A click on a toast (Apply's "Prompt rewritten") is not a wish to leave.
          if ((e.target as Element | null)?.closest('[data-sonner-toaster]')) return
          void requestClose()
        }}
        // Only a new task starts in the name field (autoFocus there); an
        // existing one opens without a field grabbing the caret.
        onOpenAutoFocus={(e) => {
          e.preventDefault()
          ;(e.currentTarget as HTMLElement | null)?.focus()
        }}
      >
        {current ? <EditorForm key={current.id} session={current.session} onDirty={onDirty} /> : <Loading />}
      </SheetContent>
    </Sheet>
  )
}

function Loading() {
  return (
    <>
      <SheetHeader title="Loading task…" />
      <SheetBody className="flex flex-col gap-4 px-6 py-5">
        <Skeleton className="h-8 w-2/3" />
        <Skeleton className="h-[220px]" />
        <Skeleton className="h-8" />
        <Skeleton className="h-7 w-1/2" />
        <Skeleton className="h-24" />
      </SheetBody>
    </>
  )
}
