import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { shortcuts, stepToComment, type CommentStop } from '../src/shortcuts'

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

/**
 * The list is what the ? overlay draws, so a row that has gone stale is the
 * product telling the reader something that stopped being true. #291 widened
 * the / search from paths to paths and diff lines - the box says "Search
 * paths and lines ( / )" - while this row went on saying "Filter the file
 * list by path".
 */
describe('the shortcut list', () => {
  const rowFor = (key: string) => shortcuts.find((s) => s.keys.includes(key))

  it('describes the / search as reading the lines, not only the paths', () => {
    const row = rowFor('/')
    assert.ok(row, 'no row answers to /')
    assert.match(
      row.what,
      /line/i,
      `the / row says nothing about lines: ${JSON.stringify(row.what)}`,
    )
    assert.doesNotMatch(
      row.what,
      /by path/i,
      `the / row still describes the filter #291 replaced: ${JSON.stringify(row.what)}`,
    )
  })

  // #68 made the line gutter a stop in the tab order that answers Enter and
  // Space, and #318 is that nothing said so: the sheet is where a reader
  // looks, the README described commenting as clicking, and the gutter cell
  // announces itself only once focus is already on it. Asserted by the claim
  // rather than the wording - a row that mentions a line and one of the two
  // keys - so rephrasing the sentence is free and dropping it is not.
  it('says how to start a comment from the keyboard', () => {
    const row = shortcuts.find((s) => s.keys.includes('Enter') || s.keys.includes('Space'))
    assert.ok(row, 'no row covers the Enter/Space press that starts a comment on the focused line')
    assert.match(
      row.what,
      /line/i,
      `the Enter/Space row does not say it is about a line: ${JSON.stringify(row.what)}`,
    )
    assert.match(
      row.what,
      /comment/i,
      `the Enter/Space row does not say it starts a comment: ${JSON.stringify(row.what)}`,
    )
  })

  it('explains every key it answers to', () => {
    for (const row of shortcuts) {
      assert.ok(row.keys.length > 0, `a row has no keys: ${JSON.stringify(row)}`)
      assert.ok(row.what.trim().length > 0, `${row.keys.join('/')} has no explanation`)
    }
  })

  it('answers to each key once', () => {
    const seen = shortcuts.flatMap((row) => row.keys)
    assert.deepEqual([...new Set(seen)], seen, 'a key is listed twice')
  })
})

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
