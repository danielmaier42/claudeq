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
      description="A pool task runs on the member whose weekly allowance would otherwise expire unused soonest."
    >
      <Card className="divide-y divide-border">
        {pools.map((p) => (
          <PoolRow key={p.id} pool={p} />
        ))}
      </Card>
    </Section>
  )
}

// PoolRow is one pool on one line: its name, then its members in the order a
// run started now would try them, the next one marked.
function PoolRow({pool}: {pool: Pool}) {
  const ranking = pool.ranking ?? []
  const next = ranking.find((r) => r.tier < 3)
  return (
    <div className="flex min-h-10 flex-wrap items-center gap-x-4 gap-y-1.5 px-4 py-2">
      <span className="w-[140px] shrink-0 truncate text-[13px] font-medium">{pool.name || pool.id}</span>
      <ol className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
        {ranking.map((r, i) => (
          <MemberChip key={r.provider} r={r} rank={i + 1} next={r === next} />
        ))}
      </ol>
      {!next && (
        <span className={cn('text-[12px]', 'text-warning-text')}>No member can take work right now</span>
      )}
    </div>
  )
}

function MemberChip({r, rank, next}: {r: PoolRanking; rank: number; next: boolean}) {
  // Nearly out (tier 2) or unable to take work (tier 3) go to the back.
  const back = r.tier >= 2
  return (
    <li>
      <Tooltip
        content={`${next ? 'Next run goes here. ' : ''}${r.note ? `${r.note}. ` : ''}Weight ${fmtWeight(r.weight)}: how large its plan is next to the others'.`}
      >
        <span
          tabIndex={0}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-[12.5px] outline-none focus-visible:ring-2 focus-visible:ring-accent/40',
            next && 'border-transparent bg-accent-soft font-medium text-accent-text',
            back && 'text-muted-foreground',
          )}
        >
          <span className="text-[11px] text-subtle-foreground tabular-nums">{rank}</span>
          {r.name}
          {next && <ArrowRight className="size-3" aria-label="next" />}
        </span>
      </Tooltip>
    </li>
  )
}
