import {toast} from '@/components/ui/toaster'
import {api, errorMessage} from '@/lib/api'
import type {ImportResult} from '@/lib/types'
import {openTaskEditor} from './store'

// pickAndImportTask asks for a .dispatchq file and opens the editor on it: a shared
// file's paths belong to the machine it came from, so nothing is queued until
// the user has looked it over. In the app window WKWebView answers the file
// input with a native open panel.
export function pickAndImportTask() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.dispatchq,.claudeq'
  input.onchange = async () => {
    const file = input.files?.[0]
    if (!file) return
    try {
      const result = await api<ImportResult>('POST', '/api/tasks/import', file)
      openTaskEditor({mode: 'import', result})
    } catch (e) {
      toast.error('Could not import ' + file.name, {description: errorMessage(e)})
    }
  }
  input.click()
}
