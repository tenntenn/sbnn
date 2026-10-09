import test from 'node:test'
import assert from 'node:assert/strict'

import { blockRanges, blockSlotKey, drawable, finestMark, placeComments } from '../src/previewComments'
import { renderBlocks } from '../src/lineMarks'
import type { Comment } from '../src/types'

/**
 * A Markdown preview draws the comments of its file under the block holding
 * the line each one ends on. Drawing needs a DOM; deciding where is pure and
 * is what these cases drive.
 */

function comment(id: string, endLine: number, over: Partial<Comment> = {}): Comment {
  return {
    id,
    group: 'default',
    diffId: 'd',
    fileId: 'f',
    path: 'a.md',
    side: 'new',
    startLine: endLine,
    endLine,
    body: id,
    snippet: '',
    resolved: false,
    createdAt: '',
    updatedAt: '',
    ...over,
  }
}

const blocks = [
  { start: 1, end: 1 }, // heading
  { start: 3, end: 5 }, // paragraph
  { start: 7, end: 12 }, // list
]

const cases = [
  { name: 'a line inside a block', comments: [comment('a', 4)], want: { 1: ['a'] } },
  { name: 'the first and the last line of a block', comments: [comment('a', 3), comment('b', 5)], want: { 1: ['a', 'b'] } },
  { name: 'one line of a long list stays with the list', comments: [comment('a', 10)], want: { 2: ['a'] } },
  { name: 'the end line decides, not the start', comments: [comment('a', 11, { startLine: 2 })], want: { 2: ['a'] } },
  { name: 'a blank line falls back to the block before it', comments: [comment('a', 6)], want: { 1: ['a'] } },
  { name: 'a line past the end falls back to the last block', comments: [comment('a', 99)], want: { 2: ['a'] } },
  { name: 'the line of a one line block', comments: [comment('a', 1, { startLine: 1 })], want: { 0: ['a'] } },
  { name: 'old side comments are not drawn', comments: [comment('a', 4, { side: 'old' })], want: {} },
  { name: 'file level comments are not drawn', comments: [comment('a', 0, { startLine: 0 })], want: {} },
  { name: 'order within a block is kept', comments: [comment('b', 12), comment('a', 8)], want: { 2: ['b', 'a'] } },
  { name: 'resolved comments are placed like open ones', comments: [comment('a', 4, { resolved: true })], want: { 1: ['a'] } },
]

for (const c of cases) {
  test(`placeComments: ${c.name}`, () => {
    const got: Record<number, string[]> = {}
    for (const [i, list] of placeComments(blocks, c.comments)) got[i] = list.map((x) => x.id)
    assert.deepEqual(got, c.want)
  })
}

test('placeComments: a line before every block falls back to the first', () => {
  const later = [{ start: 5, end: 6 }]
  assert.deepEqual(
    Array.from(placeComments(later, [comment('a', 2)]).keys()),
    [0],
  )
})

test('placeComments: no blocks places nothing', () => {
  assert.equal(placeComments([], [comment('a', 1)]).size, 0)
})

test('drawable', () => {
  assert.equal(drawable(comment('a', 3)), true)
  assert.equal(drawable(comment('a', 3, { side: 'old' })), false)
  assert.equal(drawable(comment('a', 0, { startLine: 0 })), false)
})

test('blockRanges reads the wrappers renderMarkdown writes, in order', () => {
  const html = renderBlocks('# T\n\npara\nmore\n\n- a\n- b\n', 1, 'm', (h) => h)
  assert.deepEqual(blockRanges(html), [
    { start: 1, end: 1 },
    { start: 3, end: 4 },
    { start: 6, end: 7 },
  ])
})

test('blockRanges ignores a data-ln a code block spells out', () => {
  const html = renderBlocks('```\n<div data-ln="9-9">\n```\n', 1, 'm', (h) => h)
  assert.deepEqual(blockRanges(html), [
    { start: 1, end: 3 },
  ])
})

/** marksIn are the lines of the marks inside the first block of md, in
 * order, and the lines that block covers. */
function marksIn(md: string): { marks: number[]; block: { start: number; end: number } } {
  const html = renderBlocks(md, 1, 'm', (h) => h)
  const marks = Array.from(html.matchAll(/<span m="(\d+)"><\/span>/g), (m) => Number(m[1]))
  return { marks, block: blockRanges(html)[0] }
}

const list = '- one\n- two\n  - nested\n- three\n'
const table = '| a | b |\n| - | - |\n| 1 | 2 |\n| 3 | 4 |\n'
const code = '```go\nfunc a()\nfunc b()\n```\n'

const markCases = [
  { name: 'the item a comment ends in', md: list, endLine: 4, want: 4 },
  { name: 'the first item', md: list, endLine: 1, want: 1 },
  { name: 'the innermost item of a nested list', md: list, endLine: 3, want: 3 },
  { name: 'the table header', md: table, endLine: 1, want: 1 },
  { name: 'the delimiter row stays with the header', md: table, endLine: 2, want: 1 },
  { name: 'a body row', md: table, endLine: 4, want: 4 },
  { name: 'a code line', md: code, endLine: 3, want: 3 },
  { name: 'the opening fence has no line of its own', md: code, endLine: 1, want: null },
  { name: 'a line past the block is left to the block', md: list, endLine: 9, want: null },
]

for (const c of markCases) {
  test(`finestMark: ${c.name}`, () => {
    const { marks, block } = marksIn(c.md)
    assert.equal(finestMark(marks, c.endLine, block), c.want)
  })
}

test('blockSlotKey never meets a line', () => {
  assert.equal(blockSlotKey(0) < 0, true)
  assert.notEqual(blockSlotKey(3), blockSlotKey(4))
})
