/**
 * Keeping what did not change the same object (#376).
 *
 * Every SSE event makes the page fetch the whole group again, and a fetch
 * returns freshly parsed objects for every round, file and comment. React
 * decides what to redo by comparing props with ===, so a reload that changed
 * one comment still handed every file section a "new" file, a "new" diff and a
 * "new" comment list, and the page rendered all of them again: with 400 files
 * that was 670 ms of main thread per event and a growing heap (measured in
 * #376). Reusing the previous object wherever the content is the same lets
 * everything below stay put.
 */
import type { Comment, Diff } from './types'

/**
 * shareDiffs returns next with every round the page already holds replaced by
 * the object it holds.
 *
 * A diff never changes once the server has accepted it - a different review is
 * a different id - so the id is the identity, and the title, size of the raw
 * diff and number of files are only there to notice a server that broke that
 * promise. When nothing at all changed, prev itself is returned so that
 * setState bails out and nothing renders.
 */
export function shareDiffs(prev: Diff[], next: Diff[]): Diff[] {
  const byId = new Map(prev.map((d) => [d.id, d]))
  const merged = next.map((d) => {
    const old = byId.get(d.id)
    const same =
      old !== undefined &&
      old.title === d.title &&
      old.raw.length === d.raw.length &&
      old.files.length === d.files.length &&
      old.createdAt === d.createdAt
    return same ? old : d
  })
  return sameElements(prev, merged) ? prev : merged
}

/**
 * shareComments is shareDiffs for comments, which do change (an edit, a
 * resolve), so a comment is reused only when every field is the same.
 */
export function shareComments(prev: Comment[], next: Comment[]): Comment[] {
  const byId = new Map(prev.map((c) => [c.id, c]))
  const merged = next.map((c) => {
    const old = byId.get(c.id)
    return old !== undefined && JSON.stringify(old) === JSON.stringify(c) ? old : c
  })
  return sameElements(prev, merged) ? prev : merged
}

/**
 * shareGroups rebuilds a Map of lists keyed by section so that a key whose
 * list holds the same objects as before keeps the list it had. Without it
 * every file section gets a new (even if empty) array on every render.
 */
export function shareGroups<T>(prev: Map<string, T[]>, next: Map<string, T[]>): Map<string, T[]> {
  for (const [key, list] of next) {
    const old = prev.get(key)
    if (old !== undefined && sameElements(old, list)) next.set(key, old)
  }
  return next
}

function sameElements<T>(a: T[], b: T[]): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i])
}
