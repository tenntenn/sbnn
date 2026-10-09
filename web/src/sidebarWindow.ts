/**
 * Which rows of the sidebar's file list are on the page.
 *
 * Every row is about nine DOM nodes, so 3000 files were 28,000 nodes that the
 * browser walks for style, pre-paint and garbage collection on every event,
 * whether or not a single row changed (#404). The list is cut into chunks of
 * ROW_CHUNK rows; a chunk far from the sidebar's viewport is an empty list
 * standing as tall as its rows were, and one near it holds the rows. A review
 * small enough to fit a few screens (WINDOW_MIN_ROWS) is not windowed at all,
 * so what it renders is what it always rendered.
 *
 * Nothing about the rows' state lives in the DOM: the read marks, the active
 * file and the keyboard walk are App's, so a chunk coming and going loses
 * nothing - apart from focus, which is why a chunk holding the focused
 * element stays (see Sidebar.tsx).
 */

/** ROW_CHUNK is how many rows are mounted or dropped together. */
export const ROW_CHUNK = 25

/** WINDOW_MIN_ROWS is the number of files above which the list is windowed. */
export const WINDOW_MIN_ROWS = 200

/** DEFAULT_ROW_HEIGHT is what a row measures before any has been measured. */
export const DEFAULT_ROW_HEIGHT = 29

/** NEAR_MARGIN is how far past the sidebar's viewport a chunk still counts as
 * near: about forty rows each way, so a wheel flick does not outrun the rows. */
export const NEAR_MARGIN = '1200px 0px 1200px 0px'

/** isWindowed says whether a list of this many files is windowed. */
export function isWindowed(files: number): boolean {
  return files > WINDOW_MIN_ROWS
}

/** chunkStarts is the index of the first row of each chunk of `count` rows. */
export function chunkStarts(count: number): number[] {
  const starts: number[] = []
  for (let i = 0; i < count; i += ROW_CHUNK) starts.push(i)
  return starts
}

/**
 * placeholderHeight is how tall an unmounted chunk stands: what it measured
 * while its rows were there, else the rows it holds at the row height seen so
 * far. A measurement of 0 is a list that was hidden, which says nothing.
 */
export function placeholderHeight(
  measured: number | undefined,
  rows: number,
  rowHeight: number,
): number {
  if (measured !== undefined && measured > 0) return measured
  return Math.round(rows * rowHeight)
}

/** Near tells chunks when they are near the sidebar's viewport. */
export interface Near {
  /** watch calls `onChange` with whether `el` is near, now and on each change;
   * the result stops it. */
  watch: (el: Element, onChange: (near: boolean) => void) => () => void
  /** rowHeight is the height of one row as last measured. */
  rowHeight: number
  /** learn records a measured chunk: `height` for `rows` rows. */
  learn: (height: number, rows: number) => void
  /** dispose lets go of the observer. */
  dispose: () => void
}

/** createNear makes the shared observer for one sidebar. `getRoot` is the
 * scrolling element, asked for when the first chunk starts watching. */
export function createNear(getRoot: () => Element | null): Near {
  const watched = new Map<Element, (near: boolean) => void>()
  let observer: IntersectionObserver | null = null
  const near: Near = {
    rowHeight: DEFAULT_ROW_HEIGHT,
    watch(el, onChange) {
      if (typeof IntersectionObserver === 'undefined') return () => {}
      if (!observer) {
        observer = new IntersectionObserver(
          (entries) => {
            for (const entry of entries) watched.get(entry.target)?.(entry.isIntersecting)
          },
          { root: getRoot(), rootMargin: NEAR_MARGIN, threshold: 0 },
        )
      }
      watched.set(el, onChange)
      observer.observe(el)
      return () => {
        watched.delete(el)
        observer?.unobserve(el)
      }
    },
    learn(height, rows) {
      if (rows > 0 && height > 0) near.rowHeight = height / rows
    },
    dispose() {
      observer?.disconnect()
      observer = null
      watched.clear()
    },
  }
  return near
}
