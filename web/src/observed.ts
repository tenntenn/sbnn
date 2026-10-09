/**
 * Keeping an IntersectionObserver pointed at the right elements without
 * starting over (#402).
 *
 * The stacks used to disconnect their observers and observe every section
 * again whenever a round arrived: with 3000 sections that is 3000 observe calls
 * on each of several observers, for a change that added two sections. Each
 * observer is now kept for as long as the pane lives, and this brings the set
 * of elements it watches up to date: only what was added is observed, and only
 * what is gone is let go of.
 */

/** Observing is the part of IntersectionObserver this needs. */
export interface Observing {
  observe(el: Element): void
  unobserve(el: Element): void
}

/**
 * syncObserved makes `watched` - the element each key is observed through now -
 * match the elements `getEl` finds for `keys`, observing and unobserving the
 * difference. A key whose element was replaced is moved over to the new one.
 *
 * It returns the keys it stopped watching - a key that is gone, or one whose
 * element was replaced - so that a caller holding state per key (which
 * sections are near the viewport) can drop it: an unobserved element never
 * reports leaving, and the new element reports afresh.
 */
export function syncObserved<E extends Element>(
  observer: Observing,
  watched: Map<string, E>,
  keys: readonly string[],
  getEl: (key: string) => E | undefined,
): string[] {
  const wanted = new Set(keys)
  const gone: string[] = []
  for (const [key, el] of watched) {
    if (wanted.has(key) && getEl(key) === el) continue
    observer.unobserve(el)
    watched.delete(key)
    gone.push(key)
  }
  for (const key of keys) {
    if (watched.has(key)) continue
    const el = getEl(key)
    if (!el) continue
    observer.observe(el)
    watched.set(key, el)
  }
  return gone
}
