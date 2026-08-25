/**
 * The keys the review page answers to.
 *
 * Reviewing is reading, and reading is done with both hands on the
 * keyboard: the next file, the next comment, fold this away, submit. Every
 * key here is a single unmodified press, which is only safe because none of
 * them fires while something is being typed into - see typingInto below.
 *
 * The list is the documentation: the help overlay is drawn from it, so a
 * shortcut cannot exist without a line explaining it.
 */
export interface Shortcut {
  keys: string[]
  what: string
}

export const shortcuts: Shortcut[] = [
  { keys: ['j'], what: 'Next file' },
  { keys: ['k'], what: 'Previous file' },
  { keys: ['/'], what: 'Search paths and lines' },
  { keys: ['n'], what: 'Next comment' },
  { keys: ['p'], what: 'Previous comment' },
  { keys: ['f'], what: 'Fold or unfold this file' },
  { keys: ['v'], what: 'Split or unified' },
  { keys: ['s'], what: 'Follow the diff with the preview' },
  { keys: ['r'], what: 'Submit review' },
  { keys: ['?'], what: 'This list' },
  { keys: ['Esc'], what: 'Close what is open' },
]

/**
 * typingInto reports whether a key belongs to whatever the reader is
 * writing in. A comment full of the letter "f" would be unusable if every
 * one of them folded the file away.
 */
export function typingInto(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  if (el.isContentEditable) return true
  return /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)
}

/** plainKey reports whether an event is an unmodified key press. */
export function plainKey(ev: KeyboardEvent): boolean {
  return !ev.metaKey && !ev.ctrlKey && !ev.altKey
}

/**
 * CommentStop is one stop on the `n` / `p` tour: a comment's own id, and
 * the sectionKey of the file it sits in. Nothing else about a comment
 * matters to the stepping, and keeping it to this leaves the rule here,
 * where the keys are described, rather than in the page.
 */
export interface CommentStop {
  id: string
  key: string
}

/**
 * stepToComment is where `n` (by = 1) and `p` (by = -1) land.
 *
 * The tour is over comments, not files: several comments on one file are
 * several stops, so the position has to be a comment and not the file being
 * read. `currentId` is the comment the reader was last taken to; stepping
 * from it wraps around the ends.
 *
 * When there is no such comment - nothing visited yet, or it was resolved
 * away - the tour rejoins at the file being read: the first of its comments
 * going forward, the last going back. That comment is the destination rather
 * than a place to step off from, because the reader is not standing on it
 * yet. A file with no comments of its own falls back to the first or last
 * comment of the whole review.
 *
 * The comment is looked up by id alone. Requiring it to also sit in the
 * active file looked like a way to notice a reader who had scrolled
 * elsewhere, but ids are unique, and activeKey is not the reader's alone:
 * the page re-centres the comment it just jumped to, and the observer that
 * picks the active file ignores the top 70% of the viewport, so a comment
 * near the top of its file can hand activeKey back to the file before it
 * with nobody having touched anything. Discarding the recorded position on
 * that made the next press rejoin - and walk backwards - which is the same
 * stuck feeling this function exists to end.
 */
export function stepToComment(
  stops: CommentStop[],
  currentId: string | null,
  activeKey: string | null,
  by: number,
): CommentStop | null {
  if (stops.length === 0) return null
  const at = stops.findIndex((s) => s.id === currentId)
  if (at >= 0) {
    const index = (at + by) % stops.length
    return stops[(index + stops.length) % stops.length]
  }
  if (activeKey !== null) {
    if (by > 0) {
      const first = stops.find((s) => s.key === activeKey)
      if (first) return first
    } else {
      for (let i = stops.length - 1; i >= 0; i--) {
        if (stops[i].key === activeKey) return stops[i]
      }
    }
  }
  return by > 0 ? stops[0] : stops[stops.length - 1]
}
