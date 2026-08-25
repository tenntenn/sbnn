/**
 * urlState keeps the address bar pointed at the file the reader is looking
 * at, and puts a reader who arrives on a link back at that file.
 *
 * It works on the document rather than on React state on purpose. Every file
 * section already carries a stable DOM id built from its section key
 * (`d1:f1-abcd1234`), so the anchor a link needs exists already and nothing
 * has to be threaded through the component tree to reach it. Reading the
 * page also means the exported page - which has no server and no router -
 * behaves exactly like the live one.
 *
 * The group is not touched: it lives in the path, which this never writes.
 */

/**
 * SECTION_ID is the shape of a file section's id: a diff id from the store's
 * counter, then the file's own id, which is its index in that diff plus the
 * first eight hex digits of its path's SHA-256. Matching the shape rather
 * than a class name keeps this independent of the stylesheet, and keeps ids
 * that belong to something else (`comment-local-1`, `root`) out.
 */
const SECTION_ID = /^d\d+:f\d+-[0-9a-f]{8}$/

/** How long a scroll burst is collapsed into one address bar write. */
const THROTTLE_MS = 150

/** How long, and how often, to wait for the sections to be rendered. */
const RETRY_MS = 100
const RETRY_LIMIT = 30

/** How long after a scroll of our own to leave the address bar alone. */
const QUIET_MS = 400

/** How far past the top of the pane a section may sit and still count as
 * the one being read. */
const ANCHOR_SLACK = 4

/** isSectionKey reports whether a string identifies one file of one round. */
export function isSectionKey(value: string): boolean {
  return SECTION_ID.test(value)
}

/**
 * keyFromHash reads a section key out of a URL fragment, or returns null
 * when the fragment is empty, is not a section key, or is not even a valid
 * escape sequence. A fragment nobody here wrote is left for whoever did.
 */
export function keyFromHash(hash: string): string | null {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  if (raw === '') return null
  let decoded = raw
  try {
    decoded = decodeURIComponent(raw)
  } catch {
    // A stray '%' is not an escape; take the fragment as it was written.
  }
  return isSectionKey(decoded) ? decoded : null
}

/**
 * hashForKey is the fragment that addresses a section, or '' for anything
 * that is not a section key. Section keys are made of unreserved characters
 * and ':', all of which a fragment may carry as they are, so escaping them
 * would only make the link harder to read and to paste.
 */
export function hashForKey(key: string): string {
  return isSectionKey(key) ? `#${key}` : ''
}

/** scrollParent is the box the element scrolls inside, or null for the page. */
function scrollParent(el: HTMLElement): HTMLElement | null {
  for (let node = el.parentElement; node; node = node.parentElement) {
    const overflow = getComputedStyle(node).overflowY
    if ((overflow === 'auto' || overflow === 'scroll') && node.scrollHeight > node.clientHeight) {
      return node
    }
  }
  return null
}

/**
 * sections are the file sections on screen, in the order they are read.
 *
 * The diff pane and the preview pane render the same files, so a key can be
 * in the document twice; the first of the pair wins, which is the diff. That
 * also leaves the tops increasing down the list, which the search below
 * relies on.
 */
function sections(): HTMLElement[] {
  const found: HTMLElement[] = []
  const seen = new Set<string>()
  for (const node of document.querySelectorAll<HTMLElement>('[id]')) {
    if (!isSectionKey(node.id) || seen.has(node.id)) continue
    if (node.getClientRects().length === 0) continue
    seen.add(node.id)
    found.push(node)
  }
  return found
}

/**
 * stickyOffset is how much of the top of the pane is already spoken for by
 * bars that stay there while the content scrolls under them. A section
 * parked at the very top would be sitting behind them.
 *
 * The bars are found by what they do - stuck to the top of the scroller, and
 * laid out before the section - rather than by what they are called, because
 * the stylesheet is not this lane's to depend on.
 */
function stickyOffset(el: HTMLElement, scroller: HTMLElement | null): number {
  let height = 0
  const stop = scroller ?? document.body
  for (let node: HTMLElement | null = el; node && node !== stop; node = node.parentElement) {
    for (let sib = node.previousElementSibling; sib; sib = sib.previousElementSibling) {
      const style = getComputedStyle(sib)
      if (style.position !== 'sticky') continue
      // 'auto' parses as NaN, and a bar parked lower down is one the section
      // scrolls past rather than under.
      if (!(parseFloat(style.top) <= 1)) continue
      height = Math.max(height, sib.getBoundingClientRect().height)
    }
  }
  return height
}

