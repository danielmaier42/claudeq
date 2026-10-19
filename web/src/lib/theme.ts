import {useEffect} from 'react'
import {create} from 'zustand'

// The app's look: light or dark, and the accent colour. Both follow the Mac
// unless Settings → Appearance pins them. The choice lives in the daemon's
// config (appearance, accent) so the app window and a browser tab agree; a
// copy in localStorage lets index.html paint the right scheme before the
// settings have loaded.

export type Appearance = '' | 'light' | 'dark'

export const APPEARANCES: {value: Appearance; label: string}[] = [
  {value: '', label: 'System'},
  {value: 'light', label: 'Light'},
  {value: 'dark', label: 'Dark'},
]

// The presets mirror the macOS accent palette, plus two that macOS lacks.
export const ACCENT_PRESETS: {name: string; hex: string}[] = [
  {name: 'Blue', hex: '#0a84ff'},
  {name: 'Indigo', hex: '#5e5ce6'},
  {name: 'Purple', hex: '#bf5af2'},
  {name: 'Pink', hex: '#ff375f'},
  {name: 'Red', hex: '#ff453a'},
  {name: 'Orange', hex: '#ff9f0a'},
  {name: 'Yellow', hex: '#ffd60a'},
  {name: 'Green', hex: '#30d158'},
  {name: 'Teal', hex: '#40c8e0'},
  {name: 'Graphite', hex: '#8e8e93'},
]

const FALLBACK_ACCENT = '#0a84ff'

export function pinnedAppearance(v: unknown): Appearance {
  return v === 'light' || v === 'dark' ? v : ''
}

export function validAccent(v: unknown): string {
  return typeof v === 'string' && /^#[0-9a-f]{6}$/i.test(v) ? v.toLowerCase() : ''
}

interface ThemeState {
  appearance: Appearance
  accent: string
  systemDark: boolean
  systemAccent: string
  setAppearance: (a: Appearance) => void
  setAccent: (hex: string) => void
}

const darkQuery = window.matchMedia('(prefers-color-scheme: dark)')

function stored(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}

export const useTheme = create<ThemeState>((set) => ({
  appearance: pinnedAppearance(stored('cq-appearance')),
  accent: validAccent(stored('cq-accent-override')),
  systemDark: darkQuery.matches,
  systemAccent: validAccent(window.cqSystemAccent) || probeAccentColor(),
  setAppearance: (appearance) => set({appearance}),
  setAccent: (hex) => set({accent: validAccent(hex)}),
}))

export const resolvedDark = (s: Pick<ThemeState, 'appearance' | 'systemDark'>) =>
  s.appearance === 'dark' || (s.appearance === '' && s.systemDark)

export const resolvedAccent = (s: Pick<ThemeState, 'accent' | 'systemAccent'>) =>
  s.accent || s.systemAccent || FALLBACK_ACCENT

// probeAccentColor reads the CSS system colour AccentColor, which WebKit maps
// to the Mac's accent. Where the keyword is unknown it reads back as the probe's
// default and the fallback blue is used.
function probeAccentColor(): string {
  if (!CSS.supports('color', 'AccentColor')) return ''
  const p = document.createElement('span')
  p.style.color = 'AccentColor'
  document.body.append(p)
  const rgb = getComputedStyle(p).color
  p.remove()
  return rgbToHex(rgb)
}

function rgbToHex(rgb: string): string {
  const m = rgb.match(/\d+(\.\d+)?/g)
  if (!m || m.length < 3) return ''
  return (
    '#' +
    m
      .slice(0, 3)
      .map((n) => Math.round(Number(n)).toString(16).padStart(2, '0'))
      .join('')
  )
}

// readableOn picks black or white text for a fill, by relative luminance.
export function readableOn(hex: string): string {
  const v = validAccent(hex)
  if (!v) return '#fff'
  const lin = (i: number) => {
    const c = parseInt(v.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  const l = 0.2126 * lin(1) + 0.7152 * lin(3) + 0.0722 * lin(5)
  return l > 0.45 ? '#111114' : '#fff'
}

// useThemeEffects applies the resolved theme to the document and keeps the
// system half current: the Mac's scheme through the media query, its accent
// through the app window's native bridge (or AccentColor in a browser).
export function useThemeEffects() {
  const appearance = useTheme((s) => s.appearance)
  const accent = useTheme((s) => s.accent)
  const systemDark = useTheme((s) => s.systemDark)
  const systemAccent = useTheme((s) => s.systemAccent)

  useEffect(() => {
    const onScheme = (e: MediaQueryListEvent) => useTheme.setState({systemDark: e.matches})
    darkQuery.addEventListener('change', onScheme)
    const onAccent = (e: Event) => {
      const hex = validAccent((e as CustomEvent<string>).detail)
      useTheme.setState({systemAccent: hex || probeAccentColor()})
    }
    window.addEventListener('cq:system-accent', onAccent)
    window.cqReadAccent?.().then((hex) => {
      if (validAccent(hex)) useTheme.setState({systemAccent: validAccent(hex)})
    })
    return () => {
      darkQuery.removeEventListener('change', onScheme)
      window.removeEventListener('cq:system-accent', onAccent)
    }
  }, [])

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle('dark', resolvedDark({appearance, systemDark}))
    window.cqSetAppearance?.(appearance)
    try {
      if (appearance) localStorage.setItem('cq-appearance', appearance)
      else localStorage.removeItem('cq-appearance')
    } catch {
      /* private mode: the pre-paint copy is a nicety */
    }
  }, [appearance, systemDark])

  useEffect(() => {
    const hex = resolvedAccent({accent, systemAccent})
    const root = document.documentElement
    root.style.setProperty('--accent', hex)
    root.style.setProperty('--accent-foreground', readableOn(hex))
    try {
      localStorage.setItem('cq-accent', hex)
      if (accent) localStorage.setItem('cq-accent-override', accent)
      else localStorage.removeItem('cq-accent-override')
    } catch {
      /* see above */
    }
  }, [accent, systemAccent])
}
