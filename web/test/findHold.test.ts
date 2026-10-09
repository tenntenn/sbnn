import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  FIND_BATCH,
  FIND_QUIET_MS,
  endsFind,
  findExpired,
  mountedWithFind,
  nextFindBatch,
  startsFind,
} from '../src/findHold'

/**
 * Sections far from the viewport have no body, so the browser's find cannot
 * see them (#398). The key that opens the find bar mounts the rest in batches
 * and the page lets go again when the reader has gone quiet. These are the
 * decisions that make up.
 */

const key = (k: string, mods: { ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean } = {}) => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  ...mods,
})

describe('startsFind', () => {
  const cases = [
    { name: 'Ctrl+F', e: key('f', { ctrlKey: true }), want: true },
    { name: 'Cmd+F', e: key('f', { metaKey: true }), want: true },
    { name: 'Ctrl+Shift+F (key is upper case)', e: key('F', { ctrlKey: true }), want: true },
    { name: 'Ctrl+G is the next match', e: key('g', { ctrlKey: true }), want: true },
    { name: 'F3', e: key('F3'), want: true },
    { name: 'plain f folds a file, it is not a search', e: key('f'), want: false },
    { name: 'Ctrl+Alt+F is not the browser find', e: key('f', { ctrlKey: true, altKey: true }), want: false },
    { name: 'Ctrl+P', e: key('p', { ctrlKey: true }), want: false },
    { name: 'plain /', e: key('/'), want: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(startsFind(c.e), c.want)
    })
  }
})

describe('endsFind', () => {
  const cases = [
    { name: 'Escape', e: key('Escape'), want: true },
    { name: 'Ctrl+Escape is not it', e: key('Escape', { ctrlKey: true }), want: false },
    { name: 'Enter', e: key('Enter'), want: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(endsFind(c.e), c.want)
    })
  }
})

describe('findExpired', () => {
  const cases = [
    { name: 'just active', now: 1000, last: 1000, want: false },
    { name: 'one millisecond short', now: FIND_QUIET_MS - 1, last: 0, want: false },
    { name: 'exactly the quiet period', now: FIND_QUIET_MS, last: 0, want: true },
    { name: 'long after', now: 10 * FIND_QUIET_MS, last: 0, want: true },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(findExpired(c.now, c.last), c.want)
    })
  }
})

describe('nextFindBatch', () => {
  const order = ['a', 'b', 'c', 'd', 'e', 'f', 'g']
  const cases = [
    { name: 'nearest to the anchor first, alternating sides', have: [], anchor: 3, size: 3, want: ['d', 'c', 'e'] },
    { name: 'skips what is already mounted', have: ['d', 'c'], anchor: 3, size: 2, want: ['e', 'b'] },
    { name: 'from the top', have: [], anchor: 0, size: 3, want: ['a', 'b', 'c'] },
    { name: 'from the bottom', have: [], anchor: 6, size: 3, want: ['g', 'f', 'e'] },
    { name: 'an anchor past the end is clamped', have: [], anchor: 99, size: 2, want: ['g', 'f'] },
    { name: 'a negative anchor is clamped', have: [], anchor: -1, size: 2, want: ['a', 'b'] },
    { name: 'fewer left than the batch', have: ['a', 'b', 'c', 'd', 'e'], anchor: 2, size: 4, want: ['f', 'g'] },
    { name: 'all mounted: nothing left', have: order, anchor: 2, size: 4, want: [] },
    { name: 'zero size', have: [], anchor: 2, size: 0, want: [] },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.deepEqual(nextFindBatch(order, new Set(c.have), c.anchor, c.size), c.want)
    })
  }

  it('is empty for an empty review', () => {
    assert.deepEqual(nextFindBatch([], new Set(), 0), [])
  })

  it('mounts a whole review in ceil(n / FIND_BATCH) batches without repeats', () => {
    const big = Array.from({ length: 400 }, (_, i) => `k${i}`)
    const have = new Set<string>()
    let batches = 0
    for (;;) {
      const keys = nextFindBatch(big, have, 123)
      if (keys.length === 0) break
      assert.ok(keys.length <= FIND_BATCH)
      for (const k of keys) {
        assert.ok(!have.has(k), `${k} offered twice`)
        have.add(k)
      }
      batches++
    }
    assert.equal(have.size, 400)
    assert.equal(batches, Math.ceil(400 / FIND_BATCH))
  })
})

describe('mountedWithFind', () => {
  it('answers the lazy set itself when the search holds nothing', () => {
    const lazy = new Set(['a'])
    assert.equal(mountedWithFind(lazy, new Set()), lazy)
  })

  it('is the union otherwise, and leaves the lazy set alone', () => {
    const lazy = new Set(['a', 'b'])
    const got = mountedWithFind(lazy, new Set(['b', 'c']))
    assert.deepEqual([...got].sort(), ['a', 'b', 'c'])
    assert.deepEqual([...lazy].sort(), ['a', 'b'])
  })
})
