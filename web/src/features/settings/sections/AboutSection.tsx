import {RefreshCw, Star} from 'lucide-react'
import {useState} from 'react'
import {Row, RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Skeleton} from '@/components/ui/skeleton'
import {Tooltip} from '@/components/ui/tooltip'
import {relTime, fullTime} from '@/lib/format'
import {openExternal} from '@/lib/native'
import {useUpdate} from '@/lib/queries'
import {UpdateBanner} from '../components/UpdateBanner'
import {checkForUpdates, updatePending, updateSummary, versionLabel} from '../update'

const REPO = 'https://github.com/danielmaier42/claudeq'

export function AboutSection() {
  const update = useUpdate().data
  const [checking, setChecking] = useState(false)

  const check = async () => {
    setChecking(true)
    try {
      await checkForUpdates()
    } finally {
      setChecking(false)
    }
  }

  return (
    <>
      {update && updatePending(update) && <UpdateBanner update={update} />}

      <Section>
        <RowGroup>
          <div className="flex items-center gap-4 px-4 py-4">
            <img src="/logo.svg" alt="" className="size-11 shrink-0 rounded-[10px] shadow-sm" />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 text-[15px] font-semibold">
                ClaudeQ
                {update?.current ? (
                  <Badge tone="outline" className="font-mono">
                    {versionLabel(update)}
                  </Badge>
                ) : (
                  <Skeleton className="h-4 w-14 rounded-full" />
                )}
              </div>
              <div className="text-[12.5px] text-muted-foreground">
                Queue Claude Code tasks by day, run them by night. Free and open source.
              </div>
            </div>
            <Button onClick={() => openExternal(REPO)}>
              <Star />
              Star on GitHub
            </Button>
          </div>
        </RowGroup>
      </Section>

      <Section title="Software updates">
        <RowGroup>
          <Row
            label={update ? updateSummary(update) : 'Check GitHub for a newer release of ClaudeQ'}
            description={
              update?.last_checked ? (
                <Tooltip content={fullTime(update.last_checked)}>
                  <span className="cursor-default">Last checked {relTime(update.last_checked)}</span>
                </Tooltip>
              ) : (
                'ClaudeQ checks GitHub every hour'
              )
            }
            control={
              <Button loading={checking || !!update?.checking} onClick={() => void check()}>
                {!(checking || update?.checking) && <RefreshCw />}
                {checking || update?.checking ? 'Checking…' : 'Check for updates'}
              </Button>
            }
          />
        </RowGroup>
      </Section>

      <p className="mt-8 text-center text-[12px] text-subtle-foreground">
        Made with <span className="text-accent-text">♥</span> by{' '}
        <button
          type="button"
          onClick={() => openExternal('https://github.com/danielmaier42')}
          className="font-medium text-accent-text outline-none hover:underline focus-visible:underline"
        >
          danielmaier42
        </button>
      </p>
    </>
  )
}
