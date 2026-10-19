import {describe, expect, it} from 'vitest'
import {createReviewCache, reviewKey} from './cache'

const memory = () => {
  const m = new Map<string, string>()
  return {getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v)}
}
const res = {enabled: true, ok: false, message: 'missing file'}

describe('reviewKey', () => {
  it('tells every part of the question apart', () => {
    const k = reviewKey('task', 'p', '/a', 'claude/opus')
    expect(k).toBe(reviewKey('task', 'p', '/a', 'claude/opus'))
    expect(k).not.toBe(reviewKey('script', 'p', '/a', 'claude/opus'))
    expect(k).not.toBe(reviewKey('task', 'p', '/b', 'claude/opus'))
    expect(k).not.toBe(reviewKey('task', 'p', '/a', 'claude/sonnet'))
  })
})

describe('createReviewCache', () => {
  it('survives a reload through storage', () => {
    const s = memory()
    createReviewCache(s).set('k', res)
    expect(createReviewCache(s).get('k')).toEqual(res)
  })
  it('drops an answer after a day', () => {
    let now = 0
    const c = createReviewCache(memory(), () => now)
    c.set('k', res)
    now = 86_400_001
    expect(c.get('k')).toBeNull()
  })
  it('keeps at most 40 answers, oldest out first', () => {
    const c = createReviewCache(memory())
    for (let i = 0; i < 41; i++) c.set('k' + i, res)
    expect(c.get('k0')).toBeNull()
    expect(c.get('k40')).toEqual(res)
  })
  it('ignores storage holding something else', () => {
    const s = memory()
    s.setItem('cq.prompt-review', '{not json')
    expect(createReviewCache(s).get('k')).toBeNull()
  })
  it('forgets a dismissed answer', () => {
    const c = createReviewCache(memory())
    c.set('k', res)
    c.forget('k')
    expect(c.get('k')).toBeNull()
  })
})
