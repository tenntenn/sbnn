import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { client } from '../client'
import { CommentForm } from './CommentThread'

interface Props {
  group: string
  onChanged: () => void
}

/**
 * HIGHLIGHT is the name the preview's own selection highlight is registered
 * under; the stylesheet paints it through ::highlight().
 *
 * The highlight is sbnn's rather than the browser's on purpose. Opening the
 * comment form focuses its textarea, and focusing anything at all collapses
 * the document selection - so a native highlight cannot survive the very
 * action it exists to start. Owning the highlight is what lets the selection
 * stay put from the moment the drag ends until the reader clears it.
 */
const HIGHLIGHT = 'sbnn-selection'

/** GAP is how far the menu sits from the point the pointer let go, and
 * MARGIN how close to the edge of the window it may come. */
const GAP = 12
const MARGIN = 8

/** Capture is a selection sbnn has taken ownership of: which lines of which
 * file it covers, the range it is painted over, and where it was released. */
interface Capture {
  /** clip is the pane the preview is seen through, so the menu can be put
   * away once the text it belongs to has been scrolled out of it. */
  clip: HTMLElement | null
  diffId: string
  fileId: string
  path: string
  startLine: number
  endLine: number
  text: string
  range: Range
  /** rectIndex, dx and dy put the release point back where it belongs after
   * the preview scrolls: it is remembered as an offset into one of the
   * selection's own client rects, not as a viewport coordinate, so the menu
   * travels with the text it is about. rectIndex is -1 when the selection
   * had no client rects to hang it on. */
  rectIndex: number
  dx: number
  dy: number
  /** drafting is set once the reader has pressed the button and the form has
   * taken over from it. */
  drafting: boolean
}

interface Point {
  x: number
  y: number
}

/** highlights is the registry, or null on a browser without the CSS Custom
 * Highlight API. There the selection simply is not painted; commenting from
 * it still works. */
function highlights(): { set(name: string, value: Highlight): void; delete(name: string): void } | null {
  if (typeof Highlight === 'undefined') return null
  const registry = CSS.highlights as unknown as
    | { set(name: string, value: Highlight): void; delete(name: string): void }
    | undefined
  return registry ?? null
}

/** anchoredRoot is the preview body node sits in, if that preview is one a
 * comment can anchor to. Only sbnn's own renderer marks itself as anchored:
 * mo renders in a frame sbnn may not read, and a partial preview marks the
 * gaps it skipped rather than keeping the file's numbering across them, so
 * neither can say which line a selection is on. */
function anchoredRoot(node: Node | null): HTMLElement | null {
  const el = node instanceof Element ? node : (node?.parentElement ?? null)
  return el?.closest<HTMLElement>('.markdown[data-line-anchored]') ?? null
}

/** scrollClip is the nearest ancestor that actually scrolls, which is the box
 * the preview is seen through. The preview's own element is not it: in the
 * stacked layout it grows to its full height and the pane around it scrolls
 * instead. Null when nothing between here and the window scrolls, where the
 * window is the box. */
function scrollClip(node: Node): HTMLElement | null {
  let el = node instanceof Element ? node : node.parentElement
  while (el) {
    const overflow = getComputedStyle(el).overflowY
    if (el.scrollHeight > el.clientHeight + 1 && (overflow === 'auto' || overflow === 'scroll')) {
      return el as HTMLElement
    }
    el = el.parentElement
  }
  return null
}

function blockLines(block: HTMLElement): [number, number] | null {
  const raw = block.dataset.ln
  if (!raw) return null
  const [start, end] = raw.split('-').map(Number)
  if (!Number.isFinite(start)) return null
  return [start, Number.isFinite(end) ? end : start]
}

/** textOf is the part of range that lies inside block. */
function textOf(range: Range, block: HTMLElement): string {
  const whole = document.createRange()
  whole.selectNodeContents(block)
  const clipped = range.cloneRange()
  if (clipped.compareBoundaryPoints(Range.START_TO_START, whole) < 0) {
    clipped.setStart(whole.startContainer, whole.startOffset)
  }
  if (clipped.compareBoundaryPoints(Range.END_TO_END, whole) > 0) {
    clipped.setEnd(whole.endContainer, whole.endOffset)
  }
  return clipped.toString()
}

/** selectedLines is the source line range the selection covers, taken from
 * the data-ln of every block it holds text of.
 *
 * Blocks it merely touches do not count: dragging past the end of a paragraph
 * leaves the selection ending at offset 0 of the next one, which the reader
 * never sees as selected and would not expect to be commenting on.
 */
