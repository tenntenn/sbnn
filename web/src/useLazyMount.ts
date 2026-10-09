import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import {
  endsFind,
  findExpired,
  mountedWithFind,
  nextFindBatch,
  startsFind,
  FIND_QUIET_MS,
  isMac,
} from './findHold'
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
  /** mounted holds the keys of the sections whose body is on the page,
   * including those mounted for the browser's find-in-page (#398). */
  mounted: ReadonlySet<string>
  /** lazy is mounted without the find hold: the sections that are mounted
   * because they are near the reader. Content that is too heavy to mount for
   * a search (a preview iframe) follows this one. */
  lazy: ReadonlySet<string>
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
  const [lazy, setMounted] = useState<ReadonlySet<string>>(() => initialMounted(order))
  const near = useRef(new Set<string>())
  const keepBand = useRef(new Set<string>())
  const orderRef = useRef(order)
  orderRef.current = order
  const getElRef = useRef(getEl)
  getElRef.current = getEl

  // The section a jump is on its way to: mounted by ensure, and not to be
  // let go of by an observer callback that arrives before the scroll lands.
  const pinned = useRef<string | null>(null)
  // False from the moment the observers are rebuilt until both have reported.
  const ready = useRef(false)

  const reconcile = useCallback(() => {
    if (!ready.current) return
    setMounted((previous) =>
      reconcileMounted(previous, orderRef.current, near.current, keepBand.current, (key) => {
        if (key === pinned.current) return true
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

  // Find-in-page (#398). The browser's find bar only searches what is on the
  // page, and it sends the page nothing but the key that opened it, so that key
  // mounts the rest in batches and holds them until the reader goes quiet.
  const [found, setFound] = useState<ReadonlySet<string>>(() => new Set())
  const foundRef = useRef(found)
  foundRef.current = found
  const lazyRef = useRef(lazy)
  lazyRef.current = lazy
  // Set while a hold is on: schedules another batch, for when the review grows.
  const resumeFind = useRef<(() => void) | null>(null)
  useEffect(() => resumeFind.current?.(), [order])

  useEffect(() => {
    let active = false
    let last = 0
    let timer: number | undefined
    let job: number | undefined
    let jobIsIdle = false

    const cancelJob = () => {
      if (job === undefined) return
      if (jobIsIdle) window.cancelIdleCallback(job)
      else window.clearTimeout(job)
      job = undefined
    }
    const batch = () => {
      job = undefined
      if (!active) return
      const anchor = orderRef.current.findIndex((key) => near.current.has(key))
      // Sections the lazy rules already mounted need no slot of the batch.
      const have = new Set(foundRef.current)
      for (const key of lazyRef.current) have.add(key)
      const keys = nextFindBatch(orderRef.current, have, anchor < 0 ? 0 : anchor)
      if (keys.length === 0) return
      const next = new Set(foundRef.current)
      for (const key of keys) next.add(key)
      foundRef.current = next
      setFound(next)
      schedule()
    }
    const schedule = () => {
      if (typeof window.requestIdleCallback === 'function') {
        jobIsIdle = true
        job = window.requestIdleCallback(batch, { timeout: 250 })
      } else {
        jobIsIdle = false
        job = window.setTimeout(batch, 16)
      }
    }
    const touch = () => {
      last = Date.now()
    }
    const watchQuiet = () => {
      timer = window.setTimeout(() => {
        if (findExpired(Date.now(), last)) end()
        else watchQuiet()
      }, Math.max(1000, FIND_QUIET_MS - (Date.now() - last)))
    }
    const events = ['keydown', 'wheel', 'mousedown', 'mousemove', 'touchstart', 'scroll'] as const
    const end = () => {
      if (!active) return
      active = false
      cancelJob()
      window.clearTimeout(timer)
      for (const type of events) document.removeEventListener(type, touch, true)
      // What the reader did while everything was mounted must not vanish with
      // the hold: a section that holds state joins the lazy set.
      const keep = [...foundRef.current].filter((key) => {
        const el = getElRef.current(key)
        return el !== undefined && sectionHoldsState(el)
      })
      if (keep.length > 0) {
        setMounted((previous) => {
          const next = new Set(previous)
          for (const key of keep) next.add(key)
          return next
        })
      }
      foundRef.current = new Set()
      setFound(foundRef.current)
    }
    const begin = () => {
      touch()
      if (active) return
      active = true
      for (const type of events) document.addEventListener(type, touch, { capture: true, passive: true })
      watchQuiet()
      schedule()
    }
    resumeFind.current = () => {
      if (active && job === undefined) schedule()
    }
    const onKey = (e: KeyboardEvent) => {
      if (startsFind(e, isMac())) begin()
      else if (active && endsFind(e)) end()
    }
    window.addEventListener('keydown', onKey, true)
    return () => {
      window.removeEventListener('keydown', onKey, true)
      resumeFind.current = null
      end()
    }
  }, [])

  const mounted = useMemo(() => mountedWithFind(lazy, found), [lazy, found])

  const ensure = useCallback((key: string) => {
    pinned.current = key
    window.setTimeout(() => {
      if (pinned.current === key) pinned.current = null
    }, 1000)
    setMounted((previous) => (previous.has(key) ? previous : new Set(previous).add(key)))
  }, [])

  return { mounted, lazy, ensure }
}
