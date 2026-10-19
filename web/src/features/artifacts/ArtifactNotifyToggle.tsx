import {Bell, BellOff} from 'lucide-react'
import {Switch} from '@/components/ui/switch'
import {Tooltip} from '@/components/ui/tooltip'
import {commitSettings} from '@/features/settings/actions'
import {useSettings} from '@/lib/queries'

// The same switch as Settings → Notifications → Notify about new artifacts, put
// where the artifacts are: whether a new one raises a notification (on every
// channel, and in Notifications). Off, it still lands here.
export function ArtifactNotifyToggle() {
  const settings = useSettings().data
  if (!settings) return null
  const on = !settings.artifact_notify_disabled
  return (
    <Tooltip
      content={
        on
          ? 'A new artifact sends a notification. Switch off to have them arrive here quietly.'
          : 'New artifacts arrive here quietly, without a notification.'
      }
    >
      <label className="flex h-8 cursor-pointer items-center gap-2 rounded-md border border-border bg-surface px-2.5 text-[13px] font-medium shadow-xs">
        {on ? (
          <Bell className="size-4 text-muted-foreground" />
        ) : (
          <BellOff className="size-4 text-muted-foreground" />
        )}
        Notify me
        <Switch
          checked={on}
          aria-label="Notify about new artifacts"
          onCheckedChange={(v) => void commitSettings({artifact_notify_disabled: !v})}
        />
      </label>
    </Tooltip>
  )
}
