import {Bot, FileCode, FileText, MessageSquareText, Terminal} from 'lucide-react'
import {Field} from '@/components/ui/field'
import {Input, Textarea} from '@/components/ui/input'
import {Segmented} from '@/components/ui/segmented'
import type {TaskKind} from '@/lib/types'
import type {FieldErrors, TaskForm} from '../form'
import {PromptReview} from '../review/PromptReview'
import {GroupCombobox} from './GroupCombobox'
import {EditorSection} from './parts'

const mono = 'font-mono text-[12.5px]'

// WhatSection names the task, files it in a group and picks the job type. The
// prompt itself has its own section further down, so the trigger (When) sits
// right below this one and is in view without scrolling.
export function WhatSection({
  form,
  errors,
  update,
  setKind,
  autoFocusName,
}: {
  form: TaskForm
  errors: FieldErrors
  update: (patch: Partial<TaskForm>) => void
  setKind: (k: TaskKind) => void
  autoFocusName: boolean
}) {
  return (
    <EditorSection icon={FileText} title="What">
      <div className="grid grid-cols-[1fr_10rem_auto] items-start gap-3">
        <Field label="Name" htmlFor="te-name" error={errors.name}>
          <Input
            id="te-name"
            value={form.name}
            placeholder="Nightly build"
            autoFocus={autoFocusName}
            autoComplete="off"
            onChange={(e) => update({name: e.target.value})}
          />
        </Field>
        <Field label="Group" htmlFor="te-group" error={errors.group}>
          <GroupCombobox
            id="te-group"
            value={form.group}
            invalid={!!errors.group}
            onChange={(group) => update({group})}
          />
        </Field>
        <Field label="Job type">
          <Segmented<TaskKind>
            tone="accent"
            aria-label="Job type"
            value={form.kind}
            onValueChange={setKind}
            options={[
              {
                value: 'agent',
                label: (
                  <>
                    <Bot />
                    Agent
                  </>
                ),
                tip: 'Claude (or another provider) works on the prompt.',
              },
              {
                value: 'script',
                label: (
                  <>
                    <Terminal />
                    Script
                  </>
                ),
                tip: 'Runs a program without a model.',
              },
            ]}
          />
        </Field>
      </div>
    </EditorSection>
  )
}

export function PromptSection({
  form,
  errors,
  update,
  freshReview,
}: {
  form: TaskForm
  errors: FieldErrors
  update: (patch: Partial<TaskForm>) => void
  freshReview: boolean
}) {
  const script = form.kind === 'script'
  return (
    <EditorSection icon={script ? FileCode : MessageSquareText} title={script ? 'Script' : 'Prompt'}>
      {script && (
        <p className="rounded-md bg-surface-2 px-3 py-2 text-[12px] leading-relaxed text-muted-foreground">
          Runs without a model: no provider, no usage, and a rate limit never holds it up. Start it with a
          shebang (<code className={mono}>#!/bin/zsh</code>,{' '}
          <code className={mono}>#!/usr/bin/env python3</code>) to pick the interpreter, or leave it out for{' '}
          <code className={mono}>/bin/sh</code>. It runs in the working directory below and can queue agent
          jobs with <code className={mono}>"$CLAUDEQ_BIN" queue --prompt …</code>.
        </p>
      )}
      <Field htmlFor="te-prompt" error={errors.prompt}>
        <Textarea
          id="te-prompt"
          aria-label={script ? 'Script' : 'Prompt'}
          value={form.prompt}
          aria-invalid={!!errors.prompt}
          spellCheck={!script}
          placeholder={script ? '#!/bin/zsh\n\ncurl -s …' : 'What should Claude do?'}
          className={`${mono} min-h-[220px] leading-relaxed`}
          onChange={(e) => update({prompt: e.target.value})}
        />
      </Field>
      <PromptReview
        kind={script ? 'script' : 'task'}
        prompt={form.prompt}
        workingDir={form.workingDir}
        fresh={freshReview}
        onApply={(revised) => update({prompt: revised})}
        className="-mt-1"
      />
    </EditorSection>
  )
}
