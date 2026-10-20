import {toast} from '@/components/ui/toaster'
import {errorMessage, get, post} from '@/lib/api'
import {keys, queryClient} from '@/lib/queries'
import type {PathResult, RelaunchResult, UpdateDismissRequest, UpdateStatus} from '@/lib/types'

const store = (u: UpdateStatus) => queryClient.setQueryData(keys.update, u)

export const updatePending = (u: UpdateStatus | undefined) => !!u && (u.available || !!u.restart_required)

export const versionLabel = (u: UpdateStatus) => (u.supported ? 'v' + u.current : u.current)

// A link from the release feed is only followed when it is https.
export const httpsOnly = (url: string | undefined) => (url && /^https:\/\//i.test(url) ? url : '')

export function updateSummary(u: UpdateStatus): string {
  if (u.restart_required) return `DispatchQ ${u.installed} is installed but not running yet`
  if (u.available) return `DispatchQ ${u.latest} is available`
  if (!u.supported) return 'Development build: updates are checked in released versions'
  return 'You’re on the latest version' + (u.current ? ` (${u.current})` : '')
}

export async function checkForUpdates() {
  let u: UpdateStatus
  try {
    u = await post<UpdateStatus>('/api/update/check')
  } catch (e) {
    toast.error('Check failed', {description: errorMessage(e)})
    return
  }
  store(u)
  if (u.restart_required)
    toast(`DispatchQ ${u.installed} is installed but not running`, {description: 'Finish the update below.'})
  else if (u.available) toast.success(`Update available: DispatchQ ${u.latest}`)
  else if (!u.supported) toast('Running a development build')
  else if (u.error) toast.error('Check failed', {description: u.error})
  else toast.success('You’re up to date')
}

export async function downloadUpdate() {
  try {
    const r = await post<PathResult>('/api/update/download')
    toast.success('Installer downloaded', {
      description: `Follow the prompts to finish updating.${r?.path ? ` Saved to ${r.path}` : ''}`,
    })
  } catch (e) {
    toast.error('Download failed', {description: errorMessage(e)})
  }
}

// Whether a dev build can go back to the latest release, and why not if it can't.
// Undefined when there is nothing to offer: a release build, or no release known.
export function reinstallSummary(u: UpdateStatus): {label: string; description: string} | undefined {
  if (u.supported || !u.latest) return undefined
  const label = `Reinstall DispatchQ ${u.latest}`
  if (u.reinstall)
    return {
      label,
      description: 'Replaces this development build with the latest release. Tasks and history stay.',
    }
  if (!u.has_installer) return {label, description: `DispatchQ ${u.latest} has no installer.`}
  return {
    label,
    description: `Not possible: DispatchQ ${u.latest} cannot read this build’s data. ${u.data_floor} or later can.`,
  }
}

export async function reinstallRelease(version: string) {
  try {
    const r = await post<PathResult>('/api/update/reinstall')
    toast.success(`DispatchQ ${version} downloaded`, {
      description: `Follow the prompts to replace this build.${r?.path ? ` Saved to ${r.path}` : ''}`,
    })
  } catch (e) {
    toast.error('Reinstall failed', {description: errorMessage(e)})
  }
}

export async function dismissUpdate(version: string) {
  try {
    store(await post<UpdateStatus>('/api/update/dismiss', {version} satisfies UpdateDismissRequest))
    toast.success('Dismissed', {description: 'We’ll tell you about the next version.'})
  } catch (e) {
    toast.error('Could not dismiss the update', {description: errorMessage(e)})
  }
}

// finishUpdate hands over to the installed build. The daemon answering this
// page is being replaced, so the page waits for the new one before reloading;
// otherwise it would reload into a dead port. Resolves false when it gave up.
export async function finishUpdate(): Promise<boolean> {
  try {
    await post<RelaunchResult>('/api/update/relaunch')
  } catch (e) {
    toast.error('Could not finish the update', {description: errorMessage(e)})
    return false
  }
  for (let i = 0; i < 100; i++) {
    await new Promise((r) => setTimeout(r, 300))
    try {
      const u = await get<UpdateStatus>('/api/update')
      if (!u.restart_required) {
        location.reload()
        return true
      }
    } catch {
      /* still restarting */
    }
  }
  toast.error('The background service did not come back', {description: 'Reopen DispatchQ.'})
  return false
}
