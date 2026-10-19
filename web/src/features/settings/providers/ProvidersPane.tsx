import {Plus, Server} from 'lucide-react'
import {useState} from 'react'
import {ProviderLogo} from '@/components/domain/provider-logo'
import {Section} from '@/components/layout/view-layout'
import {Alert} from '@/components/ui/alert'
import {Button} from '@/components/ui/button'
import {EmptyState} from '@/components/ui/empty-state'
import {Skeleton} from '@/components/ui/skeleton'
import {Tooltip} from '@/components/ui/tooltip'
import {errorMessage} from '@/lib/api'
import {useLimits, useProviders} from '@/lib/queries'
import {AddProviderDialog} from './AddProviderDialog'
import {BackfillSection} from './BackfillSection'
import {ProviderCard} from './ProviderCard'
import {byName, groupByKind} from './provider-logic'

// ProvidersPane is Settings → Providers: one card per configured harness.
// Every action on a card takes effect at once; there is no page-wide Save.
export function ProvidersPane() {
  const providers = useProviders()
  // The limits are what the daemon last read. A card without them is still a
  // complete card, so a failure here costs only the meters.
  const limits = useLimits().data
  const [adding, setAdding] = useState(false)
  const list = [...(providers.data ?? [])].sort(byName)
  const limitsOf = (id: string) => limits?.find((l) => l.id === id)?.limits

  const add = (
    <Tooltip content="A second account, or another harness such as Codex or opencode">
      <Button size="sm" onClick={() => setAdding(true)}>
        <Plus />
        Add provider
      </Button>
    </Tooltip>
  )

  return (
    <Section
      description="The CLIs ClaudeQ runs tasks with. Each one is checked by the daemon itself, without spending usage."
      actions={add}
    >
      {providers.isPending ? (
        <div className="flex flex-col gap-3">
          {[0, 1].map((i) => (
            <Skeleton key={i} className="h-40 rounded-lg" />
          ))}
        </div>
      ) : providers.isError && !providers.data ? (
        <Alert
          tone="danger"
          title="Providers could not be loaded"
          actions={
            <Button size="sm" onClick={() => void providers.refetch()}>
              Try again
            </Button>
          }
        >
          {errorMessage(providers.error)}
        </Alert>
      ) : list.length === 0 ? (
        <EmptyState
          icon={<Server />}
          title="No providers"
          description="Add the CLI ClaudeQ should run tasks with."
          action={add}
        />
      ) : (
        <div className="flex flex-col gap-6">
          {groupByKind(list).map((g) => (
            <div key={g.kind}>
              <div className="mb-2 flex items-center gap-2">
                <ProviderLogo kind={g.kind} className="size-4" />
                <h3 className="text-[12.5px] font-semibold">{g.name}</h3>
                <span className="text-[12px] text-subtle-foreground">{g.providers.length}</span>
              </div>
              <div className="flex flex-col gap-3">
                {g.providers.map((p) => (
                  <ProviderCard key={p.id} provider={p} providers={list} limits={limitsOf(p.id)} />
                ))}
              </div>
            </div>
          ))}
          <BackfillSection providers={list} />
        </div>
      )}
      <AddProviderDialog open={adding} onOpenChange={setAdding} />
    </Section>
  )
}