function selectedLines(range: Range, root: HTMLElement): [number, number] | null {
  let start = Infinity
  let end = -Infinity
  for (const block of root.querySelectorAll<HTMLElement>('[data-ln]')) {
    if (!range.intersectsNode(block)) continue
    if (textOf(range, block).trim() === '') continue
    const lines = blockLines(block)
    if (!lines) continue
    start = Math.min(start, lines[0])
    end = Math.max(end, lines[1])
  }
  return start <= end ? [start, end] : null
}

/** rectsOf drops the empty rects a range collects at block boundaries, so
 * that an index into what is left stays meaningful. */
function rectsOf(range: Range): DOMRect[] {
  return Array.from(range.getClientRects()).filter((r) => r.width > 0 || r.height > 0)
}

function distanceTo(rect: DOMRect, point: Point): number {
  const dx = Math.max(rect.left - point.x, 0, point.x - rect.right)
  const dy = Math.max(rect.top - point.y, 0, point.y - rect.bottom)
  return Math.hypot(dx, dy)
}

/** anchorTo remembers point as an offset into whichever line of the selection
 * it came down on. */
function anchorTo(range: Range, point: Point): Pick<Capture, 'rectIndex' | 'dx' | 'dy'> {
  const rects = rectsOf(range)
  if (rects.length === 0) {
    const box = range.getBoundingClientRect()
    return { rectIndex: -1, dx: point.x - box.left, dy: point.y - box.top }
  }
  let index = 0
  let best = Infinity
  rects.forEach((rect, i) => {
    const d = distanceTo(rect, point)
    if (d < best) {
      best = d
      index = i
    }
  })
  return { rectIndex: index, dx: point.x - rects[index].left, dy: point.y - rects[index].top }
}

/** anchorPoint is where the menu belongs now, from where the pointer was
 * released then and where that line of text has moved to since. */
function anchorPoint(capture: Capture): Point {
  const rects = rectsOf(capture.range)
  const rect =
    capture.rectIndex >= 0 && rects.length > 0
      ? rects[Math.min(capture.rectIndex, rects.length - 1)]
      : capture.range.getBoundingClientRect()
  return { x: rect.left + capture.dx, y: rect.top + capture.dy }
}

/** pointOf is where a press or a release happened. TouchEvent is not a
 * global everywhere, so the event is told apart by what it carries rather
 * than by what it is an instance of. */
function pointOf(ev: Event): Point | null {
  if ('changedTouches' in ev) {
    const touch = (ev as TouchEvent).changedTouches[0]
    return touch ? { x: touch.clientX, y: touch.clientY } : null
  }
  if ('clientX' in ev) {
    const mouse = ev as MouseEvent
    return { x: mouse.clientX, y: mouse.clientY }
  }
  return null
}

/** within says whether point can still be seen through clip, or through the
 * window when there is no clip. */
function within(point: Point, clip: HTMLElement | null): boolean {
  const box = clip?.getBoundingClientRect()
  const top = Math.max(box?.top ?? 0, 0)
  const bottom = Math.min(box?.bottom ?? window.innerHeight, window.innerHeight)
  const left = Math.max(box?.left ?? 0, 0)
  const right = Math.min(box?.right ?? window.innerWidth, window.innerWidth)
  return point.y >= top && point.y <= bottom && point.x >= left && point.x <= right
}

/** caretPoint is the far end of the selection - the end that just moved,
 * whether by a phone's selection handle or by shift and an arrow key. It
 * stands in for a pointer when there was none. */
function caretPoint(selection: Selection): Point | null {
  if (!selection.focusNode) return null
  const caret = document.createRange()
  caret.setStart(selection.focusNode, selection.focusOffset)
  caret.collapse(true)
  const rect = caret.getBoundingClientRect()
  if (rect.width === 0 && rect.height === 0) return null
  return { x: rect.left, y: rect.bottom }
}

/**
 * PreviewSelection turns a text selection in a Markdown preview into a
 * comment on the lines it covers.
 *
 * Releasing a drag hands the selection over to sbnn: it is painted with
 * sbnn's own highlight and stays there, through the form's textarea taking
 * focus and through anything else that would collapse a native selection,
 * until the reader clicks away, presses Escape, cancels, or posts. A button
 * appears where the pointer came up - not over the middle of the selected
 * text, which for anything more than a line or two is nowhere near the hand
 * that selected it - and opens into the same comment form the diff uses.
 *
 * One of these serves the whole page rather than one per file: a selection
 * can only be in one preview at a time, and the file it landed in is read
 * off that preview's own element.
 */
