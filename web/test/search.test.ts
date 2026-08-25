import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  MAX_SCANNED_LINES,
  matchSummary,
  matchesPath,
  queryTerms,
  searchDiffs,
  searchFile,
} from '../src/search'
import type { Diff, FileDiff, Hunk, Line } from '../src/types'

function line(content: string, n: number): Line {
  return { kind: 'context', oldNumber: n, newNumber: n, content }
}

function hunk(...contents: string[]): Hunk {
  return {
    header: '@@ -1,1 +1,1 @@',
    oldStart: 1,
    oldLines: contents.length,
    newStart: 1,
    newLines: contents.length,
    lines: contents.map(line),
  }
}

function file(id: string, path: string, ...hunks: Hunk[]): FileDiff {
  return {
    id,
    oldPath: path,
    newPath: path,
    status: 'modified',
    isBinary: false,
    additions: 1,
    deletions: 0,
    viewMode: 'unified',
    isMarkdown: false,
    isImage: false,
    isNotebook: false,
    hunks,
  }
}

function diff(id: string, ...files: FileDiff[]): Diff {
  return { id, title: `diff ${id}`, baseDir: '', createdAt: '', raw: '', files }
}

const store = file('f1', 'internal/server/store.go', hunk(
  'func (s *Store) handleGroups() {',
  '\treturn s.groups',
  '}',
))
const handler = file('f2', 'internal/server/handler.go', hunk(
  '\ts.handleGroups()',
  '\tlog.Print("SBNN_TARGET")',
))
const readme = file('f3', 'README.md', hunk('# sbnn', 'A diff viewer.'))
const review = [diff('d1', store, handler, readme)]

describe('queryTerms', () => {
  it('splits on whitespace, folds case and drops empties', () => {
    assert.deepEqual(queryTerms('  Server   GO '), ['server', 'go'])
    assert.deepEqual(queryTerms('   '), [])
    assert.deepEqual(queryTerms(''), [])
  })
})

describe('matchesPath', () => {
  // Unchanged semantics: every term has to be present, in any order,
  // ignoring case. Nothing turns up that does not contain what was typed.
  it('wants every term, in any order, ignoring case', () => {
    assert.equal(matchesPath('internal/server/server.go', 'server go'), true)
    assert.equal(matchesPath('internal/server/server.go', 'internal/server'), true)
    assert.equal(matchesPath('internal/server/server.go', 'SERVER'), true)
    assert.equal(matchesPath('internal/server/server.go', 'server missing'), false)
  })

  it('matches everything on an empty query', () => {
    assert.equal(matchesPath('anything.go', ''), true)
    assert.equal(matchesPath('anything.go', '   '), true)
  })

  // A looser match - the letters in order, anywhere - would find things the
  // reader did not ask for, which in a list being scanned is worse than
  // finding nothing.
  it('is a substring test, not a fuzzy one', () => {
    assert.equal(matchesPath('internal/server/store.go', 'isst'), false)
  })
})

describe('searchFile', () => {
  // Issue #99: the whole of search was the path, so "where did handleGroups
  // get renamed?" could not be asked at all. The text is already in memory.
  it('finds a term that is only in the content', () => {
    const got = searchFile(store, ['handlegroups'])
    assert.equal(got.inPath, false)
    assert.equal(got.lines, 1)
    assert.deepEqual(got.firstLine, { hunk: 0, line: 0 })
  })

  it('reports the path and the content separately', () => {
    const got = searchFile(store, ['store'])
    assert.equal(got.inPath, true)
    assert.equal(got.lines, 1)
  })

  // A count of lines is only meaningful if every term is on the same line:
  // "3 lines" has to be three places worth opening, not three coincidences
  // spread over a file.
  it('wants every term on one line, not scattered over the file', () => {
    const spread = file('f9', 'x.go', hunk('alpha here', 'bravo there'))
    assert.equal(searchFile(spread, ['alpha', 'bravo']).lines, 0)
    assert.equal(searchFile(spread, ['alpha', 'here']).lines, 1)
  })

  it('answers nothing for an empty query', () => {
    const got = searchFile(store, [])
    assert.equal(got.lines, 0)
    assert.equal(got.scanned, 0)
    assert.equal(got.firstLine, null)
  })

  // A partial answer must never be handed over as if it were the whole one.
  it('says so when the budget runs out', () => {
    const got = searchFile(store, ['return'], 1)
    assert.equal(got.truncated, true)
    assert.equal(got.scanned, 1)
    assert.equal(got.lines, 0)
  })

  it('does not claim truncation when the budget was enough', () => {
    const got = searchFile(store, ['return'], 3)
    assert.equal(got.truncated, false)
    assert.equal(got.lines, 1)
  })
})

describe('searchDiffs', () => {
  it('is not a search at all when the query is empty', () => {
    const got = searchDiffs(review, '   ')
    assert.equal(got.active, false)
    assert.equal(got.matches.size, 0)
    assert.equal(got.files, 0)
  })

  it('counts the files and the lines behind the summary', () => {
    const got = searchDiffs(review, 'handlegroups')
    assert.equal(got.active, true)
    assert.equal(got.files, 2)
    assert.equal(got.lines, 2)
    assert.deepEqual([...got.matches.keys()], ['d1:f1', 'd1:f2'])
  })

  it('keeps a file the path alone answered for', () => {
    const got = searchDiffs(review, 'readme')
    assert.equal(got.files, 1)
    assert.equal(got.lines, 0)
    assert.equal(got.matches.get('d1:f3')?.inPath, true)
  })

  it('leaves out a file that answered neither way', () => {
    const got = searchDiffs(review, 'sbnn_target')
    assert.deepEqual([...got.matches.keys()], ['d1:f2'])
  })

  // The budget is spent across the whole review, not per file, and the
  // result says when it ran out.
  it('spends one budget over the whole review and reports running out', () => {
    const got = searchDiffs(review, 'handlegroups', 2)
    assert.equal(got.truncated, true)
  })

  it('does not report running out when it did not', () => {
    assert.equal(searchDiffs(review, 'handlegroups').truncated, false)
    assert.ok(MAX_SCANNED_LINES > 0)
  })

  // Two rounds touching the same path have to stay apart, which is what the
  // sectionKey key is for.
  it('keys matches so two rounds touching one path stay apart', () => {
    const got = searchDiffs([diff('d1', store), diff('d2', store)], 'store')
    assert.deepEqual([...got.matches.keys()], ['d1:f1', 'd2:f1'])
    assert.equal(got.files, 2)
  })
})

describe('matchSummary', () => {
  // A file found only by its content would otherwise look like a path that
  // matched, and the reader would go looking in the name for a word that is
  // in the code.
  it('says which side answered', () => {
    const only = { inPath: true, lines: 0, firstLine: null, scanned: 0, truncated: false }
    assert.equal(matchSummary(only), 'path')
    assert.equal(matchSummary({ ...only, inPath: false, lines: 1 }), '1 line')
    assert.equal(matchSummary({ ...only, inPath: false, lines: 3 }), '3 lines')
    assert.equal(matchSummary({ ...only, lines: 3 }), 'path + 3 lines')
  })
})
