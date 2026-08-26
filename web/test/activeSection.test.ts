import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { nextActive } from '../src/activeSection'

/**
 * The bug: clicking a file in the sidebar selected the file below it.
 *
 * The diff pane decides which file is being read from one
 * IntersectionObserver whose root margin leaves only a band at the top of the
 * pane sensitive, and it took the last file in order that reached into that
 * band - the right answer while scrolling down, since the lower of two files
 * either side of a boundary is the one being scrolled into. Jumping to a file
 * puts its header at the top of that band, so a file shorter than the band
 * leaves the next file's section inside it too, and the jump was immediately
 * overruled by the file below.
 *
 * Measured in Chromium against the test/visual fixture, before the fix:
 * clicking file 0 left .file-item.active on file 1, clicking file 1 left it on
 * file 2, and clicking file 3 - the first file tall enough to fill the band by
 * itself - was correct. Waiting before the click changed nothing, so this is
 * not the settling race it was first taken for.
 */

const order = ['d1:a', 'd1:b', 'd1:c', 'd1:d']

describe('nextActive', () => {
  it('takes the last file in the band while scrolling, with no jump in force', () => {
    const got = nextActive(order, new Set(['d1:b', 'd1:c']), null)
    assert.deepEqual(got, { key: 'd1:c', jumpedTo: null })
  })

  it('keeps the file that was jumped to, even when the next one reaches into the band', () => {
    // The regression: 'd1:b' is the file that was clicked and 'd1:c' starts
    // inside the band below it. Without the jump this returns 'd1:c'.
    const got = nextActive(order, new Set(['d1:b', 'd1:c']), 'd1:b')
    assert.deepEqual(got, { key: 'd1:b', jumpedTo: 'd1:b' })
  })

  it('holds the jump for the first file, which is where the defect showed', () => {
    const got = nextActive(order, new Set(['d1:a', 'd1:b']), 'd1:a')
    assert.deepEqual(got, { key: 'd1:a', jumpedTo: 'd1:a' })
  })

  it('lets go of the jump once the reader has scrolled off that file', () => {
    const got = nextActive(order, new Set(['d1:c', 'd1:d']), 'd1:a')
    assert.deepEqual(got, { key: 'd1:d', jumpedTo: null })
  })

  it('falls back to the first file when the band holds none of them', () => {
    assert.deepEqual(nextActive(order, new Set(), null), { key: 'd1:a', jumpedTo: null })
    assert.deepEqual(nextActive(order, new Set(), 'd1:c'), { key: 'd1:a', jumpedTo: null })
  })

  it('answers null for an empty stack rather than throwing', () => {
    assert.deepEqual(nextActive([], new Set(), null), { key: null, jumpedTo: null })
  })

  it('reports a jump to a file that is no longer in the stack as let go', () => {
    const got = nextActive(order, new Set(['d1:b']), 'gone')
    assert.deepEqual(got, { key: 'd1:b', jumpedTo: null })
  })
})
