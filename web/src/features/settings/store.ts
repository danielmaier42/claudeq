import {create} from 'zustand'
import {combineEdits, type FieldKey, type SettingsEdits} from './draft'

// What Settings remembers for the session: the open section and the edits not
// saved yet. Both outlive the view, so a trip to the Queue and back finds the
// form as it was left instead of throwing the typing away.
interface SettingsUi {
  section: string
  edits: SettingsEdits
  errors: Partial<Record<FieldKey, string>>
  saving: boolean
}

export const useSettingsUi = create<SettingsUi>(() => ({section: '', edits: {}, errors: {}, saving: false}))

export function setSection(section: string) {
  useSettingsUi.setState({section})
}

// editDraft records an edit. A field that is edited loses its error: the
// complaint was about the value that is gone now.
export function editDraft(patch: SettingsEdits) {
  useSettingsUi.setState((s) => {
    const errors = {...s.errors}
    for (const [k, v] of Object.entries(patch)) {
      if (v && typeof v === 'object') for (const f of Object.keys(v)) delete errors[`${k}.${f}` as FieldKey]
    }
    return {edits: combineEdits(s.edits, patch), errors}
  })
}

export function discardDraft() {
  useSettingsUi.setState({edits: {}, errors: {}})
}
