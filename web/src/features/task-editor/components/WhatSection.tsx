import {Bot, FileText, Terminal} from 'lucide-react'
import {Field} from '@/components/ui/field'
import {Input, Textarea} from '@/components/ui/input'
import {Segmented} from '@/components/ui/segmented'
import type {TaskKind} from '@/lib/types'
import type {FieldErrors, TaskForm} from '../form'
import {PromptReview} from '../review/PromptReview'
import {EditorSection} from './parts'

const mono = 'font-mono text-[12.5px]'

export function WhatSection({
  form,
  errors,
  update,
  setKind,
  autoFocusName,
  freshReview,
}: {
  form: TaskForm
  errors: FieldErrors
  update: (patch: Partial<TaskForm>) => void
  setKind: (k: TaskKind) => void
  autoFocusName: boolean
  freshReview: boolean
}) {
  const script = form.kind === 'script'
  return (
    <EditorSection icon={FileText} title="What">
      <div className="grid grid-cols-[1fr_auto] items-end gap-3">
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
      {script && (
        <p className="rounded-md bg-surface-2 px-3 py-2 text-[12px] leading-relaxed text-muted-foreground">
          Runs without a model: no provider, no usage, and a rate limit never holds it up. Start it with a
          shebang (<code className={mono}>#!/bin/zsh</code>,{' '}
          <code className={mono}>#!/usr/bin/env python3</code>) to pick the interpreter, or leave it out for{' '}
          <code className={mono}>/bin/sh</code>. It runs in the working directory below and can queue agent
          jobs with <code className={mono}>"$CLAUDEQ_BIN" queue --prompt …</code>.
        </p>
      )}
      <Field label={script ? 'Script' : 'Prompt'} htmlFor="te-prompt" error={errors.prompt}>
        <Textarea
          id="te-prompt"
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
