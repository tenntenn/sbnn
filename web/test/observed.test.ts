import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { syncObserved } from '../src/observed'

/**
 * #402: the stacks used to observe every section again whenever a round
 * arrived. syncObserved is what lets them keep one observer and touch only the
 * sections that came or went.
 */

type El = { name: string }
const el = (name: string): El => ({ name })

function recorder() {
  const log: string[] = []
  return {
    log,
    observe: (e: Element) => void log.push('+' + (e as unknown as El).name),
    unobserve: (e: Element) => void log.push('-' + (e as unknown as El).name),
  }
}

describe('syncObserved', () => {
  const a = el('a')
  const b = el('b')
  const c = el('c')
  const all: Record<string, El> = { a, b, c }
  const find = (key: string) => all[key] as unknown as Element | undefined

  it('observes everything the first time', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    const gone = syncObserved(r, watched, ['a', 'b'], find)
    assert.deepEqual(r.log, ['+a', '+b'])
    assert.deepEqual(gone, [])
    assert.deepEqual([...watched.keys()], ['a', 'b'])
  })

  it('touches only what was added', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    syncObserved(r, watched, ['a', 'b'], find)
    r.log.length = 0
    syncObserved(r, watched, ['a', 'b', 'c'], find)
    assert.deepEqual(r.log, ['+c'])
  })

  it('does nothing when nothing changed', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    syncObserved(r, watched, ['a', 'b'], find)
    r.log.length = 0
    assert.deepEqual(syncObserved(r, watched, ['a', 'b'], find), [])
    assert.deepEqual(r.log, [])
  })

  it('lets go of what is gone and says which', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    syncObserved(r, watched, ['a', 'b', 'c'], find)
    r.log.length = 0
    const gone = syncObserved(r, watched, ['a', 'c'], find)
    assert.deepEqual(r.log, ['-b'])
    assert.deepEqual(gone, ['b'])
    assert.deepEqual([...watched.keys()], ['a', 'c'])
  })

  it('moves a key over to a replaced element', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    syncObserved(r, watched, ['a'], find)
    r.log.length = 0
    const a2 = el('a2')
    syncObserved(r, watched, ['a'], () => a2 as unknown as Element)
    assert.deepEqual(r.log, ['-a', '+a2'])
    assert.equal(watched.get('a'), a2 as unknown as Element)
  })

  it('skips a key that has no element yet and picks it up later', () => {
    const r = recorder()
    const watched = new Map<string, Element>()
    syncObserved(r, watched, ['a', 'zz'], find)
    assert.deepEqual(r.log, ['+a'])
    all.zz = el('zz')
    syncObserved(r, watched, ['a', 'zz'], find)
    assert.deepEqual(r.log, ['+a', '+zz'])
  })
})
