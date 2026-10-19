import {RotateCcw, SquareTerminal, TimerOff, X} from 'lucide-react'
import {useState} from 'react'
import {Button} from '@/components/ui/button'
import {SheetClose} from '@/components/ui/sheet'
import {Tooltip} from '@/components/ui/tooltip'
import type {Run} from '@/lib/types'
import {cancelResume, cancelRunning, continueInTerminal, replayRun} from '../actions'
import {canContinue, canTakeOver, cancelMode} from '../runs'

// The sheet's footer: call the run off, run its task again, or pick its session
// up in Terminal. Continue follows the run: offered as a take-over while it
// runs, as a plain resume once it has finished for good, and hidden while it
// waits to be resumed by the queue.
export function RunLogActions({run}: {run: Run}) {
  const [busy, setBusy] = useState<'' | 'cancel' | 'continue'>('')
  // Once the user has canceled, a poll answered before the cancel landed must
  // not offer the action again.
  const [canceled, setCanceled] = useState(false)
  const mode = canceled ? '' : cancelMode(run)
  const takeOver = canTakeOver(run)
  const resume = canContinue(run)

  const cancel = async () => {
    setBusy('cancel')
    const ok = mode === 'resume' ? await cancelResume(run) : await cancelRunning(run)
    if (ok) setCanceled(true)
    setBusy('')
  }
  const cont = async () => {
    if (busy) return
    setBusy('continue')
    await continueInTerminal(run, takeOver)
    setBusy('')
  }

  return (
    <>
      {mode && (
        <Tooltip
          content={
            mode === 'resume'
              ? 'Drop the scheduled resume so this task does not start again'
              : 'Stop the running process'
          }
        >
          <Button variant="danger-ghost" onClick={cancel} loading={busy === 'cancel'} className="mr-auto">
            {mode === 'resume' ? <TimerOff /> : <X />}
            {mode === 'resume' ? 'Cancel resume' : 'Cancel task'}
          </Button>
        </Tooltip>
      )}
      {run.task && (
        <Tooltip content="Run this task again: opens a new task prefilled from this run">
          <Button onClick={() => void replayRun(run)}>
            <RotateCcw />
            Replay
          </Button>
        </Tooltip>
      )}
      {(takeOver || resume) && (
        <Tooltip
          content={
            takeOver
              ? "Stops the unattended run and reopens its chat in Terminal, in the task's folder, with the full conversation context"
              : "Opens Terminal in the task's folder and resumes this chat interactively, with the full conversation context"
          }
        >
          <Button variant="primary" onClick={cont} loading={busy === 'continue'}>
            <SquareTerminal />
            {takeOver ? 'Take over in Terminal' : 'Continue in Terminal'}
          </Button>
        </Tooltip>
      )}
      <SheetClose asChild>
        <Button variant={takeOver || resume ? 'secondary' : 'primary'}>Done</Button>
      </SheetClose>
    </>
  )
}
