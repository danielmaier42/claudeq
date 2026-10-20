import {cleanup, render, screen} from '@testing-library/react'
import {afterEach, describe, expect, it} from 'vitest'
import type {StatsBucket} from '@/lib/types'
import {ProviderBreakdown} from './ProviderBreakdown'

const bucket = (runs: number, tokens: number): StatsBucket => ({
  runs,
  success: runs,
  failed: 0,
  cost_usd: 0,
  input_tokens: tokens,
  output_tokens: 0,
  duration_ms: 0,
})

// Each provider lists the models it ran beneath it, the harness default named
// as such rather than left blank. A provider's share is of all tokens, its
// models' shares are of the provider's, so they add up to 100%.
describe('ProviderBreakdown', () => {
  afterEach(cleanup)

  it('lists each provider with its models beneath it', () => {
    render(
      <ProviderBreakdown
        period="Last 14 days"
        metric="tokens"
        providers={[
          {
            id: 'claude',
            name: 'dm-claude',
            ...bucket(3, 300),
            models: [
              {model: 'opus', ...bucket(2, 200)},
              {model: '', ...bucket(1, 100)},
            ],
          },
          {id: 'codex', name: 'dm-codex', ...bucket(1, 100)},
        ]}
      />,
    )
    const names = ['dm-claude', 'opus', 'Harness default', 'dm-codex'].map((n) => screen.getByText(n))
    for (let i = 1; i < names.length; i++) {
      expect(names[i - 1].compareDocumentPosition(names[i]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
    expect(screen.getByText('75%')).toBeTruthy()
    expect(screen.getByText('67%')).toBeTruthy()
    expect(screen.getByText('33%')).toBeTruthy()
  })

  it('follows the measure chosen for the chart', () => {
    render(
      <ProviderBreakdown
        period="Last 14 days"
        metric="runs"
        providers={[
          {id: 'a', name: 'few-tokens', ...bucket(3, 10)},
          {id: 'b', name: 'many-tokens', ...bucket(1, 1000)},
        ]}
      />,
    )
    const [first, second] = ['few-tokens', 'many-tokens'].map((n) => screen.getByText(n))
    expect(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByText('75%')).toBeTruthy()
    expect(screen.getAllByText('runs')).toHaveLength(2)
  })
})
