import {create} from 'zustand'
import {errorMessage, post} from '@/lib/api'
import type {FeedbackDraft} from '@/lib/types'
import {manualBody} from './feedback'

/* Feedback: chat with Claude, then file the issue on GitHub yourself.
   The app never talks to GitHub and holds no token. Claude drafts the issue
   locally through the provider's CLI, the user edits it here, and the last
   step just opens GitHub's prefilled "new issue" page in the browser.

   The conversation lives in a store rather than in the view, so leaving the
   page mid-turn and coming back finds it where it was. */

export interface ChatMessage {
  id: number
  role: 'user' | 'assistant' | 'result' | 'error'
  text: string
}

interface FeedbackState {
  mode: 'chat' | 'review'
  messages: ChatMessage[]
  // Everything the user sent, for the hand-written fallback.
  said: string[]
  session: string
  // User messages in the current session, against the daemon's max_turns.
  turns: number
  composer: string
  busy: boolean
  // The assistant failed mid-conversation: offer a retry or the manual path.
  failed: boolean
  title: string
  body: string
  labels: string[]
  // Inline problem under the input that caused it.
  error: string
}

const initial: FeedbackState = {
  mode: 'chat',
  messages: [],
  said: [],
  session: '',
  turns: 0,
  composer: '',
  busy: false,
  failed: false,
  title: '',
  body: '',
  labels: [],
  error: '',
}

export const useFeedback = create<FeedbackState>(() => initial)

let nextId = 1
// A turn still running when the user starts over must not land in the new
// conversation.
let generation = 0

const say = (role: ChatMessage['role'], text: string) =>
  useFeedback.setState((s) => ({messages: [...s.messages, {id: nextId++, role, text}]}))

export const setComposer = (composer: string) => useFeedback.setState({composer, error: ''})
export const setDraft = (patch: Partial<Pick<FeedbackState, 'title' | 'body'>>) =>
  useFeedback.setState({...patch, error: ''})
export const setError = (error: string) => useFeedback.setState({error})

export function startOver() {
  generation++
  useFeedback.setState(initial)
}

// Without the assistant there is no chat: the user writes the issue directly.
export function writeManually() {
  useFeedback.setState((s) => ({
    mode: 'review',
    failed: false,
    error: '',
    body: s.body || manualBody(s.said),
  }))
}

export function sendMessage() {
  const s = useFeedback.getState()
  if (s.busy) return
  const text = s.composer.trim()
  if (!text) {
    setError('Write what you would like to report first.')
    return
  }
  say('user', text)
  useFeedback.setState((st) => ({said: [...st.said, text], composer: '', error: ''}))
  void runTurn(text)
}

// retry starts a fresh conversation after a failure, carrying everything said
// so far as its first message: the failed session is gone on the daemon's side.
export function retry() {
  const s = useFeedback.getState()
  if (s.busy || !s.said.length) return
  useFeedback.setState({failed: false, session: '', turns: 0, error: ''})
  void runTurn(manualBody(s.said))
}

async function runTurn(text: string) {
  const gen = generation
  const {session} = useFeedback.getState()
  useFeedback.setState((s) => ({busy: true, turns: s.turns + 1}))
  try {
    const d = await post<FeedbackDraft>('/api/feedback/turn', {session_id: session, text})
    if (gen !== generation) return
    useFeedback.setState({session: d.session_id || ''})
    if (d.status === 'ask') {
      say('assistant', d.question || '')
    } else {
      say('result', 'I drafted the issue below. Check it before filing.')
      useFeedback.setState({mode: 'review', title: d.title || '', body: d.body || '', labels: d.labels || []})
    }
  } catch (e) {
    if (gen !== generation) return
    say('error', errorMessage(e))
    useFeedback.setState({failed: true, session: '', turns: 0})
  } finally {
    if (gen === generation) useFeedback.setState({busy: false})
  }
}
