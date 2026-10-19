import {useQuery} from '@tanstack/react-query'
import {useEffect, useState} from 'react'
import {get} from '@/lib/api'
import {queryClient} from '@/lib/queries'
import type {CronCheck} from '@/lib/types'

/* The cron field is checked while it is typed, not only when the task is saved.
   The daemon owns the verdict (the same parser that will run the schedule), so
   the editor asks it instead of re-implementing crontab rules here. Verdicts are
   cached per expression, so saving does not ask again for text that was just
   checked, and an answer for text a later keystroke has overtaken is not shown. */
const CRON_DELAY = 250

const cronQuery = (expr: string) => ({
  queryKey: ['cron-check', expr] as const,
  queryFn: ({signal}: {signal: AbortSignal}) =>
    get<CronCheck>('/api/cron/check?expr=' + encodeURIComponent(expr), signal),
  staleTime: 60_000,
  retry: false,
})

// useCronCheck returns the verdict for exactly the text in the field, or null
// while there is none yet (empty field, still typing, daemon unreachable).
export function useCronCheck(text: string, active: boolean): CronCheck | null {
  const expr = text.trim()
  const [settled, setSettled] = useState(expr)
  useEffect(() => {
    const t = setTimeout(() => setSettled(expr), CRON_DELAY)
    return () => clearTimeout(t)
  }, [expr])
  const q = useQuery({...cronQuery(settled), enabled: active && !!settled})
  if (!active || !expr || settled !== expr || !q.data) return null
  return q.data
}

// checkCron is the verdict at save time. null means the daemon did not answer:
// saving is not blocked on a guess, since the POST/PUT validates the expression
// again server-side and reports it there.
export async function checkCron(expr: string): Promise<CronCheck | null> {
  try {
    return await queryClient.fetchQuery(cronQuery(expr.trim()))
  } catch {
    return null
  }
}
