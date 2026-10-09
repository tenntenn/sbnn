/**
 * Keeping every section's body on the page while the reader searches it.
 *
 * Sections far from the viewport are only a shell (#390), so the browser's own
 * find-in-page (Ctrl/Cmd+F) cannot see their text (#398). There is no event for
 * the find bar, but the key that opens it reaches the page as a keydown. On it
 * the stacks mount the remaining bodies in small batches, so the main thread
 * is never blocked for long, hold them while the reader is plausibly still
 * using the find bar, and fall back to the lazy rules afterwards.
 *
 * This module is the decisions; useLazyMount is the part that watches the page.
 */

/** FIND_BATCH is how many bodies are mounted per idle slot. A body is about
 * 700 DOM nodes and ten milliseconds of render here, so four of them stay well
 * inside one frame budget plus a little. */
export const FIND_BATCH = 4

/** FIND_QUIET_MS is how long the page has to see no key, wheel, scroll or
 * pointer activity before the hold is let go. The find bar sends the page
 * nothing while the reader types in it, but moving to a match scrolls the
 * page, which counts. */
export const FIND_QUIET_MS = 60_000

/** The parts of a KeyboardEvent the rules look at. */
export interface KeyFacts {
  key: string
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
}

/** startsFind reports whether the key press opens or advances the browser's
 * find: Ctrl/Cmd+F, Ctrl/Cmd+G (next match) and F3. */
export function startsFind(e: KeyFacts): boolean {
  if (e.altKey) return false
  if (e.key === 'F3') return true
  if (!e.ctrlKey && !e.metaKey) return false
  const k = e.key.toLowerCase()
  return k === 'f' || k === 'g'
}

/** endsFind reports whether the key press is the reader leaving the search. */
export function endsFind(e: KeyFacts): boolean {
  return e.key === 'Escape' && !e.ctrlKey && !e.metaKey && !e.altKey
}

/** findExpired reports whether the page has been quiet for long enough. */
export function findExpired(now: number, lastActivity: number, quietMs: number = FIND_QUIET_MS): boolean {
  return now - lastActivity >= quietMs
}

/**
 * nextFindBatch picks the next sections to mount: those in `order` that are
 * not in `have`, nearest to `anchor` (an index in `order`) first, at most
 * `size` of them. Nearest first because the browser's find starts at the
 * reader's position, so the first matches are usually close to it.
 */
export function nextFindBatch(
  order: readonly string[],
  have: ReadonlySet<string>,
  anchor: number,
  size: number = FIND_BATCH,
): string[] {
  if (size <= 0 || order.length === 0) return []
  const start = Math.min(Math.max(anchor, 0), order.length - 1)
  const out: string[] = []
  for (let d = 0; out.length < size; d++) {
    const before = start - d
    const after = start + d
    if (before < 0 && after >= order.length) break
    if (before >= 0 && !have.has(order[before])) out.push(order[before])
    if (d > 0 && after < order.length && out.length < size && !have.has(order[after])) out.push(order[after])
  }
  return out
}

/** mountedWithFind is what the stacks render: the lazily mounted sections plus
 * those mounted for the search. It answers `lazy` itself when the search holds
 * nothing, so React can skip the render. */
export function mountedWithFind(
  lazy: ReadonlySet<string>,
  found: ReadonlySet<string>,
): ReadonlySet<string> {
  if (found.size === 0) return lazy
  const all = new Set(lazy)
  for (const key of found) all.add(key)
  return all
}
