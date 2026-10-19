import type {NtfySettings, PushoverSettings, Settings, WebhookSettings} from '@/lib/types'

// Settings are written whole (PUT /api/settings takes every field), but the
// form only ever owns a few of them at a time. Edits are therefore kept as a
// patch and laid over the latest stored settings when they are written, so a
// value changed elsewhere meanwhile (appearance, accent, pause, beta) is never
// carried back stale.

type Channel = 'pushover' | 'ntfy' | 'webhook'

export type SettingsEdits = Partial<Omit<Settings, Channel>> & {
  pushover?: Partial<PushoverSettings>
  ntfy?: Partial<NtfySettings>
  webhook?: Partial<WebhookSettings>
}

const CHANNELS: Channel[] = ['pushover', 'ntfy', 'webhook']

export function mergeSettings(base: Settings, edits: SettingsEdits): Settings {
  return {
    ...base,
    ...edits,
    pushover: {...base.pushover, ...edits.pushover},
    ntfy: {...base.ntfy, ...edits.ntfy},
    webhook: {...base.webhook, ...edits.webhook},
  }
}

// combineEdits lays a newer patch over an older one, channel by channel.
export function combineEdits(a: SettingsEdits, b: SettingsEdits): SettingsEdits {
  const out: SettingsEdits = {...a, ...b}
  for (const c of CHANNELS) if (a[c] || b[c]) Object.assign(out, {[c]: {...a[c], ...b[c]}})
  return out
}

// pruneEdits drops every edit that matches what is stored: typing a value and
// typing it back is not an unsaved change.
export function pruneEdits(base: Settings, edits: SettingsEdits): SettingsEdits {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(edits)) {
    if (v === undefined) continue
    if ((CHANNELS as string[]).includes(k)) {
      const stored = base[k as Channel] as unknown as Record<string, unknown>
      const changed = Object.entries(v as Record<string, unknown>).filter(
        ([f, x]) => x !== undefined && x !== stored[f],
      )
      if (changed.length) out[k] = Object.fromEntries(changed)
    } else if (v !== base[k as keyof Settings]) {
      out[k] = v
    }
  }
  return out as SettingsEdits
}

export const hasEdits = (e: SettingsEdits) => Object.keys(e).length > 0

// The fields the API can refuse, addressed as "<channel>.<field>".
export type FieldKey =
  'pushover.token' | 'pushover.user_key' | 'ntfy.server' | 'ntfy.topic' | 'webhook.url' | 'webhook.template'

const ERROR_FIELDS: [RegExp, FieldKey][] = [
  [/^pushover: the api token/i, 'pushover.token'],
  [/^pushover: the user key/i, 'pushover.user_key'],
  [/^ntfy server/i, 'ntfy.server'],
  [/^ntfy topic/i, 'ntfy.topic'],
  [/^webhook url/i, 'webhook.url'],
  [/^webhook body/i, 'webhook.template'],
]

// fieldError places a validation error from PUT /api/settings next to the field
// it is about. The daemon prefixes each message with what it refers to; the
// field label already says that, so only the rest is shown there.
export function fieldError(message: string): {field: FieldKey; text: string} | null {
  const hit = ERROR_FIELDS.find(([re]) => re.test(message))
  if (!hit) return null
  const rest = message.slice(message.indexOf(':') + 1).trim()
  return {field: hit[1], text: rest ? rest[0].toUpperCase() + rest.slice(1) : message}
}

// The Settings section that shows a field, so the dot on the section list and a
// refused save can point at the right place.
export function sectionOfEdit(key: string): string {
  if ((CHANNELS as string[]).includes(key) || key === 'artifact_notify_disabled') return 'notifications'
  if (key === 'system_prompt' || key === 'default_working_dir') return 'general'
  if (key.startsWith('prompt_review') || key.startsWith('feedback')) return 'assistant'
  if (key === 'appearance' || key === 'accent') return 'appearance'
  if (key === 'beta_features') return 'system'
  return 'execution'
}

export function editedSections(edits: SettingsEdits): Set<string> {
  return new Set(Object.keys(edits).map(sectionOfEdit))
}
