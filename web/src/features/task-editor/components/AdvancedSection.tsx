import {ChevronRight, SlidersHorizontal} from 'lucide-react'
import {Collapsible} from 'radix-ui'
import {useState} from 'react'
import {RowGroup} from '@/components/layout/settings-row'
import {Badge} from '@/components/ui/badge'
import type {TaskForm} from '../form'
import {ToggleRow} from './parts'

// AdvancedSection holds the switches most tasks leave off. It starts open when
// any of them is on, so an edited task never hides how it behaves.
export function AdvancedSection({
  form,
  update,
}: {
  form: TaskForm
  update: (patch: Partial<TaskForm>) => void
}) {
  const script = form.kind === 'script'
  const on = [form.parallel, form.quietHistory, !script && form.backfill].filter(Boolean).length
  const [open, setOpen] = useState(on > 0)
  return (
    <Collapsible.Root open={open} onOpenChange={setOpen} className="flex flex-col gap-3">
      <Collapsible.Trigger className="group flex items-center gap-2 self-start rounded-md py-0.5 pr-1.5 outline-none focus-visible:ring-2 focus-visible:ring-accent/40">
        <span className="grid size-6 place-items-center rounded-md bg-surface-3 text-muted-foreground">
          <SlidersHorizontal className="size-3.5" aria-hidden />
        </span>
        <span className="text-[12px] font-semibold tracking-wide text-muted-foreground uppercase group-hover:text-foreground">
          Advanced
        </span>
        <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-data-[state=open]:rotate-90" />
        {!open && on > 0 && <Badge tone="accent">{on} on</Badge>}
      </Collapsible.Trigger>
      <Collapsible.Content>
        <RowGroup>
          <ToggleRow
            id="te-parallel"
            label="Run in parallel"
            description="May run alongside other parallel tasks."
            checked={form.parallel}
            onCheckedChange={(v) => update({parallel: v})}
          />
          <ToggleRow
            id="te-quiet"
            label="Quiet history"
            description="Drop successful runs from the Log; failures are kept. For frequent watcher jobs."
            checked={form.quietHistory}
            onCheckedChange={(v) => update({quietHistory: v})}
          />
          {!script && (
            <ToggleRow
              id="te-backfill"
              label="Backfill"
              description="Runs only while its provider's weekly allowance would otherwise go unused: urgency above the threshold on the Dashboard. On a pool, only such members take it."
              checked={form.backfill}
              onCheckedChange={(v) => update({backfill: v})}
            />
          )}
        </RowGroup>
      </Collapsible.Content>
    </Collapsible.Root>
  )
}
