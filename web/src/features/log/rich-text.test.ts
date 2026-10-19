import {describe, expect, it} from 'vitest'
import {parseBlocks, parseInline} from './rich-text'

describe('parseBlocks', () => {
  it('splits code, headings, lists, quotes and paragraphs', () => {
    const src = [
      '## Plan',
      'First line',
      'second line',
      '',
      '- one',
      '- two',
      '  more',
      '',
      '```ts',
      'const a = 1',
      '```',
      '> said',
      '1. x',
    ].join('\n')
    expect(parseBlocks(src)).toEqual([
      {kind: 'heading', text: 'Plan'},
      {kind: 'para', text: 'First line\nsecond line'},
      {kind: 'list', ordered: false, items: ['one', 'two\nmore']},
      {kind: 'code', lang: 'ts', text: 'const a = 1'},
      {kind: 'quote', text: 'said'},
      {kind: 'list', ordered: true, items: ['x']},
    ])
  })

  it('runs an unclosed fence to the end', () => {
    expect(parseBlocks('```\nhalf')).toEqual([{kind: 'code', lang: '', text: 'half'}])
  })
})

describe('parseInline', () => {
  it('finds code, bold and links, and nothing else', () => {
    expect(parseInline('Run `make` **now**, see [docs](https://x.dev/a) or https://y.dev/b.')).toEqual([
      {kind: 'text', text: 'Run '},
      {kind: 'code', text: 'make'},
      {kind: 'text', text: ' '},
      {kind: 'bold', text: 'now'},
      {kind: 'text', text: ', see '},
      {kind: 'link', text: 'docs', href: 'https://x.dev/a'},
      {kind: 'text', text: ' or '},
      {kind: 'link', text: 'https://y.dev/b', href: 'https://y.dev/b'},
      {kind: 'text', text: '.'},
    ])
    expect(parseInline('<b>x</b> javascript:alert(1)')).toEqual([
      {kind: 'text', text: '<b>x</b> javascript:alert(1)'},
    ])
  })
})
