import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { newDiffIds, unreadCount, withUnread } from '../src/notify'

describe('newDiffIds', () => {
  it('treats the first load as nothing new', () => {
    assert.deepEqual(newDiffIds(null, ['a', 'b']), [])
  })
  it('returns only ids not seen before', () => {
    assert.deepEqual(newDiffIds(new Set(['a']), ['a', 'b', 'c']), ['b', 'c'])
  })
  it('returns nothing when a diff was only removed', () => {
    assert.deepEqual(newDiffIds(new Set(['a', 'b']), ['a']), [])
  })
})

describe('unread title', () => {
  it('prefixes and replaces the count', () => {
    assert.equal(withUnread('sbnn', 1), '(1) sbnn')
    assert.equal(withUnread('(1) sbnn', 3), '(3) sbnn')
  })
  it('removes the prefix at zero', () => {
    assert.equal(withUnread('(2) topic · sbnn', 0), 'topic · sbnn')
  })
  it('reads the count back', () => {
    assert.equal(unreadCount('(4) sbnn'), 4)
    assert.equal(unreadCount('sbnn'), 0)
  })
})
