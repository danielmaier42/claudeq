import {Badge} from '@/components/ui/badge'
import {Tooltip} from '@/components/ui/tooltip'
import type {Run} from '@/lib/types'
import {statusLabel, statusTone} from '../runs'

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

// The run's outcome; a run that failed explains why on hover.
export function StatusPill({run, className}: {run: Run; className?: string}) {
  const running = run.status === 'running'
  return (
    <Tooltip content={run.error}>
      <Badge tone={statusTone[run.status]} dot pulse={running} className={className}>
        {cap(statusLabel(run))}
      </Badge>
    </Tooltip>
  )
}
