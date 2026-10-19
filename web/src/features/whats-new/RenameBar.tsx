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
// names: calling ClaudeQ.app only works through a link a later release removes.
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
      Calling ClaudeQ.app or reading a CLAUDEQ_ variable still works, but it is deprecated and stops in a
      later release. The prompt reviewer can check your tasks and fix the ones you pick.
    </StatusBar>
  )
}
