import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { noCounts, openCounts } from '../src/openCounts'
import type { Comment } from '../src/types'

/**
 * #402: the sidebar counted the open comments of each file by filtering the
 * whole comment list once per file. These pin the counts themselves and the
 * property the memoised sidebar rounds rely on: a round whose counts did not
 * change is handed back as the same object.
 */

function comment(id: string, diffId: string, fileId: string, resolved = false): Comment {
  return {
    id,
    group: 'g',
    diffId,
    fileId,
    path: 'a.go',
    side: 'new',
    startLine: 1,
    endLine: 1,
    body: 'b',
    snippet: '',
    resolved,
    createdAt: 't',
    updatedAt: 't',
  }
}

describe('openCounts', () => {
  const cases: {
    name: string
    comments: Comment[]
    files: [string, string, number][]
    totals: [string, number][]
  }[] = [
    { name: 'no comments', comments: [], files: [], totals: [] },
    {
      name: 'counts per file and per round',
      comments: [comment('1', 'd1', 'f1'), comment('2', 'd1', 'f1'), comment('3', 'd1', 'f2'), comment('4', 'd2', 'f1')],
      files: [['d1', 'f1', 2], ['d1', 'f2', 1], ['d2', 'f1', 1]],
      totals: [['d1', 3], ['d2', 1]],
    },
    {
      name: 'resolved comments are not counted',
      comments: [comment('1', 'd1', 'f1', true), comment('2', 'd1', 'f1'), comment('3', 'd2', 'f1', true)],
      files: [['d1', 'f1', 1]],
      totals: [['d1', 1]],
    },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const got = openCounts(c.comments)
      for (const [round, file, n] of c.files) assert.equal(got.byRound.get(round)?.get(file), n)
      assert.equal(
        [...got.byRound.values()].reduce((sum, files) => sum + files.size, 0),
        c.files.length,
      )
      assert.deepEqual([...got.totals].sort(), [...c.totals].sort())
    })
  }

  it('reports a file with no open comment as zero by absence', () => {
    assert.equal(openCounts([comment('1', 'd1', 'f1')]).byRound.get('d1')?.get('f9'), undefined)
    assert.equal(noCounts().size, 0)
  })

  it('returns prev itself when nothing changed', () => {
    const comments = [comment('1', 'd1', 'f1'), comment('2', 'd2', 'f1')]
    const prev = openCounts(comments)
    assert.equal(openCounts(comments.map((c) => ({ ...c })), prev), prev)
  })

  it('keeps the object of a round that did not change', () => {
    const base = [comment('1', 'd1', 'f1'), comment('2', 'd2', 'f1')]
    const prev = openCounts(base)
    const next = openCounts([...base, comment('3', 'd2', 'f2')], prev)
    assert.notEqual(next, prev)
    assert.equal(next.byRound.get('d1'), prev.byRound.get('d1'))
    assert.notEqual(next.byRound.get('d2'), prev.byRound.get('d2'))
    assert.equal(next.byRound.get('d2')?.get('f2'), 1)
  })

  it('does not keep a stale count when a comment is resolved', () => {
    const base = [comment('1', 'd1', 'f1'), comment('2', 'd1', 'f1')]
    const prev = openCounts(base)
    const next = openCounts([base[0], comment('2', 'd1', 'f1', true)], prev)
    assert.equal(next.byRound.get('d1')?.get('f1'), 1)
    assert.equal(next.totals.get('d1'), 1)
  })

  it('drops a round whose last open comment went away', () => {
    const prev = openCounts([comment('1', 'd1', 'f1'), comment('2', 'd2', 'f1')])
    const next = openCounts([comment('2', 'd2', 'f1')], prev)
    assert.equal(next.byRound.has('d1'), false)
    assert.equal(next.byRound.get('d2'), prev.byRound.get('d2'))
  })
})
