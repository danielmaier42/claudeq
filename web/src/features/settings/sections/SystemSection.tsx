import {Sparkles, Trash2} from 'lucide-react'
import {useState} from 'react'
import {Row, RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Switch} from '@/components/ui/switch'
import {invalidate, keys} from '@/lib/queries'
import type {Settings} from '@/lib/types'
import {commitSettings} from '../actions'
import {SaveMode} from '../components/parts'
import {UninstallDialog} from '../components/UninstallDialog'

export function SystemSection({value}: {value: Settings}) {
  const [uninstall, setUninstall] = useState(false)

  // Revealing beta providers and pools changes what may be added and chosen,
  // so every list that offers them is fetched again.
  const setBeta = async (on: boolean) => {
    if (await commitSettings({beta_features: on})) invalidate(keys.providers, keys.providerKinds, keys.pools)
  }

  return (
    <>
      <Section title="Beta features" actions={<SaveMode mode="instant" />}>
        <RowGroup>
          <Row
            htmlFor="s-beta"
            label={
              <span className="flex items-center gap-2">
                Show beta features
                <Badge tone="ai">
                  <Sparkles />
                  Beta
                </Badge>
              </span>
            }
            description={
              <>
                Parts of ClaudeQ that are not finished yet, such as more providers and pools. This decides
                what the app offers and nothing else: anything already set up keeps working, and the{' '}
                <code className="font-mono text-[12px]">claudeq</code> CLI accepts it either way.
              </>
            }
            control={
              <Switch id="s-beta" checked={value.beta_features} onCheckedChange={(on) => void setBeta(on)} />
            }
          />
        </RowGroup>
      </Section>

      <Section title="Danger zone">
        <RowGroup className="border-danger/30">
          <Row
            label="Remove ClaudeQ from this Mac"
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
