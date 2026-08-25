import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { stepToComment, type CommentStop } from '../src/shortcuts'

// The review of issue #61: three comments on file A, one on file B.
const stops: CommentStop[] = [
  { id: 'c1', key: 'A' },
  { id: 'c2', key: 'A' },
  { id: 'c3', key: 'A' },
  { id: 'c4', key: 'B' },
]

/**
 * press walks the tour the way the page does: each press steps from the
 * comment the last press landed on, and the page moves the active file to
 * the one that comment sits in.
 */
function press(by: number, times: number, startKey: string | null = 'A'): string[] {
  let current: string | null = null
  let activeKey = startKey
  const visited: string[] = []
  for (let i = 0; i < times; i++) {
    const target = stepToComment(stops, current, activeKey, by)
    if (!target) break
    current = target.id
    activeKey = target.key
    visited.push(target.id)
  }
  return visited
}

describe('stepToComment', () => {
  // The bug: `at` was the index of the first comment in the active file, and
  // arriving at the second comment of that file left the reader in the file
  // the search started from. So `n` landed on c2 and stayed there, and c3 and
  // c4 could not be reached with the keyboard at all.
  it('reaches every comment of a file that holds three', () => {
    assert.deepEqual(press(1, 6), ['c1', 'c2', 'c3', 'c4', 'c1', 'c2'])
  })

  it('walks backwards through them too', () => {
    assert.deepEqual(press(-1, 6), ['c3', 'c2', 'c1', 'c4', 'c3', 'c2'])
  })

  it('has nowhere to go when there are no open comments', () => {
    assert.equal(stepToComment([], null, 'A', 1), null)
    assert.equal(stepToComment([], 'c1', 'A', -1), null)
  })

  it('rejoins at the file being read when nothing has been visited', () => {
    assert.deepEqual(stepToComment(stops, null, 'B', 1), { id: 'c4', key: 'B' })
    assert.deepEqual(stepToComment(stops, null, 'B', -1), { id: 'c4', key: 'B' })
    assert.deepEqual(stepToComment(stops, null, 'A', -1), { id: 'c3', key: 'A' })
  })

  it('rejoins at the whole review when the file being read has no comments', () => {
    assert.deepEqual(stepToComment(stops, null, 'Z', 1), { id: 'c1', key: 'A' })
    assert.deepEqual(stepToComment(stops, null, 'Z', -1), { id: 'c4', key: 'B' })
  })

  // Resolving a comment takes it off the tour, so the recorded position is a
  // comment that is no longer in the list.
  it('rejoins when the comment it was standing on was resolved away', () => {
    assert.deepEqual(stepToComment(stops, 'gone', 'A', 1), { id: 'c1', key: 'A' })
    assert.deepEqual(stepToComment(stops, 'gone', 'B', -1), { id: 'c4', key: 'B' })
  })

  it('wraps around both ends', () => {
    assert.deepEqual(stepToComment(stops, 'c4', null, 1), { id: 'c1', key: 'A' })
    assert.deepEqual(stepToComment(stops, 'c1', null, -1), { id: 'c4', key: 'B' })
  })

  // Regression: the recorded comment used to be honoured only while it sat in
  // the active file. activeKey is not the reader's alone - the page re-centres
  // the comment it just jumped to, and the observer that picks the active file
  // ignores the top 70% of the viewport - so a comment near the top of its
  // file can hand activeKey back to the file before it. When that happened the
  // position was thrown away and the next press rejoined at the earlier file,
  // walking backwards.
  it('keeps its place when the active file drifts underneath it', () => {
    const drifted: CommentStop[] = [
      { id: 'a1', key: 'A' },
      { id: 'b1', key: 'B' },
      { id: 'b2', key: 'B' },
    ]
    // The reader pressed n and landed on b1; the observer then said "A".
    assert.deepEqual(stepToComment(drifted, 'b1', 'A', 1), { id: 'b2', key: 'B' })
    assert.deepEqual(stepToComment(drifted, 'b1', 'A', -1), { id: 'a1', key: 'A' })
  })

  it('steps from the recorded comment with no active file at all', () => {
    assert.deepEqual(stepToComment(stops, 'c2', null, 1), { id: 'c3', key: 'A' })
    assert.deepEqual(stepToComment(stops, 'c2', null, -1), { id: 'c1', key: 'A' })
  })

  it('stands still on a review with exactly one comment', () => {
    const one: CommentStop[] = [{ id: 'only', key: 'A' }]
    assert.deepEqual(stepToComment(one, 'only', 'A', 1), { id: 'only', key: 'A' })
    assert.deepEqual(stepToComment(one, 'only', 'A', -1), { id: 'only', key: 'A' })
  })
})
