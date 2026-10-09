import test from 'node:test'
import assert from 'node:assert/strict'

import { COMMENT_ID_PREFIX, PREVIEW_COMMENT_ID_PREFIX, commentDomIds, diffSlots } from '../src/previewComments'

/**
 * A comment arriving must not rebuild the slots that already hold a thread,
 * or the thread loses a reply or an edit its reader has half written. diffSlots
 * is the decision of which slots to touch.
 */
const cases = [
  { name: 'nothing to nothing', have: [] as number[], want: [] as number[], add: [] as number[], remove: [] as number[] },
  { name: 'a comment in another block adds one slot and keeps the first', have: [2], want: [2, 7], add: [7], remove: [] },
  { name: 'a second comment in a block with a slot changes nothing', have: [2], want: [2], add: [], remove: [] },
  { name: 'a deleted comment drops only its own slot', have: [1, 2, 3], want: [1, 3], add: [], remove: [2] },
  { name: 'moved comments add and remove', have: [1], want: [4, 2], add: [2, 4], remove: [1] },
]

for (const c of cases) {
  test(`diffSlots: ${c.name}`, () => {
    assert.deepEqual(diffSlots(c.have, c.want), { add: c.add, remove: c.remove })
  })
}

test('the two panes name a comment differently, and both are known', () => {
  assert.notEqual(COMMENT_ID_PREFIX, PREVIEW_COMMENT_ID_PREFIX)
  assert.deepEqual(commentDomIds('c1'), ['comment-c1', 'preview-comment-c1'])
})
