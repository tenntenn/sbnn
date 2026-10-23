import test from 'node:test'
import assert from 'node:assert/strict'

import { ADDED_CLASS, REMOVED_CLASS, markChanges, richChanges, type RichChanges } from '../src/richDiff'
import { blockRanges, placeComments } from '../src/previewComments'
import { renderBlocks } from '../src/lineMarks'
import type { Comment, Hunk, Line } from '../src/types'

/**
 * The preview marks what the diff changed (#430): which blocks hold an added
 * line, and where what a hunk removed is drawn. Both are decided on the
 * preview's HTML as a string, so they are driven here without a DOM.
 */

/** hunk builds a hunk from lines written the way a diff writes them, with
 * the numbers worked out from where the hunk starts. */
function hunk(oldStart: number, newStart: number, text: string[]): Hunk {
  let o = oldStart
  let n = newStart
  const lines: Line[] = text.map((t) => {
    const content = t.slice(1)
    if (t[0] === '-') return { kind: 'delete', oldNumber: o++, newNumber: 0, content }
    if (t[0] === '+') return { kind: 'add', oldNumber: 0, newNumber: n++, content }
    return { kind: 'context', oldNumber: o++, newNumber: n++, content }
  })
  return {
    header: '',
    oldStart,
    oldLines: o - oldStart,
    newStart,
    newLines: n - newStart,
    lines,
  }
}

const render = (md: string) => `<p>${md}</p>`
const label = 'Removed'
const preview = (md: string) => renderBlocks(md, 1, 'm', (h) => h)

// The new side: a heading, a changed paragraph, a list that lost an item, and
// a section that was added at the end.
const NEW = ['# Title', '', 'New wording.', '', '- one', '- three', '', '## Added', '', 'Fresh text.', ''].join('\n')
const HUNKS = [
  hunk(1, 1, [
    ' # Title',
    ' ',
    '-Old wording.',
    '+New wording.',
    ' ',
    ' - one',
    '-- two',
    ' - three',
    '+',
    '+## Added',
    '+',
    '+Fresh text.',
  ]),
]

test('a complete preview takes the hunks at their own line numbers', () => {
  const changes = richChanges(HUNKS, NEW, true)
  // The blank lines added around the new section are not marks: one renders
  // as nothing, and the list's lines run on over the blank after it.
  assert.deepEqual([...changes.added].sort((a, b) => a - b), [3, 8, 10])
  assert.deepEqual(changes.removed, [
    { at: 3, text: 'Old wording.' },
    { at: 6, text: '- two' },
  ])
})

test('a block holding an added line is marked, and only such a block', () => {
  const html = markChanges(preview(NEW), richChanges(HUNKS, NEW, true), render, label)
  const wrappers = Array.from(html.matchAll(/<div data-ln="(\d+-\d+)"( class="[^"]*")?>/g), (m) => [m[1], m[2] ?? ''])
  assert.deepEqual(wrappers, [
    ['1-1', ''],
    ['3-3', ` class="${ADDED_CLASS}"`],
    ['5-6', ''],
    ['8-8', ` class="${ADDED_CLASS}"`],
    ['10-10', ` class="${ADDED_CLASS}"`],
  ])
})

test('a removed run is drawn in front of the block it was removed from', () => {
  const html = markChanges(preview(NEW), richChanges(HUNKS, NEW, true), render, label)
  const order = Array.from(html.matchAll(/<div (?:data-ln="(\d+-\d+)"|class="rich-removed"[^>]*>(<p>[^<]*<\/p>))/g), (m) => m[1] ?? m[2])
  // The old paragraph sits above the new one, and the lost list item above
  // the list it was taken out of.
  assert.deepEqual(order, ['1-1', '<p>Old wording.</p>', '3-3', '<p>- two</p>', '5-6', '8-8', '10-10'])
  assert.match(html, new RegExp(`<div class="${REMOVED_CLASS}" role="group" aria-label="Removed">`))
})

test('what is removed at the end of the file goes after the last block', () => {
  const md = 'Kept.\n'
  const changes = richChanges([hunk(1, 1, [' Kept.', '-Gone.'])], md, true)
  assert.deepEqual(changes.removed, [{ at: 2, text: 'Gone.' }])
  const html = markChanges(preview(md), changes, render, label)
  assert.ok(html.endsWith(`<div class="${REMOVED_CLASS}" role="group" aria-label="Removed"><p>Gone.</p></div>`), html)
})

test('a removal written with no context follows the line its header names', () => {
  const h: Hunk = { ...hunk(3, 2, ['-Gone.']), newStart: 2, newLines: 0 }
  assert.deepEqual(richChanges([h], 'a\nb\nc\n', true).removed, [{ at: 3, text: 'Gone.' }])
})

test('lines removed from inside a code block are shown as code', () => {
  const md = '```\nkeep()\nnew()\n```\n'
  const changes = richChanges([hunk(1, 1, [' ```', ' keep()', '-old() && <x>', '+new()', ' ```'])], md, true)
  const html = markChanges(preview(md), changes, render, label)
  assert.match(html, /<div class="rich-removed"[^>]*><pre><code>old\(\) &amp;&amp; &lt;x&gt;\n<\/code><\/pre><\/div><div data-ln="1-4" class="rich-added">/)
})

test('a partial preview finds each hunk in its own source', () => {
  // A reconstruction puts a note of sbnn's own where the diff skipped lines,
  // so the second hunk, at line 40 of the file, is at line 5 of the preview.
  const source = ['Intro.', '', '> 30 lines not in the diff', '', 'Changed.', ''].join('\n')
  const hunks = [hunk(1, 1, [' Intro.']), hunk(40, 40, ['-Was.', '+Changed.'])]
  const changes = richChanges(hunks, source, false)
  assert.deepEqual([...changes.added], [5])
  assert.deepEqual(changes.removed, [{ at: 5, text: 'Was.' }])
})

test('a partial preview leaves a hunk it cannot find unmarked', () => {
  const changes = richChanges([hunk(9, 9, ['-a', '+not there'])], 'something else\n', false)
  assert.deepEqual(changes, { added: new Set(), removed: [] })
})

test('a partial preview without its source marks nothing', () => {
  assert.deepEqual(richChanges(HUNKS, undefined, false), { added: new Set(), removed: [] })
})

test('no changes leave the HTML exactly as it was', () => {
  const html = preview(NEW)
  const none: RichChanges = { added: new Set(), removed: [] }
  assert.equal(markChanges(html, none, render, label), html)
})

function comment(id: string, endLine: number): Comment {
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
  }
}

test('the marks change neither the blocks nor where comments go', () => {
  const plain = preview(NEW)
  const marked = markChanges(plain, richChanges(HUNKS, NEW, true), render, label)
  // The removed runs are not blocks of the file: they carry no line, so the
  // block list, and the index each comment is placed under, are what they
  // were without them.
  assert.deepEqual(blockRanges(marked), blockRanges(plain))
  const comments = [comment('a', 3), comment('b', 6), comment('c', 10)]
  assert.deepEqual(placeComments(blockRanges(marked), comments), placeComments(blockRanges(plain), comments))
})

test('nothing in a removed run can be read as a line of the file', () => {
  // renderRemovedMarkdown renders without line marks, and the sanitiser
  // strips any data-ln the diff writes itself; this checks the wrapper adds
  // none of its own.
  const marked = markChanges(preview(NEW), richChanges(HUNKS, NEW, true), render, label)
  for (const m of marked.matchAll(/<div class="rich-removed"[^>]*>(.*?)<\/div>/g)) {
    assert.doesNotMatch(m[0], /data-ln|\sm=/)
  }
})
