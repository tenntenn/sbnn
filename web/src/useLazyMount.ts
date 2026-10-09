import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import {
  KEEP_MARGIN,
  NEAR_MARGIN,
  holdsState,
  initialMounted,
  reconcileMounted,
} from './lazySections'
import { heldPreviewRange } from './previewHold'

/** sectionHoldsState reads the page for what unmounting `el` would lose. */
function sectionHoldsState(el: HTMLElement): boolean {
  const range = heldPreviewRange()
  return holdsState({
    focusWithin: el.contains(document.activeElement),
    hasField: el.querySelector('textarea') !== null,
    hasSelectedRows: el.querySelector('tr.selected, td.selected') !== null,
    holdsPreviewSelection: range !== null && el.contains(range.commonAncestorContainer),
  })
}

export interface LazyMount {
  /** mounted holds the keys of the sections whose body is on the page. */
  mounted: ReadonlySet<string>
  /** ensure mounts a section's body ahead of the observers, for a jump that
   * is about to land on it. */
  ensure: (key: string) => void
}

/**
 * useLazyMount decides which sections of a stack have a body on the page, from
 * how far each one is from the pane's viewport. See ../lazySections for the
 * rule; this is the part that watches the page.
 *
 * `order` is the keys in reading order and `getEl` finds a section's shell.
 */
export function useLazyMount(
  containerRef: RefObject<HTMLElement | null>,
  order: readonly string[],
  getEl: (key: string) => HTMLElement | undefined,
): LazyMount {
  const [mounted, setMounted] = useState<ReadonlySet<string>>(() => initialMounted(order))
  const near = useRef(new Set<string>())
  const keepBand = useRef(new Set<string>())
  const orderRef = useRef(order)
  orderRef.current = order
  const getElRef = useRef(getEl)
  getElRef.current = getEl

  // False from the moment the observers are rebuilt until both have reported.
  const ready = useRef(false)

  const reconcile = useCallback(() => {
    if (!ready.current) return
    setMounted((previous) =>
      reconcileMounted(previous, orderRef.current, near.current, keepBand.current, (key) => {
        const el = getElRef.current(key)
        return el ? sectionHoldsState(el) : false
      }),
    )
  }, [])

  useEffect(() => {
    const root = containerRef.current
    if (!root) return
    // Without IntersectionObserver there is nothing to measure distance with,
    // and the safe answer is the old behaviour: everything mounted.
    if (typeof IntersectionObserver === 'undefined') {
      setMounted(new Set(order))
      return
    }
    // Each observer's first callback reports every section it watches. Until
    // both have, one band is still empty, and judging by it would unmount
    // sections that are merely waiting to be reported.
    let reported = 0
    ready.current = false
    const watch = (margin: string, into: Set<string>) => {
      let first = true
      return new IntersectionObserver(
        (entries) => {
          if (first) {
            first = false
            reported++
          }
          for (const entry of entries) {
            const key = (entry.target as HTMLElement).dataset.sectionKey
            if (!key) continue
            if (entry.isIntersecting) into.add(key)
            else into.delete(key)
          }
          if (reported === 2) {
            ready.current = true
            reconcile()
          }
        },
        { root, rootMargin: margin, threshold: 0 },
      )
    }
    near.current = new Set()
    keepBand.current = new Set()
    const nearObserver = watch(NEAR_MARGIN, near.current)
    const keepObserver = watch(KEEP_MARGIN, keepBand.current)
    for (const key of order) {
      const el = getElRef.current(key)
      if (!el) continue
      nearObserver.observe(el)
      keepObserver.observe(el)
    }
    return () => {
      nearObserver.disconnect()
      keepObserver.disconnect()
    }
  }, [containerRef, order, reconcile])

  // A section that was held while it was far away is let go of once the
  // reader is done with it: focus leaving it is the moment to look again.
  useEffect(() => {
    const again = () => window.setTimeout(reconcile, 0)
    document.addEventListener('focusout', again)
    return () => document.removeEventListener('focusout', again)
  }, [reconcile])

  const ensure = useCallback((key: string) => {
    setMounted((previous) => (previous.has(key) ? previous : new Set(previous).add(key)))
  }, [])

  return { mounted, ensure }
}
