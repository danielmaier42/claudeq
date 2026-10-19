import {useQuery} from '@tanstack/react-query'
import {useEffect, useRef} from 'react'
import {toast} from '@/components/ui/toaster'
import {ApiError, get} from '@/lib/api'
import {invalidate, keys, useRuns} from '@/lib/queries'
import type {RunDetail} from '@/lib/types'
import {markRunRead} from '../actions'
import {closeRunLog} from '../store'

// While a run is running its log is tailed every 1.5 s; afterwards the record
// is still re-read now and then, since a waiting run can resume or be canceled
// from elsewhere while its sheet is open.
const LIVE = 1_500
const IDLE = 5_000

const runPath = (id: string) => `/api/runs/${encodeURIComponent(id)}`

// useRunData is everything the log sheet shows for one run: its record (with
// the prompt, which the list leaves out), its log, and whether it is live.
export function useRunData(runId: string) {
  const runs = useRuns()
  const listed = runs.data?.find((r) => r.run_id === runId)

  const detail = useQuery({
    queryKey: keys.run(runId),
    queryFn: ({signal}) => get<RunDetail>(runPath(runId), signal),
    refetchInterval: (q) => (q.state.data?.status === 'running' ? LIVE : IDLE),
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 1,
  })
  const run = detail.data ?? listed
  const live = run?.status === 'running'

  const log = useQuery({
    queryKey: keys.runLog(runId),
    // A run that has not written anything yet has no log file: that is an
    // empty log, not an error.
    queryFn: async ({signal}) => {
      try {
        return await get<string>(`${runPath(runId)}/log`, signal)
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return ''
        throw e
      }
    },
    refetchInterval: live ? LIVE : false,
  })

  // Opening a run's log marks it read.
  useEffect(() => {
    void markRunRead(runId)
  }, [runId])

  // The run may have been pruned from history since the link to it was made.
  const gone = detail.error instanceof ApiError && detail.error.status === 404 && !!runs.data && !listed
  useEffect(() => {
    if (!gone) return
    toast.error('That run is no longer in history')
    closeRunLog()
  }, [gone])

  // When the run stops, the lists catch up at once and the log is read one
  // last time, so the sheet does not end a poll short of the final output.
  const wasLive = useRef(live)
  useEffect(() => {
    if (wasLive.current && !live) void invalidate(keys.runs, keys.tasks, keys.artifacts)
    wasLive.current = live
  }, [live])

  return {
    run,
    live,
    log: log.data,
    logError: log.error,
    loading: !run && detail.isPending,
  }
}
