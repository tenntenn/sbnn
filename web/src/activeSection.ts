/**
 * Which file section the reader is looking at.
 *
 * The diff pane watches every section with one IntersectionObserver whose
 * root margin leaves only the top of the pane sensitive, so "intersecting"
 * means "some part of this file is in the band at the top". Two files are in
 * that band whenever a boundary falls inside it, and the rule for choosing
 * between them was: take the last one in file order, because scrolling down
 * means the lower of the two is the one being read.
 *
 * That rule is wrong immediately after a jump. Clicking a file in the sidebar
 * scrolls its header to the top of the band; a short file then does not fill
 * the band on its own, the next file's section reaches into it, and the last
 * rule hands the active state to the file below the one that was clicked.
 * Measured on the visual fixture in Chromium: clicking file 0 left the active
 * row on file 1, clicking file 1 left it on file 2, and clicking file 3 - the
 * first file long enough to fill the band by itself - was correct. It is not a
 * race; waiting first does not change it.
 *
 * So a jump is remembered and wins for as long as the file jumped to is still
 * in the band. Once the reader scrolls off it the jump says nothing about what
 * they are looking at any more, and the scroll rule takes over again.
 */
export type ActiveState = {
  /** The section to paint as active, or null when there are none. */
  key: string | null
  /** The jump still in force, to be carried into the next call. */
  jumpedTo: string | null
}

export function nextActive(
  order: readonly string[],
  intersecting: ReadonlySet<string>,
  jumpedTo: string | null,
): ActiveState {
  if (jumpedTo !== null) {
    if (intersecting.has(jumpedTo)) return { key: jumpedTo, jumpedTo }
    jumpedTo = null
  }
  let found: string | null = null
  for (const key of order) {
    if (intersecting.has(key)) found = key
  }
  return { key: found ?? order[0] ?? null, jumpedTo }
}
