import type { Comment } from './types'

/**
 * previewComments decides where the comments of a file are drawn in its
 * Markdown preview. It has no DOM in it, so it can be tested on its own.
 *
 * The preview wraps every top level block in an element carrying the source
 * lines it covers (`data-ln="start-end"`, see lineMarks.ts), and a comment on
 * the new side of the file names the line it ends on. A comment is drawn
 * under the block that holds that line.
 */

/** BlockRange is the source lines one top level block of the preview covers. */
export interface BlockRange {
  start: number
  end: number
}

/**
 * blockRanges reads the lines of each block off the preview's HTML, in the
 * order the blocks appear.
 *
 * Only the wrappers renderMarkdown adds can match: the sanitiser refuses data
 * attributes, so nothing a document writes itself carries one, and the text of
 * a document that merely spells one out is escaped.
 */
export function blockRanges(html: string): BlockRange[] {
  const out: BlockRange[] = []
  for (const m of html.matchAll(/<div data-ln="(\d+)-(\d+)">/g)) {
    out.push({ start: Number(m[1]), end: Number(m[2]) })
  }
  return out
}

/** drawable says whether a comment can be put in a preview at all: it has to
 * be on the new side, which is the file the preview shows, and to name a
 * line. A comment about the file as a whole has no line to sit under. */
export function drawable(c: Pick<Comment, 'side' | 'startLine' | 'endLine'>): boolean {
  return c.side === 'new' && c.startLine > 0 && c.endLine > 0
}

/**
 * placeComments maps the index of a block to the comments drawn under it.
 *
 * A comment goes under the block holding its end line. A line no block holds
 * (the blank line between two blocks, the front matter, a line past the end
 * of what the preview shows) falls back to the nearest block before it, and
 * to the first block when there is none. A list of no blocks places nothing.
 * Within one block comments keep the order they were given in.
 */
export function placeComments(blocks: BlockRange[], comments: Comment[]): Map<number, Comment[]> {
  const placed = new Map<number, Comment[]>()
  if (blocks.length === 0) return placed
  for (const c of comments) {
    if (!drawable(c)) continue
    let index = -1
    for (let i = 0; i < blocks.length; i++) {
      if (blocks[i].start <= c.endLine) index = i
      if (blocks[i].start <= c.endLine && c.endLine <= blocks[i].end) {
        index = i
        break
      }
    }
    if (index < 0) index = 0
    const list = placed.get(index)
    if (list) list.push(c)
    else placed.set(index, [c])
  }
  return placed
}
