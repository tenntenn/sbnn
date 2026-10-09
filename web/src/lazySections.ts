/**
 * Which file sections keep their body in the DOM.
 *
 * A review of four hundred files used to mount every section's body - the
 * diff table, the rendered Markdown - and keep it mounted: 282,000 DOM nodes
 * and about four seconds of long tasks on load (#390). A section is a shell
 * (its element, its id, its height) plus a body. The shell is always there, so
 * scrolling, the sidebar jump and the active-file tracking see the same page
 * as before; the body is only there while the section is within a few
 * viewports of the one being read.
 *
 * Two bands decide it, like a thermostat. A body is mounted when its section
 * comes within NEAR_MARGIN of the pane and is only unmounted again once it is
 * beyond KEEP_MARGIN, so a reader hovering at the edge of the first band does
 * not mount and unmount the same section on every wheel tick. Both are shares
 * of the pane's height, which IntersectionObserver accepts as a percentage.
 */
export const NEAR_MARGIN = '200% 0px 200% 0px'
export const KEEP_MARGIN = '400% 0px 400% 0px'

/** INITIAL_MOUNTED is how many sections, from the top, are mounted before the
 * observers have reported anything, so the first paint is never an empty pane. */
export const INITIAL_MOUNTED = 4

/** initialMounted is the set of sections mounted before anything is known. */
export function initialMounted(order: readonly string[]): Set<string> {
  return new Set(order.slice(0, INITIAL_MOUNTED))
}

/**
 * Facts about a section's DOM that make unmounting it lose something the
 * reader did. They are read off the page by the caller; this module only
 * decides what they add up to.
 */
export interface HeldFacts {
  /** The focused element is inside the section (a comment form being typed in). */
  focusWithin: boolean
  /** The section holds a text field: an open draft or an edit in progress,
   * which keeps what has been typed even when it is not focused. */
  hasField: boolean
  /** The section has selected rows, the start of a comment on lines. */
  hasSelectedRows: boolean
  /** sbnn's own selection in a preview (the '+' button or its form) is in the
   * section. */
  holdsPreviewSelection: boolean
}

/** holdsState reports whether unmounting the section's body would lose
 * something the reader has not finished with. */
export function holdsState(facts: HeldFacts): boolean {
  return (
    facts.focusWithin || facts.hasField || facts.hasSelectedRows || facts.holdsPreviewSelection
  )
}

/**
 * nextMounted decides one section.
 *
 * It is mounted when it is near, stays mounted while it is still within the
 * keep band, and is never taken away while it holds state.
 */
export function nextMounted(
  wasMounted: boolean,
  near: boolean,
  inKeepBand: boolean,
  held: boolean,
): boolean {
  if (near) return true
  if (!wasMounted) return false
  return inKeepBand || held
}

/**
 * reconcileMounted applies nextMounted to a whole review.
 *
 * It answers the very set it was given when nothing changed, so React can
 * skip the render, and it only looks at the sections that are mounted or near:
 * of four hundred files that is a few dozen. Keys that are no longer in the
 * review drop out.
 */
export function reconcileMounted(
  previous: ReadonlySet<string>,
  order: readonly string[],
  near: ReadonlySet<string>,
  keepBand: ReadonlySet<string>,
  held: (key: string) => boolean,
): ReadonlySet<string> {
  const next = new Set<string>()
  for (const key of order) {
    const was = previous.has(key)
    if (!was && !near.has(key)) continue
    if (nextMounted(was, near.has(key), keepBand.has(key), was && !near.has(key) && held(key))) {
      next.add(key)
    }
  }
  if (next.size === previous.size) {
    let same = true
    for (const key of next) {
      if (!previous.has(key)) {
        same = false
        break
      }
    }
    if (same) return previous
  }
  return next
}

/**
 * placeholderHeight is how tall an unmounted section stands.
 *
 * What the section measured the last time its body was on the page, so
 * unmounting it moves nothing; the estimate before it ever was.
 */
export function placeholderHeight(measured: number | undefined, estimate: number): number {
  if (measured !== undefined && measured > 0) return measured
  return Math.max(0, estimate)
}
