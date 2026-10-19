import {useEffect, useMemo, useRef, useState, type KeyboardEvent} from 'react'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {Kbd} from '@/components/ui/kbd'
import {Separator} from '@/components/ui/separator'
import {SheetBody, SheetClose, SheetFooter, SheetHeader} from '@/components/ui/sheet'
import {toast} from '@/components/ui/toaster'
import {api, errorMessage} from '@/lib/api'
import {useBeta} from '@/lib/providers'
import {invalidate, keys, usePools, useProviders} from '@/lib/queries'
import type {Task, TaskKind} from '@/lib/types'
import {
  buildPayload,
  fieldForError,
  validate,
  withKind,
  withTarget,
  type FieldErrors,
  type FieldKey,
  type TaskForm,
} from '../form'
import {checkCron} from '../hooks/useCronCheck'
import type {EditorSession} from '../session'
import {closeTaskEditor} from '../store'
import {takesEffort} from '../targets'
import {AdvancedSection} from './AdvancedSection'
import {HowSection} from './HowSection'
import {WhatSection} from './WhatSection'
import {WhenSection} from './WhenSection'
import {WhereSection} from './WhereSection'

const FIELD_ORDER: FieldKey[] = ['name', 'prompt', 'workingDir', 'fixedAt', 'cron', 'target']

// The reason stays on the field itself; the field is scrolled into view and
// focused, rather than the reason repeated at the footer.
function focusField(k: FieldKey) {
  const el = document.getElementById('te-' + k)
  el?.scrollIntoView({block: 'center', behavior: 'smooth'})
  el?.focus({preventScroll: true})
}

export function EditorForm({session, onDirty}: {session: EditorSession; onDirty: (dirty: boolean) => void}) {
  const [form, setForm] = useState(session.initial)
  const [errors, setErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [dirHint, setDirHint] = useState(session.dirHint)
  const [providerHint, setProviderHint] = useState(session.providerHint)
  const providers = useProviders().data ?? []
  const pools = usePools().data ?? []
  const beta = useBeta()
  const effort = takesEffort(form.target, providers, pools)

  const dirty = useMemo(
    () => JSON.stringify(form) !== JSON.stringify(session.initial),
    [form, session.initial],
  )
  useEffect(() => onDirty(dirty && !saving), [dirty, saving, onDirty])

  // Each change clears the complaint about what was changed, so a fixed field
  // never keeps an error it no longer has.
  const apply = (next: TaskForm, touched: (keyof TaskForm)[]) => {
    setForm(next)
    setErrors((e) => {
      const out = {...e}
      for (const k of touched) delete out[k as FieldKey]
      return out
    })
    setFormError('')
  }
  const update = (patch: Partial<TaskForm>) =>
    apply({...form, ...patch}, Object.keys(patch) as (keyof TaskForm)[])
  const setKind = (k: TaskKind) => apply(withKind(form, k), ['kind'])
  const setTarget = (t: string) => {
    apply(withTarget(form, t), ['target'])
    setProviderHint(undefined)
  }
  const setDir = (dir: string) => {
    update({workingDir: dir})
    if (dir.trim()) setDirHint(undefined)
  }

  // inFlight closes the gap between the first ⌘↵ and setSaving: a second
  // submit during the cron check would otherwise add the task twice.
  const inFlight = useRef(false)
  const submit = async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      await save()
    } finally {
      inFlight.current = false
    }
  }
  const save = async () => {
    const errs = validate(form)
    // A rejected schedule is refused here, with the daemon's reason on the field.
    if (form.trigger === 'cron') {
      const v = await checkCron(form.cron)
      if (v && !v.valid) errs.cron = v.error || 'Not a valid cron expression.'
    }
    setErrors(errs)
    setFormError('')
    const first = FIELD_ORDER.find((k) => errs[k])
    if (first) return focusField(first)

    setSaving(true)
    const body = buildPayload(form, {takesEffort: effort, carried: session.carried})
    try {
      if (session.editId) {
        await api<Task>('PUT', '/api/tasks/' + encodeURIComponent(session.editId), body)
        toast.success('Task updated')
      } else {
        await api<Task>('POST', '/api/tasks', body)
        toast.success(session.mode === 'replay' ? 'Task scheduled again' : 'Task added')
      }
      void invalidate(keys.tasks, keys.groups, keys.runs)
      onDirty(false)
      closeTaskEditor()
    } catch (e) {
      const msg = errorMessage(e)
      const field = fieldForError(msg)
      if (field) {
        setErrors({[field]: msg})
        focusField(field)
      } else setFormError(msg)
    } finally {
      setSaving(false)
    }
  }

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      void submit()
    }
  }

  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      noValidate
      onKeyDown={onKeyDown}
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <SheetHeader title={session.title} description={session.description} />
      <SheetBody className="flex flex-col gap-7 px-6 py-5">
        <WhatSection
          form={form}
          errors={errors}
          update={update}
          setKind={setKind}
          autoFocusName={session.mode === 'add'}
          freshReview={session.fresh}
        />
        <Separator />
        <WhereSection value={form.workingDir} error={errors.workingDir} hint={dirHint} onChange={setDir} />
        <Separator />
        <WhenSection form={form} errors={errors} update={update} />
        <Separator />
        <HowSection
          form={form}
          errors={errors}
          update={update}
          setTarget={setTarget}
          providers={providers}
          pools={pools}
          beta={beta}
          effort={effort}
          providerHint={providerHint}
        />
        <AdvancedSection form={form} update={update} />
      </SheetBody>
      {formError && (
        <div className="shrink-0 border-t border-border px-6 py-3">
          <Alert tone="danger" title="Could not save the task">
            {formError}
          </Alert>
        </div>
      )}
      <SheetFooter className="px-6">
        <span className="mr-auto flex items-center gap-1 text-[12px] text-subtle-foreground">
          <Kbd>⌘</Kbd>
          <Kbd>↵</Kbd>
          <span className="ml-1">to {session.submitLabel.toLowerCase()}</span>
        </span>
        <SheetClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </SheetClose>
        <Button type="submit" variant="primary" loading={saving}>
          {session.submitLabel}
        </Button>
      </SheetFooter>
    </form>
  )
}
