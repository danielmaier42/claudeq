import {Plus} from 'lucide-react'
import {ViewLayout} from '@/components/layout/view-layout'
import {Button} from '@/components/ui/button'
import {openAddTask} from '@/features/task-editor/store'
import {OverviewCards} from './OverviewCards'
import {PoolsSection} from './PoolsSection'
import {ProviderLimitsSection} from './ProviderLimitsSection'
import {useFreshLimits} from './use-fresh-limits'
import {useNow} from '@/hooks/use-now'

// The state of things at a glance: what runs and what is next, how much of
// each provider's allowance is left, how urgent it is to spend it, and the
// threshold above which backfill tasks do. The daemon reads the allowance
// without spending any — every quarter of an hour, after every run, and
// whenever this page opens or asks — and the background poll only ever reads
// what it remembered.
export function DashboardView() {
  const {reading, read} = useFreshLimits()
  const now = useNow()
  return (
    <ViewLayout
      title="Dashboard"
      subtitle="What runs now, what is next, and how much allowance is left"
      actions={
        <Button variant="primary" onClick={() => openAddTask()}>
          <Plus />
          Add task
        </Button>
      }
    >
      <OverviewCards />
      <ProviderLimitsSection now={now} reading={reading} onRefresh={read} />
      <PoolsSection />
    </ViewLayout>
  )
}
