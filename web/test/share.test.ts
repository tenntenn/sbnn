import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { sameElements, shareComments, shareDiffs } from '../src/share'
import type { Comment, Diff, FileDiff } from '../src/types'

/**
 * #376: every SSE event reloads the whole group, and the page used to hand
 * every file section brand new objects, so one new comment rendered all
 * 400 files again (670 ms of main thread, measured). These pin the property
 * the page now relies on to skip that - what did not change is the same
 * object - and the other half: what did change is never the stale one.
 */

function file(id: string, over: Partial<FileDiff> = {}): FileDiff {
  return {
    id,
    oldPath: id,
    newPath: id,
    status: 'modified',
    isBinary: false,
    additions: 1,
    deletions: 0,
    viewMode: 'split',
    isMarkdown: false,
    isImage: false,
    isNotebook: false,
    hunks: [{ header: '@@', oldStart: 1, oldLines: 1, newStart: 1, newLines: 1, lines: [] }],
    ...over,
  }
}

function diff(id: string, over: Partial<Diff> = {}): Diff {
  return { id, title: id, baseDir: '', createdAt: 't', raw: 'x'.repeat(10), files: [file('f1')], ...over }
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
    resolved: false,
    createdAt: 't',
    updatedAt: 't',
    ...over,
  }
}

// A reload parses fresh objects, so equal content is never the same object.
const clone = <T>(x: T): T => JSON.parse(JSON.stringify(x))

describe('shareDiffs', () => {
  const cases: { name: string; prev: Diff[]; next: Diff[]; same: boolean[]; wholeSame: boolean }[] = [
    { name: 'nothing changed', prev: [diff('d1'), diff('d2')], next: [diff('d1'), diff('d2')], same: [true, true], wholeSame: true },
    { name: 'a round arrived', prev: [diff('d1')], next: [diff('d1'), diff('d2')], same: [true, false], wholeSame: false },
    { name: 'a round was deleted', prev: [diff('d1'), diff('d2')], next: [diff('d2')], same: [true], wholeSame: false },
    { name: 'a round changed title', prev: [diff('d1')], next: [diff('d1', { title: 'other' })], same: [false], wholeSame: false },
    { name: 'a round grew a file', prev: [diff('d1')], next: [diff('d1', { files: [file('f1'), file('f2')] })], same: [false], wholeSame: false },
    { name: 'empty to empty', prev: [], next: [], same: [], wholeSame: true },
    // Same id, title, raw length and file count: only a field of the file differs.
    { name: 'a file is folded now', prev: [diff('d1')], next: [diff('d1', { files: [file('f1', { folded: true })] })], same: [false], wholeSame: false },
    { name: 'an image became available', prev: [diff('d1', { files: [file('f1', { imageStatus: 'missing' })] })], next: [diff('d1', { files: [file('f1', { imageStatus: 'ok' })] })], same: [false], wholeSame: false },
    { name: 'an image size changed', prev: [diff('d1', { files: [file('f1', { imageSize: 1 })] })], next: [diff('d1', { files: [file('f1', { imageSize: 2 })] })], same: [false], wholeSame: false },
    { name: 'a file status changed', prev: [diff('d1')], next: [diff('d1', { files: [file('f1', { status: 'renamed' })] })], same: [false], wholeSame: false },
    { name: 'a file path changed', prev: [diff('d1')], next: [diff('d1', { files: [file('f1', { newPath: 'g' })] })], same: [false], wholeSame: false },
    { name: 'counts changed', prev: [diff('d1')], next: [diff('d1', { files: [file('f1', { additions: 5, deletions: 2 })] })], same: [false], wholeSame: false },
    { name: 'hunks went away', prev: [diff('d1')], next: [diff('d1', { files: [file('f1', { hunks: null })] })], same: [false], wholeSame: false },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const next = clone(c.next)
      const got = shareDiffs(c.prev, next)
      assert.deepEqual(got, c.next)
      got.forEach((d, i) => {
        const old = c.prev.find((p) => p.id === d.id)
        assert.equal(d === old, c.same[i], `diff ${d.id}`)
      })
      assert.equal(got === c.prev, c.wholeSame, 'the previous array is returned when nothing changed')
    })
  }
})

describe('shareComments', () => {
  const cases: { name: string; prev: Comment[]; next: Comment[]; same: boolean[]; wholeSame: boolean }[] = [
    { name: 'nothing changed', prev: [comment('c1'), comment('c2')], next: [comment('c1'), comment('c2')], same: [true, true], wholeSame: true },
    { name: 'one comment added', prev: [comment('c1')], next: [comment('c1'), comment('c2')], same: [true, false], wholeSame: false },
    { name: 'one comment deleted', prev: [comment('c1'), comment('c2')], next: [comment('c2')], same: [true], wholeSame: false },
    { name: 'body edited', prev: [comment('c1'), comment('c2')], next: [comment('c1'), comment('c2', { body: 'edited' })], same: [true, false], wholeSame: false },
    { name: 'resolved', prev: [comment('c1')], next: [comment('c1', { resolved: true })], same: [false], wholeSame: false },
    { name: 'updatedAt moved', prev: [comment('c1')], next: [comment('c1', { updatedAt: 'later' })], same: [false], wholeSame: false },
    { name: 'turned into a question', prev: [comment('c1')], next: [comment('c1', { question: true })], same: [false], wholeSame: false },
    { name: 'lines moved', prev: [comment('c1')], next: [comment('c1', { startLine: 2, endLine: 3 })], same: [false], wholeSame: false },
    { name: 'suggestion changed', prev: [comment('c1', { suggestions: ['a'] })], next: [comment('c1', { suggestions: ['b'] })], same: [false], wholeSame: false },
    { name: 'same suggestions', prev: [comment('c1', { suggestions: ['a', 'b'] })], next: [comment('c1', { suggestions: ['a', 'b'] })], same: [true], wholeSame: true },
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

describe('sameElements', () => {
  const a = comment('c1')
  const b = comment('c2')
  const cases = [
    { name: 'same objects', x: [a, b], y: [a, b], want: true },
    { name: 'equal but not the same objects', x: [a], y: [comment('c1')], want: false },
    { name: 'different order', x: [a, b], y: [b, a], want: false },
    { name: 'different length', x: [a], y: [a, b], want: false },
    { name: 'both empty', x: [], y: [], want: true },
  ]
  for (const c of cases) {
    it(c.name, () => assert.equal(sameElements(c.x, c.y), c.want))
  }
})

/**
 * The guard on cost: with 400 files and 300 comments, reloading after one more
 * comment must reuse every file, every diff and every other comment, so the
 * number of objects React sees as new is the one comment, not thousands.
 */
describe('a reload after one new comment', () => {
  it('replaces one comment and nothing else', () => {
    const diffs = Array.from({ length: 20 }, (_, i) =>
      diff(`d${i}`, { files: Array.from({ length: 20 }, (_, j) => file(`f${j}`)) }),
    )
    const comments = Array.from({ length: 300 }, (_, i) => comment(`c${i}`, { diffId: `d${i % 20}` }))
    const nextComments = [...clone(comments), comment('new')]
    assert.equal(shareDiffs(diffs, clone(diffs)), diffs)
    const got = shareComments(comments, nextComments)
    assert.equal(got.filter((c, i) => c !== comments[i]).length, 1)
  })
})
