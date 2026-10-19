import {Trash2} from 'lucide-react'
import {useState} from 'react'
import {Row, RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {Button} from '@/components/ui/button'
import {UninstallDialog} from '../components/UninstallDialog'

export function SystemSection() {
  const [uninstall, setUninstall] = useState(false)

  return (
    <>
      <Section title="Danger zone">
        <RowGroup className="border-danger/30">
          <Row
            label="Remove DispatchQ from this Mac"
            description="Opens the uninstaller in the macOS Installer. It deletes the app, stops its background service and removes the wake permission. Your tasks and history stay unless you select them under Customize."
            control={
              <Button
                variant="danger-ghost"
                className="border border-danger/30"
                onClick={() => setUninstall(true)}
              >
                <Trash2 />
                Uninstall…
              </Button>
            }
          />
        </RowGroup>
      </Section>

      <UninstallDialog open={uninstall} onOpenChange={setUninstall} />
    </>
  )
}
