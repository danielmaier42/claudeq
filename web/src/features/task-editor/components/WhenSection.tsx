import {CalendarClock, CircleAlert, Clock, Repeat, Zap} from 'lucide-react'
import {Field} from '@/components/ui/field'
import {Input} from '@/components/ui/input'
import {Segmented} from '@/components/ui/segmented'
import {Tooltip} from '@/components/ui/tooltip'
import {fullTime, relTime} from '@/lib/format'
import type {TaskTrigger} from '@/lib/types'
import {cn} from '@/lib/utils'
import {DEFAULT_CRON, withTrigger, type FieldErrors, type TaskForm} from '../form'
import {useCronCheck} from '../hooks/useCronCheck'
import {EditorSection} from './parts'

const CRON_FIELDS: [string, string][] = [
  ['minute', "Minute of the hour (0–59). '*' = every minute, '*/15' = every 15 minutes, '0' = on the hour."],
  ['hour', "Hour of the day (0–23). '20' = 8 pm, '*' = every hour, '9-17' = 9 am through 5 pm."],
  ['day', "Day of the month (1–31). '*' = every day, '1' = the 1st, '1,15' = 1st and 15th."],
  ['month', "Month (1–12). '*' = every month, '6' = June only."],
  ['weekday', "Day of the week (0–6, Sunday = 0). '*' = any day, '1-5' = Mon–Fri, '0,6' = weekends."],
]

const PRESETS: [string, string][] = [
  ['Nightly 03:00', DEFAULT_CRON],
  ['Hourly', '0 * * * *'],
  ['Daily 20:00', '0 20 * * *'],
  ['Weekdays 9:00', '0 9 * * 1-5'],
  ['Mondays 8:00', '0 8 * * 1'],
]

export function WhenSection({
  form,
  errors,
  update,
  apply,
}: {
  form: TaskForm
  errors: FieldErrors
  update: (patch: Partial<TaskForm>) => void
  apply: (next: TaskForm, touched: (keyof TaskForm)[]) => void
}) {
  return (
    <EditorSection icon={Clock} title="When">
      <Segmented<TaskTrigger>
        tone="accent"
        aria-label="Trigger"
        className="self-start"
        value={form.trigger}
        onValueChange={(trigger) => apply(withTrigger(form, trigger), ['trigger', 'cron'])}
        options={[
          {
            value: 'asap',
            label: (
              <>
                <Zap />
                As soon as possible
              </>
            ),
          },
          {
            value: 'fixed',
            label: (
              <>
                <CalendarClock />
                Fixed time
              </>
            ),
          },
          {
            value: 'cron',
            label: (
              <>
                <Repeat />
                Recurring
              </>
            ),
          },
        ]}
      />
      {form.trigger === 'fixed' && (
        <Field
          label="Earliest start"
          htmlFor="te-fixedAt"
          error={errors.fixedAt}
          hint="It starts at this time or as soon after it as nothing holds it up."
        >
          <Input
            id="te-fixedAt"
            type="datetime-local"
            value={form.fixedAt}
            aria-invalid={!!errors.fixedAt}
            className="w-auto self-start"
            onChange={(e) => update({fixedAt: e.target.value})}
          />
        </Field>
      )}
      {form.trigger === 'cron' && (
        <CronField value={form.cron} error={errors.cron} onChange={(cron) => update({cron})} />
      )}
    </EditorSection>
  )
}

function CronField({value, error, onChange}: {value: string; error?: string; onChange: (v: string) => void}) {
  const verdict = useCronCheck(value, true)
  // A refusal at save time wins; otherwise the live verdict for the text as it
  // stands. An empty field is silent while typing — untouched is not an error.
  const bad =
    error ?? (verdict && !verdict.valid ? verdict.error || 'Not a valid cron expression.' : undefined)
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-2">
        <label htmlFor="te-cron" className="text-[12.5px] leading-none font-medium">
          Cron schedule
        </label>
        <div className="flex flex-wrap justify-end gap-1">
          {PRESETS.map(([label, expr]) => (
            <button
              key={expr}
              type="button"
              onClick={() => onChange(expr)}
              className={cn(
                'rounded-full border border-border px-2 py-0.5 text-[11.5px] text-muted-foreground transition-colors hover:border-border-strong hover:text-foreground focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:outline-none',
                value.trim() === expr && 'border-accent/50 bg-accent-soft text-accent-text',
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      <Input
        id="te-cron"
        value={value}
        placeholder={DEFAULT_CRON}
        spellCheck={false}
        autoComplete="off"
        aria-invalid={!!bad}
        className="font-mono text-[13px] tracking-wide"
        onChange={(e) => onChange(e.target.value)}
      />
      {bad ? (
        <p className="flex items-start gap-1.5 text-[12px] text-danger" role="alert">
          <CircleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
          {bad}
        </p>
      ) : verdict?.valid && verdict.next?.length ? (
        <div className="flex flex-wrap items-center gap-1.5 text-[12px] text-muted-foreground">
          <span>Runs next:</span>
          {verdict.next.map((t) => (
            <Tooltip key={t} content={relTime(t)}>
              <span className="rounded-md bg-success-soft px-1.5 py-0.5 text-[11.5px] text-foreground tabular-nums">
                {fullTime(t)}
              </span>
            </Tooltip>
          ))}
        </div>
      ) : null}
      <p className="text-[12px] text-muted-foreground">
        5 fields:{' '}
        {CRON_FIELDS.map(([name, tip]) => (
          <Tooltip key={name} content={tip}>
            <span
              tabIndex={0}
              className="cursor-help font-mono underline decoration-subtle-foreground decoration-dotted underline-offset-2"
            >
              {name}
            </span>
          </Tooltip>
        )).flatMap((el, i) => (i ? [' ', el] : [el]))}{' '}
        — <span className="font-mono">0 20 * * *</span> = daily at 20:00. Hover a field for details.
      </p>
    </div>
  )
}
