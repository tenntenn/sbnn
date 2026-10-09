/**
 * isSelectingPress says whether a press can be the start or the end of a text
 * selection, as opposed to a press that merely points at one.
 *
 * Only the primary button selects. The secondary button (the context menu:
 * translate, read aloud, copy) and the middle button act on a selection that
 * already exists, so a handler that treated them as the start of a new
 * selection would wipe the very text the reader is about to act on. Touch
 * events carry no button and always count.
 */
export function isSelectingPress(ev: { type: string; button?: number }): boolean {
  if (ev.type.startsWith('touch')) return true
  return (ev.button ?? 0) === 0
}
