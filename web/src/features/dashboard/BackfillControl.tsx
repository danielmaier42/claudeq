import {Slider} from '@/components/ui/slider'
import {Spinner} from '@/components/ui/spinner'
import {BACKFILL_MAX, BACKFILL_MIN} from './limits'

// The backfill threshold is a slider next to the providers it lights up. It
// applies on release, without Save; while it is held the cards preview which
// providers would qualify, and the poll cannot pull it out from under the
// pointer because the draft wins over what the daemon reports.
export function BackfillControl({
  value,
  saving,
  onDraft,
  onCommit,
}: {
  value: number
  saving: boolean
  onDraft: (v: number) => void
  onCommit: (v: number) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-3 px-4 py-3.5">
      <div className="min-w-[220px] flex-1">
        <div className="text-[13px] font-medium">Backfill threshold</div>
        <p className="mt-0.5 text-[12.5px] text-muted-foreground">
          Backfill tasks run on a provider whose urgency is above this. Urgency 1 means spending evenly from
          now on uses up the week; 2 means half of what is left would expire unused.
        </p>
      </div>
      <div className="flex w-full max-w-[280px] items-center gap-3">
        <span className="text-[11px] text-subtle-foreground tabular-nums">{BACKFILL_MIN}</span>
        <Slider
          aria-label="Backfill threshold"
          min={BACKFILL_MIN}
          max={BACKFILL_MAX}
          step={0.1}
          value={[value]}
          onValueChange={([v]) => onDraft(v)}
          onValueCommit={([v]) => onCommit(v)}
        />
        <span className="text-[11px] text-subtle-foreground tabular-nums">{BACKFILL_MAX}</span>
        <span className="flex w-12 items-center justify-end gap-1 text-[14px] font-semibold tabular-nums">
          {saving && <Spinner className="size-3.5" />}
          {value.toFixed(1)}
        </span>
      </div>
    </div>
  )
}
