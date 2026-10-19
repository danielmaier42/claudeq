import {api} from '@/lib/api'
import type {PromptReviewKind, PromptReviewResponse, ReviewContext} from '@/lib/types'
import {reviewCache, reviewKey} from './cache'

/* Prompt review: Claude checks a draft prompt against this machine.
   Purely advisory — it never blocks saving. A review costs real Claude usage, so
   it runs only when the prompt or the working directory actually changes, never
   merely because an editor was opened: reopening a task nobody has touched shows
   the finding from last time, or nothing, and asks Claude for nothing. A change
   really is analysed from scratch, because a finding about the previous text
   says nothing about the new one, and whatever is in flight is aborted first
   (which kills the daemon's Claude process too), so only the newest answer can
   reach the banner. */
export const REVIEW_DELAY = 900 // ms of quiet typing before a review is worth starting

export interface ReviewInput {
  kind: PromptReviewKind
  prompt: string
  workingDir: string
}

export type ReviewView =
  | {state: 'hidden'}
  | {state: 'busy'; noun: string}
  | {state: 'shown'; res: PromptReviewResponse; key: string | null; remembered: boolean; noun: string}

const HIDDEN: ReviewView = {state: 'hidden'}
const nounOf = (k: PromptReviewKind) => (k === 'script' ? 'script' : 'prompt')

// reviewContext asks the daemon whether a review can run at all and who would
// answer it. It costs nothing — no model is involved — and only the daemon can
// say: a provider whose CLI is gone cannot review anything. An unreachable
// daemon says nothing about the prompt.
async function reviewContext(): Promise<ReviewContext | null> {
  try {
    return await api<ReviewContext>('GET', '/api/review/context')
  } catch {
    return null
  }
}

// ReviewController owns one banner's review. It is a small external store, so
// the component reads it with useSyncExternalStore and none of the timers or
// requests live in render state.
export class ReviewController {
  private view: ReviewView = HIDDEN
  private listeners = new Set<() => void>()
  private timer: ReturnType<typeof setTimeout> | undefined
  private ctrl: AbortController | null = null
  private seq = 0
  private input: ReviewInput | null = null
  private applied: string | null = null

  subscribe = (fn: () => void) => {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  }
  getSnapshot = () => this.view

  private set(v: ReviewView) {
    this.view = v
    this.listeners.forEach((fn) => fn())
  }

  // stop retires whatever is pending or in flight: the bumped sequence number
  // makes any answer still on its way arrive too late to be rendered.
  stop() {
    clearTimeout(this.timer)
    this.timer = undefined
    this.ctrl?.abort()
    this.ctrl = null
    this.seq++
  }

  reset() {
    this.stop()
    this.set(HIDDEN)
  }

  // dispose is unmount: nothing may keep running for a banner nobody sees.
  dispose() {
    this.reset()
    this.input = null
  }

  // update is called with what the box holds now. The first call only shows
  // what is already known (restore) unless the text is new to this Mac
  // (fresh); later calls review the change.
  update(next: ReviewInput, fresh: boolean) {
    const prev = this.input
    this.input = next
    if (!prev) {
      if (fresh) void this.run()
      else void this.restore()
      return
    }
    if (prev.kind === next.kind && prev.prompt === next.prompt && prev.workingDir === next.workingDir) return
    // A finding about the text as the other kind says nothing about this one,
    // and Apply's rewrite deserves its verdict straight away.
    if (prev.kind !== next.kind || (this.applied !== null && next.prompt === this.applied)) {
      this.applied = null
      this.reset()
      void this.run()
      return
    }
    this.applied = null
    this.schedule()
  }

  schedule() {
    this.reset()
    this.timer = setTimeout(() => void this.run(), REVIEW_DELAY)
  }

  markApplied(text: string) {
    this.applied = text
  }

  dismiss() {
    const v = this.view
    this.reset()
    // Dismissed for good: forgetting the answer stops it from coming back the
    // next time this editor opens, and re-reviewing is one edit away.
    if (v.state === 'shown' && v.key) reviewCache.forget(v.key)
  }

  checkAgain() {
    const v = this.view
    if (v.state === 'shown' && v.key) reviewCache.forget(v.key)
    void this.run()
  }

  private render(
    res: PromptReviewResponse | null | undefined,
    key: string | null,
    remembered: boolean,
    noun: string,
  ) {
    if (res && res.enabled && !res.ok && res.message) this.set({state: 'shown', res, key, remembered, noun})
    else this.set(HIDDEN)
  }

  // A task without a working directory waits for one: an imported task lands
  // with its folder dropped, because the exporter's path is not on this Mac.
  // Reviewing then would call every relative path unresolvable and bury the
  // editor's own "choose a folder" hint under it.
  private askable(i: ReviewInput) {
    return !!i.prompt.trim() && (i.kind === 'system' || !!i.workingDir.trim())
  }

  async restore() {
    this.stop()
    const mine = this.seq
    const i = this.input
    if (!i || !this.askable(i)) return this.set(HIDDEN)
    const ctx = await reviewContext()
    if (mine !== this.seq) return
    if (!ctx?.enabled) return this.set(HIDDEN) // no review to remember anything about
    const key = reviewKey(i.kind, i.prompt, i.workingDir.trim(), ctx.reviewer ?? '')
    this.render(reviewCache.get(key), key, true, nounOf(i.kind))
  }

  async run() {
    this.stop()
    const mine = this.seq
    const i = this.input
    if (!i || !this.askable(i)) return this.set(HIDDEN)
    const dir = i.workingDir.trim()
    const noun = nounOf(i.kind)
    // When the daemon cannot say who reviews, the review still goes ahead — the
    // endpoint decides anyway — and its answer is simply not remembered.
    const ctx = await reviewContext()
    if (mine !== this.seq) return
    if (ctx && !ctx.enabled) return this.set(HIDDEN)
    const key = ctx ? reviewKey(i.kind, i.prompt, dir, ctx.reviewer ?? '') : null
    const cached = key ? reviewCache.get(key) : null
    if (cached) return this.render(cached, key, true, noun)
    const ctrl = new AbortController()
    this.ctrl = ctrl
    this.set({state: 'busy', noun})
    let res: PromptReviewResponse | undefined
    // A review that is aborted, unreachable or fails is silently dropped: it is
    // an extra pair of eyes, and a broken one must never get in the way of
    // writing a task.
    try {
      res = await api<PromptReviewResponse | undefined>(
        'POST',
        '/api/review/prompt',
        {kind: i.kind, prompt: i.prompt, working_dir: dir},
        ctrl.signal,
      )
    } catch {
      if (mine === this.seq) this.set(HIDDEN)
      return
    }
    if (mine !== this.seq) return
    this.ctrl = null
    // 204: a newer review (another editor, or Settings) took the daemon's slot.
    // A disabled or superseded answer says nothing about this prompt.
    if (key && res?.enabled) reviewCache.set(key, res)
    this.render(res, key, false, noun)
  }
}
