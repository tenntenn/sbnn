import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  INITIAL_MOUNTED,
  holdsState,
  initialMounted,
  nextMounted,
  placeholderHeight,
  reconcileMounted,
} from '../src/lazySections'

/**
 * Mounting a 400-file review took about 4 s and 280,000 DOM nodes because every
 * section's body was mounted and stayed mounted (#390). A section is now a
 * shell plus a body, and the body is on the page only while the section is
 * within a few viewports of the pane. These are the decisions that rule makes.
 */

const noFacts = { focusWithin: false, hasField: false, hasSelectedRows: false, holdsPreviewSelection: false }
const never = () => false

describe('nextMounted', () => {
  const cases = [
    { name: 'near and unmounted: mount it', was: false, near: true, keep: true, held: false, want: true },
    { name: 'near and mounted: stay', was: true, near: true, keep: true, held: false, want: true },
    { name: 'far and unmounted: stay a shell', was: false, near: false, keep: false, held: false, want: false },
    { name: 'in the keep band only and unmounted: not mounted from the far side', was: false, near: false, keep: true, held: false, want: false },
    { name: 'in the keep band only and mounted: hysteresis keeps it', was: true, near: false, keep: true, held: false, want: true },
    { name: 'beyond the keep band: unmount', was: true, near: false, keep: false, held: false, want: false },
    { name: 'beyond the keep band but holding a draft: keep', was: true, near: false, keep: false, held: true, want: true },
    { name: 'holding state does not mount a shell by itself', was: false, near: false, keep: false, held: true, want: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(nextMounted(c.was, c.near, c.keep, c.held), c.want)
    })
  }
})

describe('holdsState', () => {
  const cases = [
    { name: 'nothing', facts: noFacts, want: false },
    { name: 'focus inside', facts: { ...noFacts, focusWithin: true }, want: true },
    { name: 'an open text field, focused or not', facts: { ...noFacts, hasField: true }, want: true },
    { name: 'selected rows, the start of a comment', facts: { ...noFacts, hasSelectedRows: true }, want: true },
    { name: "sbnn's preview selection", facts: { ...noFacts, holdsPreviewSelection: true }, want: true },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(holdsState(c.facts), c.want)
    })
  }
})

describe('initialMounted', () => {
  it('mounts the first few sections so the first paint is not empty', () => {
    const order = Array.from({ length: 50 }, (_, i) => `k${i}`)
    const got = initialMounted(order)
    assert.equal(got.size, INITIAL_MOUNTED)
    assert.ok(got.has('k0'))
    assert.ok(!got.has(`k${INITIAL_MOUNTED}`))
  })

  it('copes with a review shorter than that, or empty', () => {
    assert.deepEqual([...initialMounted(['a', 'b'])], ['a', 'b'])
    assert.equal(initialMounted([]).size, 0)
  })
})

describe('reconcileMounted', () => {
  const order = ['a', 'b', 'c', 'd', 'e', 'f']

  it('mounts what came near and unmounts what left the keep band', () => {
    const got = reconcileMounted(new Set(['a', 'b']), order, new Set(['c', 'd']), new Set(['b', 'c', 'd', 'e']), never)
    assert.deepEqual([...got].sort(), ['b', 'c', 'd'])
  })

  it('returns the very same set when nothing changed, so React skips the render', () => {
    const previous = new Set(['b', 'c'])
    const got = reconcileMounted(previous, order, new Set(['b', 'c']), new Set(['a', 'b', 'c', 'd']), never)
    assert.equal(got, previous)
  })

  it('never takes a held section away, wherever the reader is', () => {
    const got = reconcileMounted(new Set(['a', 'e']), order, new Set(['e']), new Set(['e']), (key) => key === 'a')
    assert.deepEqual([...got].sort(), ['a', 'e'])
  })

  it('lets a held section go once it holds nothing any more', () => {
    const got = reconcileMounted(new Set(['a', 'e']), order, new Set(['e']), new Set(['e']), never)
    assert.deepEqual([...got], ['e'])
  })

  it('does not ask whether a section is held unless it would be unmounted', () => {
    const asked: string[] = []
    reconcileMounted(new Set(['c']), order, new Set(['c']), new Set(['c']), (key) => {
      asked.push(key)
      return false
    })
    assert.deepEqual(asked, [])
  })

  it('drops keys that are no longer in the review', () => {
    const got = reconcileMounted(new Set(['gone', 'a']), order, new Set(['a']), new Set(['a']), never)
    assert.deepEqual([...got], ['a'])
  })

  it('keeps reading order of the review, not of the previous set', () => {
    const got = reconcileMounted(new Set(), order, new Set(['d', 'b']), new Set(['b', 'd']), never)
    assert.deepEqual([...got], ['b', 'd'])
  })
})

describe('placeholderHeight', () => {
  const cases = [
    { name: 'what the body measured, so unmounting moves nothing', measured: 640, estimate: 400, want: 640 },
    { name: 'the estimate before it was ever measured', measured: undefined, estimate: 400, want: 400 },
    { name: 'a zero measurement is not a measurement', measured: 0, estimate: 400, want: 400 },
    { name: 'never negative', measured: undefined, estimate: -5, want: 0 },
  ]
  for (const c of cases) {
    it(c.name, () => {
      assert.equal(placeholderHeight(c.measured, c.estimate), c.want)
    })
  }
})
