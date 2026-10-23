import test from 'node:test'
import assert from 'node:assert/strict'

import {
  openThreadCount,
  replyFields,
  setThreadResolved,
  threadCount,
  threadsOf,
  withoutComment,
} from '../src/threads'
import { openCounts } from '../src/openCounts'
import type { Comment } from '../src/types'

/** comment builds a comment with only what a case cares about filled in. */
function comment(c: Partial<Comment> & { id: string }): Comment {
  return {
    group: 'api',
    diffId: 'd1',
    fileId: 'f1',
    path: 'a.go',
    side: 'new',
    startLine: 3,
    endLine: 3,
    body: c.id,
    snippet: 'x := 1',
    resolved: false,
    createdAt: '',
    updatedAt: '',
    ...c,
  }
}

/** shape writes threads as "root[reply reply] root[]". */
function shape(comments: Comment[]): string {
  return threadsOf(comments)
    .map((t) => `${t.root.id}[${t.replies.map((r) => r.id).join(' ')}]`)
    .join(' ')
}

test('threadsOf puts replies under their comment, in the order they came', () => {
  const list = [
    comment({ id: 'c1' }),
    comment({ id: 'c2' }),
    comment({ id: 'c3', replyTo: 'c1' }),
    comment({ id: 'c4', replyTo: 'c2' }),
    comment({ id: 'c5', replyTo: 'c1' }),
  ]
  assert.equal(shape(list), 'c1[c3 c5] c2[c4]')
  assert.equal(threadCount(list), 2)
})

test('a reply whose comment is gone stands on its own', () => {
  assert.equal(shape([comment({ id: 'c1' }), comment({ id: 'c3', replyTo: 'c2' })]), 'c1[] c3[]')
})

test('open threads are counted once, however many replies they have', () => {
  const list = [
    comment({ id: 'c1' }),
    comment({ id: 'c2', replyTo: 'c1' }),
    comment({ id: 'c3', resolved: true }),
    comment({ id: 'c4', replyTo: 'c3', resolved: true }),
  ]
  assert.equal(openThreadCount(list), 1)
  // The sidebar's counts agree with the header's.
  assert.equal(openCounts(list).totals.get('d1'), 1)
})

test('a reply takes the place and state of its thread, and names its first comment', () => {
  const list = [
    comment({ id: 'c1', path: 'b.go', startLine: 7, endLine: 9, side: 'new', resolved: true }),
    comment({ id: 'c2', replyTo: 'c1', resolved: true }),
  ]
  const fields = replyFields(list, 'c2')
  assert.deepEqual(fields, {
    replyTo: 'c1',
    diffId: 'd1',
    fileId: 'f1',
    path: 'b.go',
    side: 'new',
    startLine: 7,
    endLine: 9,
    snippet: 'x := 1',
    resolved: true,
  })
  assert.equal(replyFields(list, 'c9'), undefined)
})

test('resolving any comment of a thread resolves the thread', () => {
  const list = [comment({ id: 'c1' }), comment({ id: 'c2', replyTo: 'c1' }), comment({ id: 'c3' })]
  const resolved = setThreadResolved(list, 'c2', true, 'now')
  assert.deepEqual(
    resolved.map((c) => `${c.id}:${c.resolved}`),
    ['c1:true', 'c2:true', 'c3:false'],
  )
  assert.deepEqual(
    setThreadResolved(resolved, 'c1', false, 'later').map((c) => c.resolved),
    [false, false, false],
  )
})

test('deleting a comment takes its replies, deleting a reply takes only it', () => {
  const list = [comment({ id: 'c1' }), comment({ id: 'c2', replyTo: 'c1' }), comment({ id: 'c3' })]
  assert.deepEqual(
    withoutComment(list, 'c1').map((c) => c.id),
    ['c3'],
  )
  assert.deepEqual(
    withoutComment(list, 'c2').map((c) => c.id),
    ['c1', 'c3'],
  )
})
