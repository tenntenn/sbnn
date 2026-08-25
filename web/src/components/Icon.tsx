import { useSyncExternalStore } from 'react'

interface Props {
  name: string
  small?: boolean
}

/**
 * Icon renders a glyph name as a ligature, so the name itself is what sits in
 * the DOM. When the icon font is missing the browser draws that name as plain
 * text — `expand_more` is five times the width the layout budgeted for a
 * glyph, and it collides with whatever sits beside it. The store below watches
 * the font so an icon can collapse to an empty box of the right width instead.
 *
 * The check needs document.fonts. Where that is unavailable there is nothing
 * to ask, so the name is rendered as before rather than hidden on a guess.
 */
const iconFont = '16px "sbnn Icons"'

type FontState = 'unknown' | 'loaded' | 'missing'

let fontState: FontState = 'unknown'
let watching = false
const listeners = new Set<() => void>()

function setFontState(next: FontState) {
  if (fontState === next) return
  fontState = next
  for (const fn of listeners) fn()
}

function watchFont() {
  if (watching) return
  watching = true
  if (typeof document === 'undefined' || !document.fonts) return
  const fonts = document.fonts
  const settle = () => setFontState(fonts.check(iconFont) ? 'loaded' : 'missing')
  // load() rejects when the face cannot be fetched, which is the 404 case an
  // exported page hits; ready covers a face that resolved some other way.
  fonts.load(iconFont).then(settle, () => setFontState('missing'))
  fonts.ready.then(settle, () => setFontState('missing'))
}

function subscribe(onChange: () => void): () => void {
  watchFont()
  listeners.add(onChange)
  return () => {
    listeners.delete(onChange)
  }
}

function snapshot(): FontState {
  return fontState
}

/** Icon renders one glyph from the subsetted icon font in styles.css. */
export function Icon({ name, small }: Props) {
  const missing = useSyncExternalStore(subscribe, snapshot, snapshot) === 'missing'
  return (
    <span
      className={`icon${small ? ' sm' : ''}`}
      aria-hidden="true"
      // The glyphs are one em square, so an empty box of that width leaves the
      // surrounding layout exactly as it was drawn with the font present.
      style={missing ? { display: 'inline-block', width: '1em' } : undefined}
    >
      {missing ? '' : name}
    </span>
  )
}
