import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { isPreviewable, previewFormatOf, type FileDiff } from '../src/types'
import { SOURCE_PREVIEW_LINES, sourceLines } from '../src/sourceLines'
import { highlightLine, languageOf, tokenClass } from '../src/highlight'

/** file is a FileDiff with only the fields the preview pane decides on. */
function file(over: Partial<FileDiff> = {}): FileDiff {
  return {
    id: 'f1',
    oldPath: 'pkg/hello.go',
    newPath: 'pkg/hello.go',
    status: 'modified',
    isBinary: false,
    additions: 1,
    deletions: 1,
    viewMode: 'unified',
    isMarkdown: false,
    isImage: false,
    isNotebook: false,
    hunks: [],
    ...over,
  } as FileDiff
}

describe('previewFormatOf', () => {
  // The bug: the preview pane showed Markdown, notebooks and images and
  // nothing else, so selecting a .go file produced no section at all - "No
  // file in this review has a preview" on a wide window, and a disabled
  // Preview tab on a phone - even though the server had already read the
  // whole file from disk for the Markdown preview to use.
  it('draws a source file as its own lines', () => {
    assert.equal(previewFormatOf(file(), true), 'source')
    assert.equal(previewFormatOf(file({ newPath: 'app.ts', oldPath: 'app.ts' }), true), 'source')
    assert.equal(previewFormatOf(file({ newPath: 'conf.yaml', oldPath: 'conf.yaml' }), true), 'source')
    // A file with no extension at all is still text, and still readable.
    assert.equal(previewFormatOf(file({ newPath: 'runme', oldPath: 'runme' }), true), 'source')
    assert.ok(isPreviewable(file(), true))
  })

  it('leaves the rendered formats alone', () => {
    assert.equal(previewFormatOf(file({ isMarkdown: true }), true), 'markdown')
    assert.equal(previewFormatOf(file({ isNotebook: true }), true), 'notebook')
    assert.equal(previewFormatOf(file({ isImage: true, isBinary: true }), true), 'image')
    // A deleted Markdown file still gets a section, as it did before source
    // files were previewable: the section reports the server's refusal.
    assert.equal(previewFormatOf(file({ isMarkdown: true, status: 'deleted' }), true), 'markdown')
  })

  it('has nothing to show for a binary or a deleted file', () => {
    assert.equal(previewFormatOf(file({ isBinary: true }), true), null)
    assert.equal(previewFormatOf(file({ status: 'deleted' }), true), null)
    assert.ok(!isPreviewable(file({ isBinary: true }), true))
  })

  // An exported page has no server to read a working tree file from, and
  // freezes a preview only for Markdown, notebooks and images. A source
  // section there could only say it has nothing, so it is not offered.
  it('offers no source preview where there is no server', () => {
    assert.equal(previewFormatOf(file(), false), null)
    assert.ok(!isPreviewable(file(), false))
    // The frozen formats are unaffected by that.
    assert.equal(previewFormatOf(file({ isMarkdown: true }), false), 'markdown')
    assert.equal(previewFormatOf(file({ isImage: true, isBinary: true }), false), 'image')
  })
})

describe('sourceLines', () => {
  it('does not draw a row for the newline that ends the file', () => {
    assert.deepEqual(sourceLines('a\nb\n'), ['a', 'b'])
    assert.deepEqual(sourceLines('a\nb'), ['a', 'b'])
  })

  it('keeps the blank lines that are part of the file', () => {
    assert.deepEqual(sourceLines('a\n\n\nb\n'), ['a', '', '', 'b'])
    assert.deepEqual(sourceLines(''), [''])
  })

  it('drops the carriage returns of a CRLF file', () => {
    assert.deepEqual(sourceLines('a\r\nb\r\n'), ['a', 'b'])
    // A lone CR inside a line is content, not a line ending.
    assert.deepEqual(sourceLines('a\rb\n'), ['a\rb'])
  })

  it('is bounded, so one generated file cannot mount a hundred thousand rows', () => {
    assert.ok(SOURCE_PREVIEW_LINES > 0)
    const big = sourceLines(Array.from({ length: 10000 }, (_, i) => `line ${i}`).join('\n') + '\n')
    assert.equal(big.length, 10000)
    assert.ok(big.length > SOURCE_PREVIEW_LINES, 'the cap has to be reachable to be a cap')
  })
})

// The preview reuses the highlighter #300 added for the diff rather than
// bringing in one of its own; nothing in package.json changes. These are the
// three calls a source preview makes.
describe('the highlighter a source preview reuses', () => {
  it('knows the languages a preview will be asked for', () => {
    assert.equal(languageOf('pkg/hello.go'), 'go')
    assert.equal(languageOf('web/src/App.tsx'), 'js')
    assert.equal(languageOf('conf.yaml'), 'yaml')
    // An extension it does not know gets no colour rather than a guess.
    assert.equal(languageOf('notes.txt'), null)
    assert.equal(languageOf('runme'), null)
  })

  it('colours a line the same whether it came from a hunk or from a file', () => {
    const tokens = highlightLine('func Greet(name string) string { // hi', 'go')
    const kinds = tokens.map((t) => t.kind)
    assert.ok(kinds.includes('keyword'), `no keyword in ${JSON.stringify(tokens)}`)
    assert.ok(kinds.includes('comment'), `no comment in ${JSON.stringify(tokens)}`)
    // Whatever it splits into, it still spells the line.
    assert.equal(tokens.map((t) => t.text).join(''), 'func Greet(name string) string { // hi')
    assert.equal(tokenClass('keyword'), 'tok-keyword')
    assert.equal(tokenClass('plain'), undefined)
  })

  it('leaves a line of an unknown language exactly as it was', () => {
    const tokens = highlightLine('plain words', null)
    assert.deepEqual(tokens, [{ text: 'plain words', kind: 'plain' }])
  })
})
