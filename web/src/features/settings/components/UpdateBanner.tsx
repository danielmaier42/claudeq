import {ArrowUpCircle, Download, ExternalLink, RotateCw} from 'lucide-react'
import {useState, type ReactNode} from 'react'
import {Button} from '@/components/ui/button'
import {openExternal} from '@/lib/native'
import type {UpdateStatus} from '@/lib/types'
import {dismissUpdate, downloadUpdate, finishUpdate, httpsOnly} from '../update'

// UpdateBanner offers a newer ClaudeQ with the notes of every version since
// the running one, or, when the installer already replaced the app but the
// background service still runs the old build, the hand-over instead of
// another download of a version that is already on disk.
export function UpdateBanner({update}: {update: UpdateStatus}) {
  const [busy, setBusy] = useState<'' | 'download' | 'dismiss' | 'relaunch'>('')

  const act = (kind: typeof busy, fn: () => Promise<unknown>) => async () => {
    setBusy(kind)
    try {
      await fn()
    } finally {
      setBusy('')
    }
  }

  if (update.restart_required) {
    return (
      <Shell title={`ClaudeQ ${update.installed} is installed, but not running`}>
        <p>
          The background service is still on {update.current || '?'}. Finish the update to switch it over;
          your queue and running tasks are untouched.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button variant="primary" loading={busy === 'relaunch'} onClick={act('relaunch', finishUpdate)}>
            {busy !== 'relaunch' && <RotateCw />}
            {busy === 'relaunch' ? 'Switching over…' : 'Finish update'}
          </Button>
        </div>
      </Shell>
    )
  }

  const notes = (update.notes ?? '').trim()
  const skipped = update.skipped_count ?? 0
  const all = httpsOnly(update.all_releases_url)
  const page = httpsOnly(update.html_url)
  const downloading = busy === 'download' || update.downloading

  return (
    <Shell title={`Update available: ClaudeQ ${update.latest}`}>
      <p>
        {skipped > 1
          ? `You’re on ${update.current || '?'}; ${skipped} newer versions are available. Their changes are below.`
          : `You’re on ${update.current || '?'}. Download the installer and follow the prompts to update.`}
      </p>
      {notes && (
        <div className="mt-3 max-h-48 overflow-auto rounded-md border border-border bg-surface px-3 py-2.5 text-[12.5px] leading-relaxed break-words whitespace-pre-wrap text-foreground/85">
          {notes}
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        {update.has_installer || !page ? (
          <Button variant="primary" loading={downloading} onClick={act('download', downloadUpdate)}>
            {!downloading && <Download />}
            {downloading ? 'Downloading…' : 'Download & install'}
          </Button>
        ) : (
          <Button variant="primary" onClick={() => openExternal(page)}>
            Open release page
            <ExternalLink className="size-3.5" />
          </Button>
        )}
        <Button loading={busy === 'dismiss'} onClick={act('dismiss', () => dismissUpdate(update.latest))}>
          Dismiss
        </Button>
        {all && (
          <Button variant="ghost" onClick={() => openExternal(all)}>
            All releases on GitHub
            <ExternalLink className="size-3.5" />
          </Button>
        )}
      </div>
    </Shell>
  )
}

function Shell({title, children}: {title: string; children: ReactNode}) {
  return (
    <div
      role="status"
      className="mb-6 flex gap-3 rounded-lg border border-accent/30 bg-accent-soft px-4 py-3.5 text-[13px]"
    >
      <ArrowUpCircle className="mt-0.5 size-4 shrink-0 text-accent-text" />
      <div className="min-w-0 flex-1">
        <div className="font-semibold">{title}</div>
        <div className="mt-0.5 text-foreground/85">{children}</div>
      </div>
    </div>
  )
}
