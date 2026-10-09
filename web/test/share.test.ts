import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { shareComments, shareDiffs, shareGroups } from '../src/share'
import type { Comment, Diff } from '../src/types'

/**
 * #376: every SSE event reloads the whole group, and the page used to hand
 * every file section brand new objects, so one new comment rendered all
 * 400 files again (670 ms of main thread, measured). These pin the property
 * the page now relies on to skip that: what did not change is the same object.
 */

function diff(id: string, over: Partial<Diff> = {}): Diff {
  return { id, title: id, baseDir: '', createdAt: 't', raw: 'x'.repeat(10), files: [], ...over }
}

function comment(id: string, over: Partial<Comment> = {}): Comment {
  return {
    id,
    group: 'g',
    diffId: 'd1',
    fileId: 'f1',
    path: 'a.go',
    side: 'new',
    startLine: 1,
    endLine: 1,
    body: 'b',
    snippet: '',
    ...over,
  } as Comment
}

// A reload parses fresh objects, so equal content is never the same object.
const clone = <T>(x: T): T => JSON.parse(JSON.stringify(x))

describe('shareDiffs', () => {
  const cases: { name: string; prev: Diff[]; next: Diff[]; same: boolean[]; wholeSame: boolean }[] = [
    { name: 'nothing changed', prev: [diff('d1'), diff('d2')], next: [diff('d1'), diff('d2')], same: [true, true], wholeSame: true },
    { name: 'a round arrived', prev: [diff('d1')], next: [diff('d1'), diff('d2')], same: [true, false], wholeSame: false },
    { name: 'a round was deleted', prev: [diff('d1'), diff('d2')], next: [diff('d2')], same: [true], wholeSame: false },
    { name: 'a round changed under its id', prev: [diff('d1')], next: [diff('d1', { title: 'other' })], same: [false], wholeSame: false },
    { name: 'a round grew a file', prev: [diff('d1')], next: [diff('d1', { files: [{ id: 'f' } as never] })], same: [false], wholeSame: false },
    { name: 'empty to empty', prev: [], next: [], same: [], wholeSame: true },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const prev = c.prev
      const next = clone(c.next)
      const got = shareDiffs(prev, next)
      assert.deepEqual(got, c.next)
      got.forEach((d, i) => {
        const old = prev.find((p) => p.id === d.id)
        assert.equal(d === old, c.same[i], `diff ${d.id}`)
      })
      assert.equal(got === prev, c.wholeSame, 'the previous array is returned when nothing changed')
    })
  }
})

describe('shareComments', () => {
  const cases: { name: string; prev: Comment[]; next: Comment[]; same: boolean[]; wholeSame: boolean }[] = [
    { name: 'nothing changed', prev: [comment('c1'), comment('c2')], next: [comment('c1'), comment('c2')], same: [true, true], wholeSame: true },
    { name: 'one comment added', prev: [comment('c1')], next: [comment('c1'), comment('c2')], same: [true, false], wholeSame: false },
    { name: 'one comment edited', prev: [comment('c1'), comment('c2')], next: [comment('c1'), comment('c2', { body: 'edited' })], same: [true, false], wholeSame: false },
    { name: 'one comment deleted', prev: [comment('c1'), comment('c2')], next: [comment('c2')], same: [true], wholeSame: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const next = clone(c.next)
      const got = shareComments(c.prev, next)
      assert.deepEqual(got, c.next)
      got.forEach((x, i) => {
        const old = c.prev.find((p) => p.id === x.id)
        assert.equal(x === old, c.same[i], `comment ${x.id}`)
      })
      assert.equal(got === c.prev, c.wholeSame)
    })
  }
})

describe('shareGroups', () => {
  it('keeps the list of a section whose comments did not change', () => {
    const a = comment('c1')
    const b = comment('c2')
    const prev = new Map([['k1', [a]], ['k2', [b]]])
    const next = new Map([['k1', [a]], ['k2', [b, comment('c3')]]])
    const got = shareGroups(prev, next)
    assert.equal(got.get('k1'), prev.get('k1'))
    assert.notEqual(got.get('k2'), prev.get('k2'))
    assert.equal(got.get('k2')?.length, 2)
  })
})

/**
 * The guard on cost: with 400 files and 300 comments, reloading after one more
 * comment must reuse every file, every diff and every other comment, so the
 * number of objects React sees as new is the one comment, not thousands.
 */
describe('a reload after one new comment', () => {
  it('replaces one comment and nothing else', () => {
    const diffs = Array.from({ length: 20 }, (_, i) =>
      diff(`d${i}`, { files: Array.from({ length: 20 }, (_, j) => ({ id: `f${j}` }) as never) }),
    )
    const comments = Array.from({ length: 300 }, (_, i) => comment(`c${i}`, { diffId: `d${i % 20}` }))
    const nextComments = [...clone(comments), comment('new')]
    const gotDiffs = shareDiffs(diffs, clone(diffs))
    const gotComments = shareComments(comments, nextComments)
    assert.equal(gotDiffs, diffs)
    assert.equal(gotComments.filter((c, i) => c !== comments[i]).length, 1)
  })
})
