import {useEffect, useState} from 'react'

// useNow re-renders every `every` ms while `live`, so relative times ("3 min
// ago", "resets in 2 h") and a running run's duration move on between two
// polls. Not live, it stays put and costs nothing.
export function useNow(every = 60_000, live = true): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!live) return
    const id = setInterval(() => setNow(Date.now()), every)
    return () => clearInterval(id)
  }, [live, every])
  return now
}
