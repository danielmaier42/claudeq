import {useCallback, useLayoutEffect, useRef, useState} from 'react'

// useStickToBottom follows new output like a terminal: while the view is at
// the bottom it stays there as content arrives; once the user scrolls up it
// leaves them where they are until they come back down.
export function useStickToBottom<T extends HTMLElement>(content: unknown) {
  const ref = useRef<T>(null)
  const stuck = useRef(true)
  const [atBottom, setAtBottom] = useState(true)

  const onScroll = useCallback(() => {
    const el = ref.current
    if (!el) return
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48
    stuck.current = bottom
    setAtBottom(bottom)
  }, [])

  useLayoutEffect(() => {
    const el = ref.current
    if (el && stuck.current) el.scrollTop = el.scrollHeight
  }, [content])

  const toBottom = useCallback(() => {
    const el = ref.current
    if (!el) return
    stuck.current = true
    el.scrollTo({top: el.scrollHeight, behavior: 'smooth'})
  }, [])

  return {ref, onScroll, atBottom, toBottom}
}