export function PreviewSelection({ group, onChanged }: Props) {
  const [capture, setCapture] = useState<Capture | null>(null)
  const menu = useRef<HTMLDivElement>(null)
  // The document level listeners below are attached once and read the
  // current capture through this, never through the closure they were
  // created in.
  const current = useRef<Capture | null>(null)
  current.current = capture

  /** clear puts the selection away, the browser's along with sbnn's. Leaving
   * the native one behind would leave the preview looking selected after the
   * reader has said they are done with it. It answers to nothing when sbnn
   * holds no selection, so a click that ends somewhere else on the page
   * cannot take away a selection made in the diff. */
  const clear = useCallback(() => {
    if (current.current === null) return
    window.getSelection()?.removeAllRanges()
    setCapture(null)
  }, [])

  /** take reads the selection as it stands and takes ownership of it,
   * answering whether there was one worth taking. point is where the pointer
   * let go, or null when there was no pointer involved. */
  const take = useCallback((point: Point | null): boolean => {
    const selection = window.getSelection()
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) return false
    const range = selection.getRangeAt(0)
    if (range.toString().trim() === '') return false
    const root = anchoredRoot(range.commonAncestorContainer)
    const { diffId, fileId, path } = root?.dataset ?? {}
    if (!root || !diffId || !fileId || !path) return false
    const lines = selectedLines(range, root)
    if (!lines) return false
    const at = point ?? caretPoint(selection)
    // A range outlives the selection it came from: the selection is about to
    // be collapsed by the first thing that takes focus, and this has to
    // survive that.
    const owned = range.cloneRange()
    const box = owned.getBoundingClientRect()
    setCapture({
      clip: scrollClip(root),
      diffId,
      fileId,
      path,
      startLine: lines[0],
      endLine: lines[1],
      text: range.toString(),
      range: owned,
      drafting: false,
      ...anchorTo(owned, at ?? { x: box.left, y: box.bottom }),
    })
    return true
  }, [])

  /** place puts the menu at the release point, measured rather than
   * guessed: the same element holds a small button and then a whole comment
   * form, and both have to stay on screen. */
  const place = useCallback(() => {
    const el = menu.current
    const capture = current.current
    if (!el || !capture) return
    // The preview was reloaded or replaced underneath: the range points at
    // nodes that are no longer on the page, so there is nothing left to
    // comment on and nowhere to put the menu.
    if (!capture.range.commonAncestorContainer.isConnected) {
      clear()
      return
    }
    const point = anchorPoint(capture)
    // Scrolled out of the pane: the button waits there rather than sliding
    // along the edge of a window whose text it is no longer next to. A form
    // being written into stays put instead - it holds what has been typed,
    // and taking that off the screen would be losing it.
    if (!capture.drafting && !within(point, capture.clip)) {
      el.style.visibility = 'hidden'
      return
    }
    const { width, height } = el.getBoundingClientRect()
    const left = Math.max(MARGIN, Math.min(point.x - width / 2, window.innerWidth - width - MARGIN))
    let top = point.y + GAP
    // Below the release point, where a menu opening downwards does not cover
    // what was just selected - unless it would not fit there.
    if (top + height > window.innerHeight - MARGIN) top = point.y - GAP - height
    top = Math.max(MARGIN, Math.min(top, window.innerHeight - height - MARGIN))
    el.style.left = `${left}px`
    el.style.top = `${top}px`
    el.style.visibility = 'visible'
  }, [clear])

  // Placed before the browser paints, so the menu is never seen at the
  // top left corner it starts at. It is re-placed whenever it changes size
  // (the button growing into the form, the textarea growing as it is typed
  // into) and whenever anything under it scrolls or the window is resized:
  // the menu is pinned to the viewport, so it has to be moved back over the
  // text it belongs to as that text moves.
  useLayoutEffect(() => {
    if (!capture) return
    place()
    const el = menu.current
    const resize = el ? new ResizeObserver(() => place()) : null
    if (el) resize?.observe(el)
    // Scrolling fires far faster than the screen is drawn, and placing the
    // menu reads layout, so it is done once per frame.
    let frame = 0
    const follow = () => {
      if (frame !== 0) return
      frame = window.requestAnimationFrame(() => {
        frame = 0
        place()
      })
    }
    // Capturing, and on the document: the preview is a scrolling pane inside
    // the page, and a scroll event on it does not bubble.
    document.addEventListener('scroll', follow, true)
    window.addEventListener('resize', follow)
    return () => {
      resize?.disconnect()
      document.removeEventListener('scroll', follow, true)
      window.removeEventListener('resize', follow)
      if (frame !== 0) window.cancelAnimationFrame(frame)
    }
  }, [capture, place])

  useEffect(() => {
    const registry = highlights()
    if (!registry) return
    if (!capture) {
      registry.delete(HIGHLIGHT)
      return
    }
    registry.set(HIGHLIGHT, new Highlight(capture.range))
    return () => registry.delete(HIGHLIGHT)
  }, [capture])

  useEffect(() => {
    // Whether the press that is now finishing came down on the menu itself,
    // and whether it came from a finger. A press on the menu is the reader
    // reaching for its button, not a click away from the selection.
    let onMenu = false
    let touched = false
    let settle: number | undefined

    const down = (ev: Event) => {
      const target = ev.target
      onMenu = target instanceof Node && menu.current !== null && menu.current.contains(target)
      touched = ev.type === 'touchstart'
      // A press that lands on text the browser still counts as selected
      // starts a drag of that text rather than a new selection - so a second
      // look at the same paragraph would select nothing at all. Nothing in a
      // preview is worth dragging anywhere, so the slate is wiped and every
      // press in one starts a selection.
      if (!onMenu && target instanceof Node && anchoredRoot(target)) {
        window.getSelection()?.removeAllRanges()
      }
    }

    // The selection is read when the pointer comes up, not while it moves.
    // Nothing is shown mid-drag, so there is nothing that has to be kept in
    // step with a selection that is still changing - the races that comes
    // with simply do not arise.
    const up = (ev: Event) => {
      if (onMenu) return
      // Once the form is open it owns the interaction: a click in the
      // preview behind it neither re-aims it nor throws away what has been
      // written. Cancel, Escape and posting are the ways out of it.
      if (current.current?.drafting) return
      // Nothing selected on the way up means this was a plain click, which
      // is how the reader puts a selection away.
      if (!take(pointOf(ev))) clear()
    }

    // A phone goes on adjusting the selection by its handles long after the
    // finger that started it has lifted, and fires no pointer event of its
    // own while doing so. Only then is selectionchange worth listening to,
    // and only to follow a selection sbnn already holds.
    const changed = () => {
      if (!touched) return
      const capture = current.current
      if (!capture || capture.drafting) return
      window.clearTimeout(settle)
      settle = window.setTimeout(() => {
        if (!take(null)) clear()
      }, 250)
    }

    // Shift and the arrow keys select too, and land on no pointer at all -
    // the menu goes to the end that moved. A plain Shift press that selects
    // nothing is not a dismissal, so this never clears.
    const keyUp = (ev: KeyboardEvent) => {
      if (current.current?.drafting) return
      if (!ev.shiftKey && ev.key !== 'Shift') return
      take(null)
    }

    const keyDown = (ev: KeyboardEvent) => {
      if (ev.key !== 'Escape') return
      const capture = current.current
      if (!capture) return
      // While the form holds the focus Escape is its own: it cancels, which
      // clears this in turn. Once the focus has gone elsewhere - a click in
      // the preview behind it leaves the form open but focused on nothing -
      // Escape is the way out again.
      if (capture.drafting && menu.current?.contains(document.activeElement)) return
      clear()
    }

    document.addEventListener('mousedown', down, true)
    document.addEventListener('touchstart', down, true)
    document.addEventListener('mouseup', up)
    document.addEventListener('touchend', up)
    document.addEventListener('selectionchange', changed)
    document.addEventListener('keyup', keyUp)
    document.addEventListener('keydown', keyDown)
    return () => {
      document.removeEventListener('mousedown', down, true)
      document.removeEventListener('touchstart', down, true)
      document.removeEventListener('mouseup', up)
      document.removeEventListener('touchend', up)
      document.removeEventListener('selectionchange', changed)
      document.removeEventListener('keyup', keyUp)
      document.removeEventListener('keydown', keyDown)
      window.clearTimeout(settle)
    }
  }, [take, clear])

  if (!capture) return null

  const label =
    `${capture.path}:${capture.startLine}` +
    (capture.endLine > capture.startLine ? `-${capture.endLine}` : '')

  const submit = async (body: string, question: boolean) => {
    await client.addComment(group, {
      diffId: capture.diffId,
      fileId: capture.fileId,
      path: capture.path,
      // A preview is the file as it is after the change, so a comment on it
      // is on the new side.
      side: 'new',
      startLine: capture.startLine,
      endLine: capture.endLine,
      body,
      question,
      snippet: capture.text,
    })
    clear()
    onChanged()
  }

  return (
    <div
      className="selection-menu"
      ref={menu}
      data-drafting={capture.drafting ? 'true' : undefined}
      onMouseDown={(ev) => {
        // Pressing the button must not also move the caret into the text
        // under it. The form is left alone: its textarea has to be able to
        // take focus, and to be selected within.
        if (!capture.drafting) ev.preventDefault()
      }}
    >
      {!capture.drafting ? (
        <button
          className="ghost selection-add"
          title={`Comment on ${label}`}
          aria-label={`Comment on ${label}`}
          onClick={() => setCapture((c) => (c ? { ...c, drafting: true } : c))}
        >
          +
        </button>
      ) : (
        <CommentForm
          label={label}
          seed={capture.text}
          canSuggest
          hint="Selected in the preview; the comment covers whole blocks"
          onSubmit={submit}
          onCancel={clear}
        />
      )}
    </div>
  )
}
