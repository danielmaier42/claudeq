import {Cpu} from 'lucide-react'
import {ModelSelect} from '@/components/domain/model-select'
import {RowGroup} from '@/components/layout/settings-row'
import {Field} from '@/components/ui/field'
import {Select, type SelectGroupDef, type SelectOption} from '@/components/ui/select'
import type {Pool, Provider} from '@/lib/types'
import type {FieldErrors, TaskForm} from '../form'
import {effortOptions, modelProvider, targetOptions, type TargetOption} from '../targets'
import {EditorSection, Hint, ToggleRow} from './parts'

const asOption = (o: TargetOption): SelectOption => ({
  value: o.value,
  label: (
    <span className={o.broken ? 'text-danger' : undefined}>
      {o.label}
      {o.beta && (
        <span className="ml-1.5 rounded bg-ai-soft px-1 text-[10.5px] font-semibold text-ai uppercase">
          beta
        </span>
      )}
    </span>
  ),
  description: o.note,
})

// HowSection is where and how the run happens: the provider (or pool), its
// model and reasoning effort, and what the run may do. A script job has no
// provider, model, effort or permission grant — showing those would offer
// settings the daemon refuses on save.
export function HowSection({
  form,
  errors,
  update,
  setTarget,
  providers,
  pools,
  beta,
  effort,
  providerHint,
}: {
  form: TaskForm
  errors: FieldErrors
  update: (patch: Partial<TaskForm>) => void
  setTarget: (t: string) => void
  providers: Provider[]
  pools: Pool[]
  beta: boolean
  effort: boolean
  providerHint?: string
}) {
  const script = form.kind === 'script'
  const targets = targetOptions(providers, pools, beta, form.target)
  const groups: SelectGroupDef[] = [{options: targets.providers.map(asOption)}]
  if (targets.pools.length) groups.push({label: 'Pools', options: targets.pools.map(asOption)})

  return (
    <EditorSection icon={Cpu} title="How it runs">
      {!script && (
        <div className={effort ? 'grid grid-cols-3 gap-3' : 'grid grid-cols-2 gap-3'}>
          <Field label="Provider" htmlFor="te-target" error={errors.target}>
            <Select id="te-target" value={form.target} onValueChange={setTarget} groups={groups} />
            {providerHint && !errors.target && <Hint tone="danger">{providerHint}</Hint>}
          </Field>
          <Field label="Model" htmlFor="te-model">
            <ModelSelect
              id="te-model"
              provider={modelProvider(form.target, pools)}
              value={form.model}
              onValueChange={(model) => update({model})}
              emptyLabel="Provider default"
            />
          </Field>
          {effort && (
            <Field label="Reasoning effort" htmlFor="te-effort">
              <Select
                id="te-effort"
                value={form.effort}
                onValueChange={(v) => update({effort: v})}
                options={effortOptions(form.effort)}
              />
            </Field>
          )}
        </div>
      )}
      <RowGroup>
        {!script && (
          <ToggleRow
            id="te-skip"
            label="Skip permission prompts"
            description="Needed for unattended writes."
            checked={form.skipPermissions}
            onCheckedChange={(v) => update({skipPermissions: v})}
          />
        )}
        <ToggleRow
          id="te-notify"
          label="Notify me with the result"
          description="Send outcome + last message when it finishes."
          checked={form.notify}
          onCheckedChange={(v) => update({notify: v})}
        />
      </RowGroup>
    </EditorSection>
  )
}
