import {cleanup, fireEvent, render, screen} from '@testing-library/react'
import {afterEach, describe, expect, it, vi} from 'vitest'
import {Segmented} from './segmented'
import {TooltipProvider} from './tooltip'

// The Log's show filter has All as the empty value. Radix reports a deselect
// as '', so a click on All was once dropped and the filter stuck on Running.
describe('Segmented', () => {
  afterEach(cleanup)

  const options = [
    {value: '', label: 'All'},
    {value: 'running', label: 'Running'},
  ]

  it('selects an option whose value is empty', () => {
    const onValueChange = vi.fn()
    render(
      <TooltipProvider>
        <Segmented value="running" onValueChange={onValueChange} options={options} />
      </TooltipProvider>,
    )
    expect(screen.getByRole('radio', {name: 'Running'}).getAttribute('aria-checked')).toBe('true')
    fireEvent.click(screen.getByRole('radio', {name: 'All'}))
    expect(onValueChange).toHaveBeenCalledWith('')
  })

  it('keeps the selected option when it is clicked again', () => {
    const onValueChange = vi.fn()
    render(
      <TooltipProvider>
        <Segmented value="running" onValueChange={onValueChange} options={options} />
      </TooltipProvider>,
    )
    fireEvent.click(screen.getByRole('radio', {name: 'Running'}))
    expect(onValueChange).not.toHaveBeenCalled()
  })
})
