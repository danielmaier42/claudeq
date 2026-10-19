import {ArrowRight, MoreHorizontal, Pencil, Trash2} from 'lucide-react'
import {useState} from 'react'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Card} from '@/components/ui/card'
import {confirm} from '@/components/ui/confirm'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {del, errorMessage} from '@/lib/api'
import {relTime} from '@/lib/format'
import {invalidate, keys} from '@/lib/queries'
import type {Pool, PoolRanking} from '@/lib/types'
import {cn} from '@/lib/utils'
import {formatWeight, nextMember} from './pool-logic'
import {PoolDialog} from './PoolDialog'

// PoolCard shows a pool the way the daemon would use it right now: its
// members in the order a run started this moment would try them, best first.
export function PoolCard({pool}: {pool: Pool}) {
  const [editing, setEditing] = useState(false)
  const next = nextMember(pool)
  const label = pool.name || pool.id
  const set = new Map(pool.members.map((m) => [m.provider, m.weight]))
  const ranking = pool.ranking ?? []

  const remove = async () => {
    const ok = await confirm({
      title: `Remove the pool “${label}”?`,
      description: 'Tasks that run on it have to be moved first.',
      confirmLabel: 'Remove',
      destructive: true,
    })
    if (!ok) return
    try {
      await del('/api/pools/' + encodeURIComponent(pool.id))
      toast.success('Pool removed')
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await invalidate(keys.pools)
  }

  return (
    <Card className="overflow-hidden">
      <div className="flex items-start gap-3 px-4 pt-3.5 pb-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="text-[13.5px] font-semibold">{label}</h3>
            <Badge tone="outline">Pool</Badge>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-2 text-[12.5px] text-muted-foreground">
            <span className="font-mono">{pool.id}</span>
            <span>·</span>
            {next ? (
              <span className="inline-flex items-center gap-1">
                Next run goes to <span className="font-medium text-foreground">{next.name}</span>
              </span>
            ) : (
              <span className="text-warning">No member can take work right now</span>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => setEditing(true)}>
            <Pencil />
            Edit
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label={`More actions for ${label}`}>
                <MoreHorizontal />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem destructive onSelect={() => void remove()}>
                <Trash2 />
                Remove…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <ol className="flex flex-wrap gap-2 border-t border-border bg-surface-2/40 px-4 py-3">
        {ranking.map((r, i) => (
          <MemberChip
            key={r.provider}
            rank={i + 1}
            r={r}
            explicit={!!set.get(r.provider)}
            first={r === next}
          />
        ))}
        {ranking.length === 0 && (
          <li className="text-[12.5px] text-muted-foreground">No members. Edit the pool to add some.</li>
        )}
      </ol>
      <PoolDialog pool={pool} open={editing} onOpenChange={setEditing} />
    </Card>
  )
}

function MemberChip({
  rank,
  r,
  explicit,
  first,
}: {
  rank: number
  r: PoolRanking
  explicit: boolean
  first: boolean
}) {
  const detail = [
    r.note,
    r.tier < 3 && r.week_free ? `${Math.round(r.week_free)}% of the week left` : '',
    r.resets_at ? `resets ${relTime(r.resets_at)}` : '',
    r.running ? `${r.running} run${r.running > 1 ? 's' : ''} on it` : '',
    `urgency ${r.urgency.toFixed(1)} × weight ${formatWeight(r.weight)} = score ${r.score.toFixed(1)}`,
    explicit ? 'weight set by you' : 'weight from the plan the provider reports',
  ]
    .filter(Boolean)
    .join('\n')
  return (
    <Tooltip content={detail}>
      <li
        tabIndex={0}
        className={cn(
          'flex min-w-0 items-center gap-2 rounded-md border border-border bg-surface px-2.5 py-1.5 text-[12.5px] shadow-xs outline-none focus-visible:ring-3 focus-visible:ring-accent/20',
          first && 'border-accent/50 bg-accent-soft',
          r.tier >= 2 && 'opacity-60',
        )}
      >
        <span className="text-[11px] font-semibold text-subtle-foreground tabular-nums">{rank}</span>
        {first && <ArrowRight className="size-3.5 text-accent-text" />}
        <span className="font-medium">{r.name}</span>
        <span className="truncate text-muted-foreground">{r.note}</span>
        <Badge tone={explicit ? 'neutral' : 'outline'} className="tabular-nums">
          {formatWeight(r.weight)}×{explicit ? '' : ' auto'}
        </Badge>
      </li>
    </Tooltip>
  )
}
