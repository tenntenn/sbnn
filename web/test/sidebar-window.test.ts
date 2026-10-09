import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  DEFAULT_ROW_HEIGHT,
  ROW_CHUNK,
  WINDOW_MIN_ROWS,
  chunkStarts,
  createNear,
  isWindowed,
  placeholderHeight,
} from '../src/sidebarWindow'

/**
 * #404: 3000 file rows were 28,000 DOM nodes that the browser walked on every
 * event. A long list keeps only the chunks of rows near the viewport; these are
 * the rules that decide what a chunk is and how tall it stands when it is gone.
 */

describe('isWindowed', () => {
  const cases = [
    { name: 'a small review is rendered whole', files: 8, want: false },
    { name: 'the limit itself is still whole', files: WINDOW_MIN_ROWS, want: false },
    { name: 'one past the limit is windowed', files: WINDOW_MIN_ROWS + 1, want: true },
    { name: '3000 files are windowed', files: 3000, want: true },
  ]
  for (const c of cases) {
    it(c.name, () => assert.equal(isWindowed(c.files), c.want))
  }
})

describe('chunkStarts', () => {
  const cases = [
    { name: 'no rows, no chunks', count: 0, want: [] },
    { name: 'one row is one chunk', count: 1, want: [0] },
    { name: 'exactly a chunk', count: ROW_CHUNK, want: [0] },
    { name: 'one more row starts another', count: ROW_CHUNK + 1, want: [0, ROW_CHUNK] },
    { name: 'three chunks', count: ROW_CHUNK * 3, want: [0, ROW_CHUNK, ROW_CHUNK * 2] },
  ]
  for (const c of cases) {
    it(c.name, () => assert.deepEqual(chunkStarts(c.count), c.want))
  }

  it('covers every row exactly once', () => {
    const count = 3000
    const starts = chunkStarts(count)
    let covered = 0
    for (const start of starts) covered += Math.min(start + ROW_CHUNK, count) - start
    assert.equal(covered, count)
  })
})

describe('placeholderHeight', () => {
  const cases = [
    { name: 'what the rows measured wins', measured: 700, rows: 25, rowHeight: 29, want: 700 },
    { name: 'never measured: rows at the row height', measured: undefined, rows: 25, rowHeight: 29, want: 725 },
    { name: 'a hidden list measured 0 and says nothing', measured: 0, rows: 25, rowHeight: 29, want: 725 },
    { name: 'a short last chunk', measured: undefined, rows: 3, rowHeight: 29.4, want: 88 },
  ]
  for (const c of cases) {
    it(c.name, () => assert.equal(placeholderHeight(c.measured, c.rows, c.rowHeight), c.want))
  }
})

describe('createNear', () => {
  it('starts from the default row height and learns from a measurement', () => {
    const near = createNear(() => null)
    assert.equal(near.rowHeight, DEFAULT_ROW_HEIGHT)
    near.learn(750, 25)
    assert.equal(near.rowHeight, 30)
  })

  it('ignores a measurement of nothing', () => {
    const near = createNear(() => null)
    near.learn(0, 25)
    near.learn(100, 0)
    assert.equal(near.rowHeight, DEFAULT_ROW_HEIGHT)
  })

  it('without IntersectionObserver a watch is a no-op that can be stopped', () => {
    const near = createNear(() => null)
    const stop = near.watch({} as Element, () => assert.fail('nothing to report'))
    stop()
    near.dispose()
  })
})
