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
 *
 * That fallback is a decision, not an accident: a comment whose end line is
 * in a gap between blocks, in the front matter (which has no block of its
 * own) or before the first block is shown under the nearest preceding block,
 * or under the first block when there is none.
 *
 * There is no group filter here or in the caller: the comments in App are
 * those of the one group the page shows, and the caller only narrows them to
 * the file.
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

/**
 * finestMark picks the line mark a comment is drawn under inside its block:
 * the last mark at or before the comment's end line, which is the start of the
 * list item, table row or code line the comment ends in. Marks are the lines
 * lineMarks.ts put at the start of each line, in document order.
 *
 * It answers null - draw under the whole block - when the end line is not
 * inside the block (the fallback for a line in a gap, see placeComments) or
 * when no mark comes before it.
 */
export function finestMark(marks: number[], endLine: number, block: BlockRange): number | null {
  if (endLine < block.start || endLine > block.end) return null
  let found: number | null = null
  for (const m of marks) {
    if (m <= endLine) found = m
    else break
  }
  return found
}

/** blockSlotKey is the key of the slot under a whole block. A slot under a
 * finer element is keyed by its mark's (positive) line, so the two cannot
 * meet. */
export function blockSlotKey(index: number): number {
  return -(index + 1)
}

/** SLOT_CLASS is the class of the element a file's comments are drawn into.
 * Everything inside such an element is a comment, never the document. */
export const SLOT_CLASS = 'preview-comments'

/** COMMENT_ID_PREFIX is how the diff pane's copy of a comment is named in
 * the DOM; PREVIEW_COMMENT_ID_PREFIX is the preview's copy, so that one
 * comment drawn in both panes does not give the page two elements with the
 * same id. */
export const COMMENT_ID_PREFIX = 'comment-'
export const PREVIEW_COMMENT_ID_PREFIX = 'preview-comment-'

/** commentDomIds are the ids a comment may have on the page, one per pane. */
export function commentDomIds(id: string): string[] {
  return [COMMENT_ID_PREFIX, PREVIEW_COMMENT_ID_PREFIX].map((p) => `${p}${id}`)
}

/**
 * diffSlots says which slots to create and which to drop to go from the
 * blocks that have one to the blocks that should. Slots that stay are left
 * alone, which is what keeps a thread's own state (a half written reply)
 * when another comment arrives.
 */
export function diffSlots(have: Iterable<number>, want: Iterable<number>): { add: number[]; remove: number[] } {
  const h = new Set(have)
  const w = new Set(want)
  return {
    add: [...w].filter((i) => !h.has(i)).sort((a, b) => a - b),
    remove: [...h].filter((i) => !w.has(i)).sort((a, b) => a - b),
  }
}
