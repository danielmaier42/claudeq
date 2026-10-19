import {toast} from '@/components/ui/toaster'
import {del, errorMessage, post, put} from '@/lib/api'
import {invalidate, keys, queryClient} from '@/lib/queries'
import type {Provider, ProviderInput} from '@/lib/types'

const path = (id: string, action = '') =>
  '/api/providers/' + encodeURIComponent(id) + (action ? '/' + action : '')

// Whatever changes a provider can change what the limits, the pools' rankings
// and that provider's model suggestions say, so all of them are read again.
export function refreshProviders(id?: string) {
  return invalidate(keys.providers, keys.limits, keys.pools, ...(id ? [keys.models(id)] : []))
}

function patchCached(id: string, patch: (p: Provider) => Provider) {
  queryClient.setQueryData<Provider[]>(keys.providers, (list) =>
    list?.map((p) => (p.id === id ? patch(p) : p)),
  )
}

// checkProvider re-probes the CLI now and puts the fresh verdict straight onto
// the card; the endpoint is the one that always spends the probe.
export async function checkProvider(p: Provider) {
  try {
    const fresh = await post<Provider>(path(p.id, 'check'))
    patchCached(p.id, () => fresh)
    const ok = fresh.health.state === 'ready'
    toast[ok ? 'success' : 'error'](`${p.name || p.id}: ${ok ? 'ready' : 'not ready'}`, {
      description: ok ? undefined : fresh.health.reason || fresh.health.detail,
    })
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await refreshProviders()
}

// setEnabled flips the switch at once and puts it back if the daemon refuses.
export async function setEnabled(p: Provider, enabled: boolean) {
  patchCached(p.id, (x) => ({...x, enabled}))
  try {
    await post(path(p.id, enabled ? 'enable' : 'disable'))
  } catch (e) {
    patchCached(p.id, (x) => ({...x, enabled: !enabled}))
    toast.error(errorMessage(e))
  }
  await refreshProviders()
}

export async function makeDefault(p: Provider) {
  try {
    await post(path(p.id, 'default'))
    toast.success(`${p.name || p.id} is now the default provider`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await invalidate(keys.providers, keys.limits, keys.settings)
}

export async function removeProvider(p: Provider) {
  try {
    await del(path(p.id))
    toast.success('Provider removed')
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await refreshProviders()
}

// saveProvider and addProvider throw: their dialogs show the daemon's refusal
// next to the fields rather than in a toast that leaves the form behind.
export async function saveProvider(id: string, body: ProviderInput) {
  const saved = await put<Provider>(path(id), body)
  patchCached(id, () => saved)
  await refreshProviders(id)
  return saved
}

export async function addProvider(body: ProviderInput) {
  const added = await post<Provider>('/api/providers', body)
  await refreshProviders()
  return added
}
