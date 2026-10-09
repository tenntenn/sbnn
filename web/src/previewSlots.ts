import type { Comment } from './types'
import { SLOT_CLASS, blockSlotKey, finestMark, type BlockRange } from './previewComments'

/**
 * previewSlots decides, against the preview's DOM, which element each comment
 * thread is drawn under: the list item, table row or code line the comment
 * ends in, and the whole top level block when there is nothing finer.
 *
 * A thread is a block level element and cannot sit just anywhere, so each kind
 * of element has its own place for it: inside the list item (before a nested
 * list), in a row of its own after a table row, and inside the code element
 * between two lines.
 */

/** SlotTarget is one slot to create: the comments that go in it, the kind of
 * element it is, and how to put it on the page. */
export interface SlotTarget {
  comments: Comment[]
  /** tag is span inside code, where only inline content is allowed (the
   * stylesheet lays it out as a block), and div everywhere else. */
  tag: 'div' | 'span'
  /** attach puts slot where the thread belongs and returns the element to
   * remove to take it away again (the slot itself, or the row around it). */
  attach: (slot: HTMLElement) => HTMLElement
}

function markLine(el: Element): number {
  return Number((el as HTMLElement).dataset.ln)
}

/** marksOf are the single line marks inside el, in document order. A range
 * (`3-5`) is a block's wrapper, not a mark. */
function marksOf(el: Element): HTMLElement[] {
  return Array.from(el.querySelectorAll<HTMLElement>('span[data-ln]')).filter((m) => /^\d+$/.test(m.dataset.ln ?? ''))
}

/** blockElements are the top level blocks of the preview, the same elements
 * blockRanges counted. */
export function blockElements(body: Element): HTMLElement[] {
  return Array.from(body.children).filter(
    (el): el is HTMLElement =>
      el instanceof HTMLElement && el.dataset.ln !== undefined && !el.classList.contains(SLOT_CLASS),
  )
}

/** newSlot makes the element a thread is portalled into. */
export function newSlot(tag: 'div' | 'span'): HTMLElement {
  const slot = document.createElement(tag)
  slot.className = SLOT_CLASS
  return slot
}

function afterBlock(block: HTMLElement): SlotTarget['attach'] {
  return (slot) => {
    block.after(slot)
    return slot
  }
}

function inItem(li: HTMLElement, mark: HTMLElement): SlotTarget['attach'] {
  return (slot) => {
    // After the item's own text and before the list nested in it, which is
    // about the items below; a nested list that comes before the mark belongs
    // to text further up and does not count.
    const nested = Array.from(li.children).find(
      (c) =>
        (c.tagName === 'UL' || c.tagName === 'OL') &&
        (mark.compareDocumentPosition(c) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
    )
    li.insertBefore(slot, nested ?? null)
    return slot
  }
}

function afterRow(tr: HTMLElement): SlotTarget['attach'] {
  return (slot) => {
    const row = document.createElement('tr')
    row.className = 'preview-comments-row'
    const cell = document.createElement('td')
    cell.colSpan = Math.max(tr.children.length, 1)
    cell.append(slot)
    row.append(cell)
    tr.after(row)
    return row
  }
}

function beforeMark(code: HTMLElement, next: HTMLElement | null): SlotTarget['attach'] {
  return (slot) => {
    // A mark wrapped in another element is not a child of code.
    if (next && next.parentNode) next.parentNode.insertBefore(slot, next)
    else code.append(slot)
    return slot
  }
}

/**
 * resolveTargets maps a slot key to the slot's target. placement is
 * placeComments' answer (comments by block); the key of a slot under a whole
 * block is blockSlotKey(index) and the key of one under a finer element is the
 * line of its mark. Comments that end in the same element share a slot.
 */
export function resolveTargets(
  body: Element,
  ranges: BlockRange[],
  placement: Map<number, Comment[]>,
): Map<number, SlotTarget> {
  const out = new Map<number, SlotTarget>()
  const blocks = blockElements(body)
  for (const [index, comments] of placement) {
    const block = blocks[index]
    if (!block) continue
    const marks = marksOf(block)
    const lines = marks.map(markLine)
    for (const c of comments) {
      let key = blockSlotKey(index)
      let target: Omit<SlotTarget, 'comments'> = { tag: 'div', attach: afterBlock(block) }
      const line = finestMark(lines, c.endLine, ranges[index])
      if (line !== null) {
        // The last mark of that line: a tight list item has two on it.
        const mark = marks[lines.lastIndexOf(line)]
        // A quote inside an item is not the item's own line: it stays under
        // the whole block rather than at the end of the item.
        const host = mark.closest('li, tr, pre, blockquote')
        if (host instanceof HTMLElement && host.tagName !== 'BLOCKQUOTE' && block.contains(host)) {
          key = line
          if (host.tagName === 'LI') target = { tag: 'div', attach: inItem(host, mark) }
          else if (host.tagName === 'TR') target = { tag: 'div', attach: afterRow(host) }
          else {
            const code = host.querySelector('code') ?? host
            const next = marksOf(code).find((m) => markLine(m) > line) ?? null
            target = { tag: 'span', attach: beforeMark(code, next) }
          }
        }
      }
      const have = out.get(key)
      if (have) have.comments.push(c)
      else out.set(key, { comments: [c], ...target })
    }
  }
  return out
}
