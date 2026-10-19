import {Layers, Plus} from 'lucide-react'
import {useState} from 'react'
import {Section} from '@/components/layout/view-layout'
import {Alert} from '@/components/ui/alert'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {EmptyState} from '@/components/ui/empty-state'
import {Skeleton} from '@/components/ui/skeleton'
import {errorMessage} from '@/lib/api'
import {useBeta} from '@/lib/providers'
import {usePools} from '@/lib/queries'
import {PoolCard} from './PoolCard'
import {PoolDialog} from './PoolDialog'

/* A pool is several providers of one type that a task can be given as a whole.
   Each run goes to the member whose weekly allowance would otherwise expire
   unused soonest (free share × weight ÷ hours to reset); the daemon decides,
   and the card shows the order it would take right now.

   Pools are a beta feature: without beta features nothing new is offered, but a
   pool that already exists keeps its card, because hiding setup never hides
   work that is set up. */
export function PoolsPane() {
  const beta = useBeta()
  const pools = usePools()
  const [adding, setAdding] = useState(false)
  const list = pools.data ?? []

  const add = beta && (
    <Button variant="ai" size="sm" onClick={() => setAdding(true)}>
      <Plus />
      Add pool
    </Button>
  )

  return (
    <Section
      title={
        <span className="inline-flex items-center gap-1.5">
          Pools <Badge tone="ai">Beta</Badge>
        </span>
      }
      description="Several accounts of one type behind one name. Every run goes where allowance would otherwise expire unused."
      actions={add}
    >
      {pools.isPending ? (
        <Skeleton className="h-28 rounded-lg" />
      ) : pools.isError && !pools.data ? (
        <Alert
          tone="danger"
          title="Pools could not be loaded"
          actions={
            <Button size="sm" onClick={() => void pools.refetch()}>
              Try again
            </Button>
          }
        >
          {errorMessage(pools.error)}
        </Alert>
      ) : list.length === 0 ? (
        <EmptyState
          icon={<Layers />}
          title="No pools yet"
          description={
            beta
              ? 'A pool groups providers of one type (two Claude subscriptions, say), and every run of a task on it goes to the account whose weekly allowance would otherwise expire unused soonest.'
              : 'Pools are a beta feature. Turn on beta features under System to set one up.'
          }
          action={add}
        />
      ) : (
        <div className="flex flex-col gap-3">
          {list.map((p) => (
            <PoolCard key={p.id} pool={p} />
          ))}
        </div>
      )}
      <PoolDialog open={adding} onOpenChange={setAdding} />
    </Section>
  )
}
