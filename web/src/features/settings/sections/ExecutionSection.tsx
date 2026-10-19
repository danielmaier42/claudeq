import {Row, RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {Badge} from '@/components/ui/badge'
import {Select} from '@/components/ui/select'
import {Switch} from '@/components/ui/switch'
import type {Settings} from '@/lib/types'
import {commitSettings, setPaused} from '../actions'
import {SaveMode} from '../components/parts'
import {HEARTBEAT, HISTORY, IDLE, choiceOptions, heartbeatValue, historyValue, idleValue} from '../options'

export function ExecutionSection({value}: {value: Settings}) {
  const hb = heartbeatValue(value.heartbeat_minutes)
  const idle = idleValue(value.idle_timeout_minutes)
  const hist = historyValue(value.max_run_history)

  return (
    <>
      <Section title="Global pause" actions={<SaveMode mode="instant" />}>
        <RowGroup className={value.paused ? 'border-warning/40' : undefined}>
          <Row
            htmlFor="s-paused"
            label={
              <span className="flex items-center gap-2">
                Pause all runs
                {value.paused && (
                  <Badge tone="warning" dot>
                    Paused
                  </Badge>
                )}
              </span>
            }
            description={
              <>
                No task starts while this is on, not even <strong className="font-medium">Run now</strong>. A
                run already in flight keeps going, and a task that came due meanwhile runs as soon as you
                resume.
              </>
            }
            control={
              <Switch id="s-paused" checked={value.paused} onCheckedChange={(on) => void setPaused(on)} />
            }
          />
        </RowGroup>
      </Section>

      <Section title="Runs" actions={<SaveMode mode="instant" />}>
        <RowGroup>
          <Row
            htmlFor="s-idle"
            label="Stop a run with no output for"
            description="Kills a hung run; a working run keeps streaming, so it is unaffected"
            control={
              <Select
                id="s-idle"
                className="w-44"
                value={String(idle)}
                options={choiceOptions(IDLE, idle, (m) => `${m} minutes`)}
                onValueChange={(v) => void commitSettings({idle_timeout_minutes: Number(v)})}
              />
            }
          />
          <Row
            htmlFor="s-hist"
            label="Keep run history"
            description="Older runs and their logs are pruned"
            control={
              <Select
                id="s-hist"
                className="w-44"
                value={String(hist)}
                options={choiceOptions(HISTORY, hist, (n) => `${n} runs`)}
                onValueChange={(v) => void commitSettings({max_run_history: Number(v)})}
              />
            }
          />
        </RowGroup>
      </Section>

      <Section title="Scheduler" actions={<SaveMode mode="instant" />}>
        <RowGroup>
          <Row
            htmlFor="s-hb"
            label="Check for due tasks every"
            description="How often the daemon wakes to look for work, as a safety net next to the planned wake-ups"
            control={
              <Select
                id="s-hb"
                className="w-44"
                value={String(hb)}
                options={choiceOptions(HEARTBEAT, hb, (m) => `Every ${m} minutes`)}
                onValueChange={(v) => void commitSettings({heartbeat_minutes: Number(v)})}
              />
            }
          />
        </RowGroup>
      </Section>
    </>
  )
}
