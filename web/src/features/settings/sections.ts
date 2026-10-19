import {
  Bell,
  Boxes,
  Info,
  Layers,
  Palette,
  Play,
  Settings2,
  Sparkles,
  Wrench,
  type LucideIcon,
} from 'lucide-react'

export const SECTION_IDS = [
  'general',
  'appearance',
  'execution',
  'notifications',
  'providers',
  'pools',
  'assistant',
  'system',
  'about',
] as const
export type SectionId = (typeof SECTION_IDS)[number]

export interface SectionDef {
  id: SectionId
  label: string
  icon: LucideIcon
  description: string
}

export const SECTIONS: SectionDef[] = [
  {
    id: 'general',
    label: 'General',
    icon: Settings2,
    description: 'Defaults every new task and every run starts from',
  },
  {
    id: 'appearance',
    label: 'Appearance',
    icon: Palette,
    description: 'How ClaudeQ looks on this Mac and in a browser tab',
  },
  {
    id: 'execution',
    label: 'Execution',
    icon: Play,
    description: 'When runs start, how long they may hang and what is kept',
  },
  {
    id: 'notifications',
    label: 'Notifications',
    icon: Bell,
    description: 'Where run outcomes and new artifacts are announced',
  },
  {id: 'providers', label: 'Providers', icon: Boxes, description: 'The coding agents your tasks run on'},
  {id: 'pools', label: 'Pools', icon: Layers, description: 'Groups of providers that share a queue'},
  {
    id: 'assistant',
    label: 'Assistant',
    icon: Sparkles,
    description: 'The jobs Claude does for ClaudeQ itself',
  },
  {id: 'system', label: 'System', icon: Wrench, description: 'Beta features and removing ClaudeQ'},
  {id: 'about', label: 'About', icon: Info, description: 'Version and software updates'},
]

// Pools are a beta feature. Without beta features the app offers none and the
// section is gone, but a pool that already exists keeps it: hiding setup never
// hides work that is set up.
export function visibleSections(beta: boolean, poolCount: number): SectionDef[] {
  return SECTIONS.filter((s) => s.id !== 'pools' || beta || poolCount > 0)
}

export const isSectionId = (v: string): v is SectionId => (SECTION_IDS as readonly string[]).includes(v)

// pickSection decides what Settings shows: a section someone asked for (a
// link, the native menu), else the one last open this session, else About
// while an update waits, since that is where it is offered, else General.
export function pickSection({
  requested,
  last,
  updatePending,
  visible,
}: {
  requested: string
  last: string
  updatePending: boolean
  visible: SectionDef[]
}): SectionId {
  const ok = (v: string) => isSectionId(v) && visible.some((s) => s.id === v)
  if (ok(requested)) return requested as SectionId
  if (ok(last)) return last as SectionId
  return updatePending ? 'about' : 'general'
}
