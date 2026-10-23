import type { Hunk } from './types'
import { escapeHTML } from './markdown'

/**
 * richDiff marks what a diff changed inside the rendered Markdown preview, the
 * way GitHub's rich diff does for prose (#430): a block holding an added line
 * is tinted green, and what a hunk removed is rendered too, in place, tinted
 * red, so the old wording sits next to the new.
 *
 * It has no DOM in it. It reads the preview's HTML as the string
 * renderMarkdown wrote, where every top level block is a
 * `<div data-ln="start-end">` (see lineMarks.ts), and works on that string, so
 * it can be tested on its own and an exported page does exactly what the live
 * one does.
 */

/** ADDED_CLASS is put on the wrapper of a block that holds an added line. */
export const ADDED_CLASS = 'rich-added'

/**
 * REMOVED_CLASS is the element removed Markdown is drawn in.
 *
 * It is not the file. It carries no line marks at all - it is rendered without
 * them, see renderRemovedMarkdown - so blockRanges, the comment slots and the
 * scroll position, which all count the marked blocks, do not see it; and
 * PreviewSelection treats it like a drawn comment, so nothing selected in it
 * becomes a comment on a line it is not on.
 */
export const REMOVED_CLASS = 'rich-removed'

/** MARKS_OFF_CLASS, on any ancestor of a preview, hides the change marks. The
 * marks stay in the HTML either way, so turning them off and on again does not
 * replace the preview and the comment threads drawn into it. */
export const MARKS_OFF_CLASS = 'rich-marks-off'

/** RemovedRun is one run of consecutive removed lines, and the preview line it
 * was removed in front of. `at` is Infinity when nothing of the new side
 * follows it. */
export interface RemovedRun {
  at: number
  text: string
}

/** RichChanges is what a file's hunks changed, in the preview's own line
 * numbers: the lines added, and the runs removed. */
export interface RichChanges {
  added: Set<number>
  removed: RemovedRun[]
}

/** stripCR compares a line of a CRLF file the same way as its LF copy: the
 * diff and the file read from disk need not agree on which one they kept. */
function stripCR(s: string): string {
  return s.endsWith('\r') ? s.slice(0, -1) : s
}

/**
 * findRun is the first index at or after from where lines holds want in a
 * row, or -1.
 */
function findRun(lines: string[], want: string[], from: number): number {
  outer: for (let j = from; j + want.length <= lines.length; j++) {
    for (let k = 0; k < want.length; k++) {
      if (lines[j + k] !== want[k]) continue outer
    }
    return j
  }
  return -1
}

/**
 * richChanges works out which preview lines a file's hunks added and where
 * what they removed goes.
 *
 * A complete preview numbers its lines the way the file does, so a hunk's new
 * line numbers are the preview's. A partial one does not: it is rebuilt from
 * the diff, with a note of sbnn's own standing in for each stretch the diff
 * skipped (see Reconstruct in internal/diff), which shifts every line after
 * it. There each hunk is found in the preview's source by its new side lines,
 * in order, which is what the reconstruction is made of. A hunk that cannot be
 * found is left unmarked rather than marked in the wrong place.
 */
export function richChanges(hunks: readonly Hunk[], source: string | undefined, complete: boolean): RichChanges {
  const added = new Set<number>()
  const removed: RemovedRun[] = []
  const lines = complete || source === undefined ? null : source.split('\n').map(stripCR)
  if (!complete && lines === null) return { added, removed }
  // cursor is the 0-based preview line after the last hunk found.
  let cursor = 0
  for (const hunk of hunks) {
    let offset = 0
    if (lines !== null) {
      const kept = hunk.lines.filter((l) => l.kind !== 'delete')
      if (kept.length > 0) {
        const at = findRun(lines, kept.map((l) => stripCR(l.content)), cursor)
        if (at < 0) continue
        offset = at + 1 - kept[0].newNumber
        cursor = at + kept.length
      }
    }
    const previewLine = (newNumber: number) => newNumber + offset
    let pending: string[] = []
    let last: number | null = null
    for (const line of hunk.lines) {
      if (line.kind === 'delete') {
        pending.push(line.content)
        continue
      }
      const p = previewLine(line.newNumber)
      if (pending.length > 0) {
        removed.push({ at: p, text: pending.join('\n') })
        pending = []
      }
      // A blank line renders as nothing, and a block's lines may run on over
      // the blank after it: counting one would paint a block nobody changed.
      if (line.kind === 'add' && line.content.trim() !== '') added.add(p)
      last = p
    }
    if (pending.length > 0) {
      // Nothing of the hunk's new side follows the run, so it goes after the
      // last line the hunk kept. A hunk with none (a removal written with no
      // context) names the new line it follows in its header, which a partial
      // preview cannot be trusted to number; there it follows the hunk before.
      let at: number
      if (last !== null) at = last + 1
      else if (lines === null) at = hunk.newStart + 1
      else at = cursor + 1
      removed.push({ at, text: pending.join('\n') })
    }
  }
  return { added, removed }
}

/** RemovedRender turns a removed run into HTML. It is renderRemovedMarkdown
 * on the page and a stand-in in the tests, which have no DOM to sanitise in. */
export type RemovedRender = (markdown: string) => string

const WRAPPER = /<div data-ln="(\d+)-(\d+)">/g

/**
 * markChanges puts the change marks on a rendered preview.
 *
 * A block whose lines overlap an added line gets ADDED_CLASS on its wrapper.
 * A removed run is drawn in front of the first block that ends at or after the
 * line it was removed before, which is the block it was removed from or the
 * one that now follows it; that keeps the old wording above the new, as the
 * diff has it. A run removed from inside a code block is shown as code, since
 * its lines are code without the fence around them.
 *
 * label is the removed runs' accessible name ("Removed"), which the stylesheet
 * also shows as their caption.
 */
export function markChanges(html: string, changes: RichChanges, render: RemovedRender, label: string): string {
  if (changes.added.size === 0 && changes.removed.length === 0) return html
  const blocks = Array.from(html.matchAll(WRAPPER), (m) => ({
    index: m.index,
    length: m[0].length,
    start: Number(m[1]),
    end: Number(m[2]),
  }))
  const before = new Map<number, string[]>()
  const atEnd: string[] = []
  for (const run of changes.removed) {
    const i = blocks.findIndex((b) => b.end >= run.at)
    const inCode =
      i >= 0 && blocks[i].start < run.at && html.startsWith('<pre', blocks[i].index + blocks[i].length)
    const body = inCode ? `<pre><code>${escapeHTML(run.text)}\n</code></pre>` : render(run.text)
    if (body.trim() === '') continue
    const el = `<div class="${REMOVED_CLASS}" role="group" aria-label="${escapeHTML(label)}">${body}</div>`
    if (i < 0) atEnd.push(el)
    else before.set(i, [...(before.get(i) ?? []), el])
  }
  let out = ''
  let pos = 0
  blocks.forEach((b, i) => {
    out += html.slice(pos, b.index)
    out += (before.get(i) ?? []).join('')
    let added = false
    for (let ln = b.start; ln <= b.end && !added; ln++) added = changes.added.has(ln)
    out += added ? `<div data-ln="${b.start}-${b.end}" class="${ADDED_CLASS}">` : html.slice(b.index, b.index + b.length)
    pos = b.index + b.length
  })
  return out + html.slice(pos) + atEnd.join('')
}
