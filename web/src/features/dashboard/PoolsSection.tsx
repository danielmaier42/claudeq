import {ArrowRight} from 'lucide-react'
import {Section} from '@/components/layout/view-layout'
import {Badge} from '@/components/ui/badge'
import {Card} from '@/components/ui/card'
import {Tooltip} from '@/components/ui/tooltip'
import {usePools} from '@/lib/queries'
import type {Pool, PoolRanking} from '@/lib/types'
import {cn} from '@/lib/utils'
import {fmtWeight} from './limits'

// PoolsSection shows, per pool, where a run started now would go and how the
// members stand: the same order the daemon takes them in. A configured pool
// shows here whether or not beta features are on.
export function PoolsSection() {
  const pools = usePools().data ?? []
  if (!pools.length) return null
  return (
    <Section
      title={
        <span className="flex items-center gap-1.5">
          Pools <Badge tone="ai">beta</Badge>
        </span>
      }
      description="Each run of a pool task goes to the member whose weekly allowance would otherwise expire unused soonest."
    >
      <Card className="divide-y divide-border">
        {pools.map((p) => (
          <PoolRow key={p.id} pool={p} />
        ))}
      </Card>
    </Section>
  )
}

function PoolRow({pool}: {pool: Pool}) {
  const ranking = pool.ranking ?? []
  const next = ranking.find((r) => r.tier < 3)
  return (
    <div className="flex flex-wrap items-start gap-x-6 gap-y-3 px-4 py-3.5">
      <div className="min-w-[200px] flex-1 basis-[220px]">
        <div className="flex items-center gap-1.5">
          <span className="text-[13.5px] font-semibold">{pool.name || pool.id}</span>
          <Badge tone="outline">Pool</Badge>
        </div>
        <p className="mt-1 text-[12.5px] text-muted-foreground">
          {next ? (
            <>
              Next run goes to <span className="font-medium text-foreground">{next.name}</span>
            </>
          ) : (
            <span className="text-[color-mix(in_oklab,var(--warning)_75%,var(--foreground))]">
              No member can take work right now
            </span>
          )}
        </p>
      </div>
      <ol className="min-w-[300px] flex-[2] basis-[320px] space-y-1">
        {ranking.map((r, i) => (
          <MemberRow key={r.provider} r={r} rank={i + 1} next={r === next} />
        ))}
      </ol>
    </div>
  )
}

function MemberRow({r, rank, next}: {r: PoolRanking; rank: number; next: boolean}) {
  // Nearly out (tier 2) or unable to take work (tier 3) go to the back.
  const back = r.tier >= 2
  return (
    <li
      className={cn(
        'flex items-center gap-2.5 rounded-md px-2 py-1 text-[13px]',
        next && 'bg-accent-soft',
        back && 'text-muted-foreground',
      )}
    >
      <span className="w-4 text-right text-[11.5px] text-subtle-foreground tabular-nums">{rank}</span>
      <span className={cn('truncate', next && 'font-medium')}>{r.name}</span>
      {next && <ArrowRight className="size-3.5 shrink-0 text-accent-text" aria-label="next" />}
      <span className="ml-auto truncate text-[12px] text-muted-foreground">{r.note}</span>
      <Tooltip content="Weight: how large its plan is next to the others'.">
        <span className="shrink-0 text-[12px] text-subtle-foreground tabular-nums">
          {fmtWeight(r.weight)}
        </span>
      </Tooltip>
    </li>
  )
}
