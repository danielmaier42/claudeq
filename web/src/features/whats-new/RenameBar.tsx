import {Search, Tag, X} from 'lucide-react'
import {useState} from 'react'
import {Button} from '@/components/ui/button'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {StatusBar} from '@/features/status/StatusBar'
import {errorMessage, post} from '@/lib/api'
import {invalidate} from '@/lib/queries'
import {openWhatsNew, whatsNewKey} from './store'

// RenameBar stays above every view until the tasks were checked for ClaudeQ
// names: a task that still calls ClaudeQ.app fails once the old app is gone.
export function RenameBar() {
  const [dismissing, setDismissing] = useState(false)

  const dismiss = async () => {
    setDismissing(true)
    try {
      await post('/api/rename-check/dismiss')
      invalidate(whatsNewKey)
    } catch (e) {
      toast.error('Could not hide the banner', {description: errorMessage(e)})
    } finally {
      setDismissing(false)
    }
  }

  return (
    <StatusBar
      tone="info"
      icon={<Tag />}
      title="Some tasks still mention ClaudeQ"
      actions={
        <>
          <Button size="sm" variant="secondary" onClick={() => openWhatsNew(true)}>
            <Search />
            Check tasks
          </Button>
          <Tooltip content="Don't check">
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label="Don't check"
              loading={dismissing}
              onClick={() => void dismiss()}
            >
              {!dismissing && <X />}
            </Button>
          </Tooltip>
        </>
      }
    >
      A task that calls ClaudeQ.app or reads a CLAUDEQ_ variable fails once the old app is gone. The prompt
      reviewer can check them and fix the ones you pick.
    </StatusBar>
  )
}
