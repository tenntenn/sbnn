import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { eventScope } from '../src/api'
import { LoadOrder } from '../src/loadOrder'

/**
 * A change event says what it touched (#403). The page fetches only the
 * comments for a comments-only event, and everything for any other, so every
 * doubtful case here must come out as "everything".
 */
describe('eventScope', () => {
  const cases = [
    { name: 'a comment event', raw: '{"type":"change","group":"g","scope":"comments"}', want: { scope: 'comments' } },
    { name: 'a change without a scope', raw: '{"type":"change","group":"g"}', want: { scope: undefined } },
    { name: 'a scope this page does not know', raw: '{"type":"change","group":"g","scope":"hooks"}', want: { scope: undefined } },
    { name: 'a change of every group', raw: '{"type":"change","group":"","scope":"comments"}', want: { scope: 'comments' } },
    { name: 'another group', raw: '{"type":"change","group":"other","scope":"comments"}', want: undefined },
    { name: 'a review', raw: '{"type":"review","group":"g","scope":"comments"}', want: { scope: undefined } },
    { name: 'a review of another group', raw: '{"type":"review","group":"other"}', want: undefined },
    { name: 'a type it does not follow', raw: '{"type":"ping","group":"g"}', want: undefined },
    { name: 'a message that is not JSON', raw: 'nope', want: { scope: undefined } },
  ]
  for (const tc of cases) {
    it(tc.name, () => {
      assert.deepEqual(eventScope(tc.raw, 'g'), tc.want)
    })
  }
})

describe('LoadOrder', () => {
  it('lets a lone full reload apply everything', () => {
    const order = new LoadOrder()
    const t = order.begin(true)
    assert.equal(t.freshComments(), true)
    assert.equal(t.freshDiffs(), true)
  })

  it('never lets a comments-only reload apply diffs', () => {
    const t = new LoadOrder().begin(false)
    assert.equal(t.freshComments(), true)
    assert.equal(t.freshDiffs(), false)
  })

  it('drops the comments of a full reload that a comments reload overtook, and keeps its diffs', () => {
    const order = new LoadOrder()
    const full = order.begin(true)
    const comments = order.begin(false)
    assert.equal(full.freshComments(), false)
    assert.equal(full.freshDiffs(), true)
    assert.equal(comments.freshComments(), true)
  })

  it('drops everything of an older full reload when a newer full one started', () => {
    const order = new LoadOrder()
    const first = order.begin(true)
    const second = order.begin(true)
    assert.equal(first.freshComments(), false)
    assert.equal(first.freshDiffs(), false)
    assert.equal(second.freshDiffs(), true)
  })

  it('drops an older comments reload when a full one started after it', () => {
    const order = new LoadOrder()
    const comments = order.begin(false)
    const full = order.begin(true)
    assert.equal(comments.freshComments(), false)
    assert.equal(full.freshComments(), true)
    assert.equal(full.freshDiffs(), true)
  })
})
