import type { StaticPayload } from './client'
import { groupFromLocation } from './api'

/** BASE is what the tool is called, and the title index.html ships with. */
const BASE = 'sbnn'

/**
 * pageTitle is the tab name for a review of the given group.
 *
 * Every review used to be called "sbnn", so a reader with three of them open
 * - the point of groups - saw three identical tabs and had to guess. The
 * group name is the one word that tells them apart, so it leads; the tool
 * name stays behind it because a narrowed tab shows the front of the string.
 * The unnamed group is left as plain "sbnn": "default" names nothing and
 * only costs the reader the characters a narrow tab has to spend.
 */
export function pageTitle(group: string): string {
  const name = group.trim()
  if (name === '' || name === 'default') return BASE
  return `${name} · ${BASE}`
}

/**
 * isGeneratedTitle reports whether the title already in the document is one
 * nobody chose on purpose.
 *
 * An exported page carries a title written by Go: `sbnn export --title` puts
 * the reader's own words there, and that must survive. Only the two titles
 * nothing picked - index.html's placeholder and export's `sbnn review: x`
 * fallback - are ours to replace.
 */
export function isGeneratedTitle(current: string, group: string): boolean {
  const title = current.trim()
  return (
    title === '' ||
    title === BASE ||
    title === `${BASE} review: ${group}` ||
    title === pageTitle(group)
  )
}

/**
 * currentGroup is the group this page is showing. An exported page carries
 * the name in its payload; a live one has it in the path, which is the only
 * thing the server routes on.
 */
export function currentGroup(): string {
  const payload = (window as Window & { __SBNN_DATA__?: StaticPayload }).__SBNN_DATA__
  const group = payload?.group
  if (typeof group === 'string' && group !== '') return group
  return groupFromLocation()
}

/** applyPageTitle names the tab after the review it is showing. */
export function applyPageTitle(): void {
  const group = currentGroup()
  if (!isGeneratedTitle(document.title, group)) return
  document.title = pageTitle(group)
}
