import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { client } from '../client'
import { useT } from '../i18n'
import { placeCard, TOUR_STEPS, type Rect, type TourStep } from '../tour'

interface Props {
  /** onClose is called when the reader finishes or skips; the caller
   * remembers that, and takes the tour off the page. */
  onClose: () => void
}

/** SPOT_PAD is how far the highlight stands off the element it frames. */
const SPOT_PAD = 4

/** shown finds the element a step points at, if one of its targets is on
 * screen: laid out, with a size, and not inside a pane that has been put away
 * (a collapsed pane keeps its width at zero and says aria-hidden). */
function shown(step: TourStep): HTMLElement | null {
  for (const selector of step.targets) {
    const el = document.querySelector<HTMLElement>(selector)
    if (!el || el.closest('[aria-hidden="true"]')) continue
    const box = el.getBoundingClientRect()
    if (box.width > 0 && box.height > 0) return el
  }
  return null
}

function rectOf(el: HTMLElement): Rect {
  const box = el.getBoundingClientRect()
  return { top: box.top, left: box.left, width: box.width, height: box.height }
}

/** clip keeps the highlight inside the window: a pane taller than the window
 * would otherwise draw its frame off screen, where nobody sees it. */
function clip(rect: Rect): Rect {
  const top = Math.max(rect.top - SPOT_PAD, 0)
  const left = Math.max(rect.left - SPOT_PAD, 0)
  const bottom = Math.min(rect.top + rect.height + SPOT_PAD, window.innerHeight)
  const right = Math.min(rect.left + rect.width + SPOT_PAD, window.innerWidth)
  return { top, left, width: Math.max(0, right - left), height: Math.max(0, bottom - top) }
}

/**
 * Tour walks a new reader past the parts of the page that are not obvious
 * from looking at it. See ../tour.ts for what it points at and why.
 *
 * It is a modal dialog while it is up: focus moves into the card and stays
 * there, Escape skips, the arrow keys step, and the page's own single-key
 * shortcuts are held off by the caller. The highlight is a frame drawn over
 * the target rather than anything done to the target itself, so the page
 * underneath is left exactly as it was when the tour goes.
 */
