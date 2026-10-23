/**
 * The first-run tour: what it points at, whether it has been seen, and where
 * its card goes. The component in components/Tour.tsx draws it; this file is
 * the part that can be reasoned about, and tested, without a DOM.
 *
 * A reader meets the page once as a stranger, and the parts of it that are
 * not obvious - that a line number is the way into a comment, that a
 * selection in the preview is one too, that a review ends with a button
 * rather than a last comment - are exactly the ones nobody discovers by
 * looking. The tour walks past them once and then stays out of the way.
 */

import { readSetting, writeSetting } from './storage'
import type { MessageKey } from './i18n'

const TOUR_KEY = 'sbnn.tour'

/** TourStep is one stop. targets are tried in order and the first one that is
 * on screen is the one highlighted: the wide layout and the phone put the
 * same thing in different elements. A step none of whose targets is on the
 * page - the preview on a phone showing the diff, the file list put away -
 * is left out of that run of the tour rather than pointing at nothing. */
export interface TourStep {
  id: string
  targets: string[]
  title: MessageKey
  body: MessageKey
  /** staticBody replaces body on an exported page, where the step's
   * controls are not the same. */
  staticBody?: MessageKey
}

export const TOUR_STEPS: readonly TourStep[] = [
  { id: 'files', targets: ['[data-tour="files"]'], title: 'tour.files.title', body: 'tour.files.body' },
  { id: 'diff', targets: ['[data-tour="diff"]', '.diff-pane'], title: 'tour.diff.title', body: 'tour.diff.body' },
  { id: 'preview', targets: ['[data-tour="preview"]'], title: 'tour.preview.title', body: 'tour.preview.body' },
  // The < and > riding on the bar between the diff and the preview. They are
  // small and unlabelled, and the drag and double click they stand in for
  // are invisible, so they get a stop of their own.
  { id: 'panes', targets: ['[data-tour="panes"]'], title: 'tour.panes.title', body: 'tour.panes.body' },
  {
    id: 'comment',
    // The first line number that takes a comment: the cell itself is what
    // the step is about, so it is what gets the highlight.
    targets: ['.diff-table td.num[role="button"]'],
    title: 'tour.comment.title',
    body: 'tour.comment.body',
  },
  {
    id: 'previewComment',
    // A Markdown preview whose blocks know their lines, so a selection in it
    // can become a comment. A review with no Markdown file has none, and the
    // step is left out.
    targets: ['[data-line-anchored="true"]'],
    title: 'tour.previewComment.title',
    body: 'tour.previewComment.body',
  },
  {
    id: 'review',
    targets: ['[data-tour="review"]'],
    title: 'tour.review.title',
    body: 'tour.review.body',
    staticBody: 'tour.review.bodyStatic',
  },
  { id: 'help', targets: ['[data-tour="help"]'], title: 'tour.help.title', body: 'tour.help.body' },
]

/** tourSeen says whether this browser has finished or skipped the tour. */
export function tourSeen(): boolean {
  return readSetting(TOUR_KEY) === 'done'
}

export function markTourSeen(): void {
  writeSetting(TOUR_KEY, 'done')
}

/**
 * shouldAutoStart says whether to open the tour without being asked.
 *
 * Only where the answer can be remembered. Storage is refused in a sandboxed
 * frame and with site data blocked - an exported page embedded with
 * `--fragment` is the common case - and there "once per browser" would be
 * "on every load", which turns a welcome into an obstacle. The tour stays a
 * click away from the help button for that reader instead.
 */
export function shouldAutoStart(): boolean {
  if (tourSeen()) return false
  writeSetting(TOUR_KEY, 'pending')
  return readSetting(TOUR_KEY) === 'pending'
}

export interface Rect {
  top: number
  left: number
  width: number
  height: number
}

export interface Size {
  width: number
  height: number
}

/** GAP is the space between a target and the card, MARGIN the closest the
 * card comes to the edge of the window. */
const GAP = 12
const MARGIN = 12

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(value, max))
}

/**
 * placeCard says where the card goes for a target, so that it is next to
 * what it describes without covering it.
 *
 * Below the target first, then above it, then beside it - a pane is as tall
 * as the window, so for the big targets it is the sides that have room. A
 * target that leaves room nowhere (a pane as wide as a phone) gets the card
 * over its upper part, which still leaves the target's edges lit around it.
 * No target at all is the centre of the window.
 */
export function placeCard(target: Rect | null, card: Size, view: Size): { top: number; left: number } {
  const maxLeft = Math.max(MARGIN, view.width - card.width - MARGIN)
  const maxTop = Math.max(MARGIN, view.height - card.height - MARGIN)
  if (!target) {
    return { top: clamp((view.height - card.height) / 2, MARGIN, maxTop), left: clamp((view.width - card.width) / 2, MARGIN, maxLeft) }
  }
  const bottom = target.top + target.height
  const right = target.left + target.width
  const centredLeft = clamp(target.left + target.width / 2 - card.width / 2, MARGIN, maxLeft)
  const besideTop = clamp(target.top, MARGIN, maxTop)
  if (bottom + GAP + card.height <= view.height - MARGIN) return { top: bottom + GAP, left: centredLeft }
  if (target.top - GAP - card.height >= MARGIN) return { top: target.top - GAP - card.height, left: centredLeft }
  if (right + GAP + card.width <= view.width - MARGIN) return { top: besideTop, left: right + GAP }
  if (target.left - GAP - card.width >= MARGIN) return { top: besideTop, left: target.left - GAP - card.width }
  return { top: clamp(target.top + MARGIN * 3, MARGIN, maxTop), left: centredLeft }
}
