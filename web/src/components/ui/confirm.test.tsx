import {act, fireEvent, render, screen} from '@testing-library/react'
import {describe, expect, it} from 'vitest'
import {confirm, ConfirmHost, promptText} from './confirm'

// The host draws every confirmation in the app (deleting a task, a provider,
// an artifact). It once crashed on open, which blanked the whole window.
describe('ConfirmHost', () => {
  it('asks and resolves with the answer', async () => {
    render(<ConfirmHost />)
    let answer: Promise<boolean> | undefined
    act(() => {
      answer = confirm({title: 'Delete the task?', confirmLabel: 'Delete', destructive: true})
    })
    expect(await screen.findByRole('heading', {name: 'Delete the task?'})).toBeTruthy()
    fireEvent.click(screen.getByRole('button', {name: 'Delete'}))
    await expect(answer).resolves.toBe(true)
  })

  it('returns the typed text from a prompt', async () => {
    render(<ConfirmHost />)
    let answer: Promise<string | null> | undefined
    act(() => {
      answer = promptText({title: 'Name the group', confirmLabel: 'Create'})
    })
    fireEvent.change(await screen.findByRole('textbox'), {target: {value: '  Nightly  '}})
    fireEvent.click(screen.getByRole('button', {name: 'Create'}))
    await expect(answer).resolves.toBe('Nightly')
  })
})