/** activeSectionKey is the file currently at the top of the pane. */
function activeSectionKey(list: HTMLElement[]): string | null {
  if (list.length === 0) return null
  const scroller = scrollParent(list[0])
  const paneTop = scroller ? scroller.getBoundingClientRect().top : 0
  const anchor = paneTop + stickyOffset(list[0], scroller) + ANCHOR_SLACK

  // The sections are in document order, so their tops only ever increase:
  // binary search for the last one that has already crossed the anchor
  // line. A 500 file review is a dozen measurements rather than 500.
  let lo = 0
  let hi = list.length - 1
  let last = 0
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (list[mid].getBoundingClientRect().top <= anchor) {
      last = mid
      lo = mid + 1
    } else {
      hi = mid - 1
    }
  }
  return list[last].id
}

/** scrollToSection puts a section under the toolbar rather than behind it. */
function scrollToSection(el: HTMLElement): void {
  const scroller = scrollParent(el)
  const offset = stickyOffset(el, scroller)
  const rect = el.getBoundingClientRect()
  if (scroller) {
    const top = rect.top - scroller.getBoundingClientRect().top + scroller.scrollTop - offset
    scroller.scrollTo({ top: Math.max(0, top), behavior: 'auto' })
  } else {
    window.scrollTo({ top: Math.max(0, rect.top + window.scrollY - offset), behavior: 'auto' })
  }
}

/**
 * startURLState begins reflecting the reader's position in the URL and
 * honouring the fragment a reader arrives with. It returns a function that
 * stops it again.
 */
export function startURLState(): () => void {
  let stopped = false
  let quietUntil = 0
  let restoring = false
  let throttle: number | undefined
  let retry: number | undefined
  let attempt = 0

  const write = () => {
    throttle = undefined
    if (stopped || restoring || Date.now() < quietUntil) return
    const key = activeSectionKey(sections())
    if (key === null) return
    const hash = hashForKey(key)
    if (hash === '' || hash === window.location.hash) return
    try {
      // replaceState, never pushState: a history entry per scroll tick would
      // bury the page the reader came from under hundreds of them and leave
      // the back button useless.
      window.history.replaceState(window.history.state, '', hash)
    } catch {
      // Some hosts refuse to rewrite their own URL - an exported page opened
      // from a sandboxed frame, say. Losing the address is not worth losing
      // the page over.
    }
  }

  const onScroll = () => {
    if (stopped || throttle !== undefined) return
    throttle = window.setTimeout(write, THROTTLE_MS)
  }

  const restore = () => {
    if (retry !== undefined) window.clearTimeout(retry)
    retry = undefined
    attempt = 0
    const key = keyFromHash(window.location.hash)
    // Nothing to go to: an empty, unknown or differently shaped fragment is
    // somebody else's, and the page is left exactly as it is.
    if (key === null) return
    restoring = true
    const step = () => {
      retry = undefined
      if (stopped) return
      const el = sections().find((section) => section.id === key)
      if (el) {
        scrollToSection(el)
        // The scroll we just made would otherwise be read back as the reader
        // moving, and rewrite the fragment before the page has settled.
        quietUntil = Date.now() + QUIET_MS
        restoring = false
        return
      }
      // The sections are rendered after this starts, and a large review takes
      // a moment to lay out. Give up quietly rather than hold the address bar.
      if (++attempt >= RETRY_LIMIT) {
        restoring = false
        return
      }
      retry = window.setTimeout(step, RETRY_MS)
    }
    step()
  }

  // Scroll does not bubble, but it does capture, so one listener on the
  // window hears whichever pane is scrolling without this having to know
  // which panes exist or wait for them to be mounted.
  window.addEventListener('scroll', onScroll, { capture: true, passive: true })
  window.addEventListener('hashchange', restore)
  window.addEventListener('popstate', restore)
  restore()

  return () => {
    stopped = true
    if (throttle !== undefined) window.clearTimeout(throttle)
    if (retry !== undefined) window.clearTimeout(retry)
    window.removeEventListener('scroll', onScroll, { capture: true })
    window.removeEventListener('hashchange', restore)
    window.removeEventListener('popstate', restore)
  }
}