export function Tour({ onClose }: Props) {
  const t = useT()
  // The steps whose targets are on the page are worked out once, when the
  // tour opens: a step that vanished half way through would renumber the
  // counter under the reader's eyes.
  const [steps] = useState(() => TOUR_STEPS.filter((step) => shown(step) !== null))
  const [at, setAt] = useState(0)
  const [target, setTarget] = useState<Rect | null>(null)
  const [cardPos, setCardPos] = useState<{ top: number; left: number } | null>(null)
  const card = useRef<HTMLDivElement>(null)
  const next = useRef<HTMLButtonElement>(null)
  const returnFocus = useRef<Element | null>(null)
  const step: TourStep | undefined = steps[at]
  const last = at >= steps.length - 1

  // Focus goes back where it was once the tour is gone, so a reader who
  // opened it from the help sheet is not left at the top of the document.
  useEffect(() => {
    returnFocus.current = document.activeElement
    return () => {
      const el = returnFocus.current
      if (el instanceof HTMLElement && el.isConnected) el.focus()
    }
  }, [])

  // Measured rather than remembered: the target moves when the window is
  // resized or a pane scrolls, and the frame has to move with it.
  const measure = useCallback(() => {
    const el = step ? shown(step) : null
    const rect = el ? rectOf(el) : null
    setTarget(rect ? clip(rect) : null)
    const box = card.current?.getBoundingClientRect()
    if (!box) return
    setCardPos(
      placeCard(rect, { width: box.width, height: box.height }, { width: window.innerWidth, height: window.innerHeight }),
    )
  }, [step])

  useLayoutEffect(() => {
    // A line number may be below the fold; bring it up before measuring.
    const el = step ? shown(step) : null
    el?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    measure()
    next.current?.focus()
  }, [step, measure])

  useEffect(() => {
    let frame = 0
    const follow = () => {
      if (frame !== 0) return
      frame = window.requestAnimationFrame(() => {
        frame = 0
        measure()
      })
    }
    window.addEventListener('resize', follow)
    document.addEventListener('scroll', follow, true)
    const observer = typeof ResizeObserver === 'undefined' || !card.current ? null : new ResizeObserver(follow)
    if (card.current) observer?.observe(card.current)
    return () => {
      window.removeEventListener('resize', follow)
      document.removeEventListener('scroll', follow, true)
      observer?.disconnect()
      if (frame !== 0) window.cancelAnimationFrame(frame)
    }
  }, [measure])

  const forward = useCallback(() => {
    if (last) onClose()
    else setAt((i) => i + 1)
  }, [last, onClose])

  const back = useCallback(() => setAt((i) => Math.max(0, i - 1)), [])

  // On the window and capturing, so a key reaches the tour before anything
  // on the page that would act on it.
  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') {
        ev.preventDefault()
        ev.stopPropagation()
        onClose()
        return
      }
      if (ev.key === 'ArrowRight' && !ev.altKey && !ev.metaKey && !ev.ctrlKey) {
        ev.preventDefault()
        forward()
        return
      }
      if (ev.key === 'ArrowLeft' && !ev.altKey && !ev.metaKey && !ev.ctrlKey) {
        ev.preventDefault()
        back()
        return
      }
      // Tab stays in the card: the page behind it is dimmed and is not what
      // the keyboard is for while the tour is up.
      if (ev.key === 'Tab' && card.current) {
        const stops = Array.from(card.current.querySelectorAll<HTMLElement>('button:not(:disabled)'))
        if (stops.length === 0) return
        const first = stops[0]
        const end = stops[stops.length - 1]
        const inside = card.current.contains(document.activeElement)
        if (ev.shiftKey && (document.activeElement === first || !inside)) {
          ev.preventDefault()
          end.focus()
        } else if (!ev.shiftKey && (document.activeElement === end || !inside)) {
          ev.preventDefault()
          first.focus()
        }
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose, forward, back])

  // Nothing to show at all - a page with none of the targets on it - is
  // nothing to walk through.
  useEffect(() => {
    if (steps.length === 0) onClose()
  }, [steps.length, onClose])

  if (!step) return null

  const body = client.isStatic && step.staticBody ? step.staticBody : step.body

  return (
    <div className="tour">
      {/* Clicks on the dimmed page are swallowed rather than taken as a
          dismissal: a stray click should not lose the reader their place. */}
      <div className="tour-blocker" aria-hidden="true" />
      {target ? (
        <div
          className="tour-spot"
          aria-hidden="true"
          style={{ top: target.top, left: target.left, width: target.width, height: target.height }}
        />
      ) : (
        <div className="tour-dim" aria-hidden="true" />
      )}
      <div
        ref={card}
        className="tour-card"
        role="dialog"
        aria-modal="true"
        aria-label={t('tour.label')}
        aria-describedby="sbnn-tour-body"
        // Transparent rather than hidden until placed: a hidden button cannot
        // take the focus the first step hands it.
        style={cardPos ? { top: cardPos.top, left: cardPos.left } : { top: 0, left: 0, opacity: 0 }}
      >
        <div className="tour-head">
          <h2 id="sbnn-tour-title">{t(step.title)}</h2>
          <span className="hint tour-count">{t('tour.step', { at: at + 1, total: steps.length })}</span>
        </div>
        <p id="sbnn-tour-body" aria-live="polite">
          {t(body)}
        </p>
        <div className="tour-actions">
          <button className="ghost" onClick={onClose}>
            {t('tour.skip')}
          </button>
          <span className="spacer" />
          <button className="ghost" onClick={back} disabled={at === 0}>
            {t('tour.back')}
          </button>
          <button ref={next} onClick={forward}>
            {last ? t('tour.done') : t('tour.next')}
          </button>
        </div>
      </div>
    </div>
  )
}
