/**
 * isSelectingPress says whether a press can be the start or the end of a text
 * selection, as opposed to a press that merely points at one.
 *
 * Only the primary button selects. The secondary button (the context menu:
 * translate, read aloud, copy) and the middle button act on a selection that
 * already exists, so a handler that treated them as the start of a new
 * selection would wipe the very text the reader is about to act on. On macOS
 * Ctrl+click is the secondary button too, though it reports button 0. Touch
 * events carry no button and always count.
 */
export function isSelectingPress(
  ev: { type: string; button?: number; ctrlKey?: boolean },
  mac = false,
): boolean {
  if (ev.type.startsWith('touch')) return true
  if (mac && ev.ctrlKey) return false
  return (ev.button ?? 0) === 0
}

/** isMac says whether the page runs on macOS, where Ctrl+click opens the
 * context menu. */
export function isMac(): boolean {
  return typeof navigator !== 'undefined' && /^Mac/i.test(navigator.platform)
}
