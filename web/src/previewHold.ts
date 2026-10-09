/**
 * The range of the preview selection sbnn is holding, if any.
 *
 * PreviewSelection keeps a selection (the '+' button, then its comment form)
 * alive until the reader is done with it. The stacks unmount the body of a
 * section that has scrolled far away, which would take that selection with it,
 * so they ask here before they do. A module variable rather than props: the
 * selection serves the whole page and is owned far from the stacks.
 */
let held: Range | null = null

export function setHeldPreviewRange(range: Range | null): void {
  held = range
}

export function heldPreviewRange(): Range | null {
  return held
}
