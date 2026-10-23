import { useCallback, useRef, type ReactNode, type Ref } from 'react'
import { Divider } from './Divider'
import { useT } from '../i18n'

interface Props {
  left: ReactNode
  right: ReactNode
  /** ratio is the share of the width the left pane gets, 0 or 1 when one of
   * the panes is minimised away. */
  ratio: number
  onRatioChange: (ratio: number) => void
  /** leftRef and rightRef reach the actual scrolling element of each pane -
   * what a stack inside it needs to run an IntersectionObserver against, or
   * to scroll programmatically. */
  leftRef?: Ref<HTMLDivElement>
  rightRef?: Ref<HTMLDivElement>
}

/** SPLIT_DEFAULT is the share the diff gets before anyone drags anything. */
export const SPLIT_DEFAULT = 0.55

// A pane can be squeezed to a sliver and, past that, minimised away
// completely. The divider stays put either way, so a minimised pane is one
// drag (or a double click) away from coming back.
const MIN_RATIO = 0.04
const MAX_RATIO = 0.96
const SNAP = 56 // pixels from an edge that minimise the pane on that side
const KEY_STEP = 0.02

/** clampRatio keeps a ratio usable, collapsing it once it passes the sliver. */
export function clampRatio(value: number): number {
  if (value < MIN_RATIO) return 0
  if (value > MAX_RATIO) return 1
  return value
}

/** nudgeRatio moves a divider by one step, out of a minimised pane included. */
export function nudgeRatio(ratio: number, direction: -1 | 1): number {
  if (ratio === 0) return direction > 0 ? MIN_RATIO : 0
  if (ratio === 1) return direction < 0 ? MAX_RATIO : 1
  return clampRatio(ratio + direction * KEY_STEP)
}

/** SplitPane shows the diff and the preview side by side, with a draggable divider. */
export function SplitPane({ left, right, ratio, onRatioChange, leftRef, rightRef }: Props) {
  const t = useT()
  const containerRef = useRef<HTMLDivElement>(null)

  const onDrag = useCallback(
    (clientX: number) => {
      const container = containerRef.current
      if (!container) return
      const rect = container.getBoundingClientRect()
      if (rect.width === 0) return
      const x = clientX - rect.left
      if (x < SNAP) {
        onRatioChange(0)
        return
      }
      if (x > rect.width - SNAP) {
        onRatioChange(1)
        return
      }
      onRatioChange(clampRatio(x / rect.width))
    },
    [onRatioChange],
  )

  return (
    <div className="split" ref={containerRef}>
      <div
        ref={leftRef}
        className={`split-pane${ratio === 0 ? ' collapsed' : ''}`}
        style={{ width: `${ratio * 100}%` }}
        aria-hidden={ratio === 0}
      >
        {left}
      </div>
      <Divider
        label={t('divider.split')}
        tour="panes"
        onDrag={onDrag}
        onReset={() => onRatioChange(SPLIT_DEFAULT)}
        onNudge={(direction) => onRatioChange(nudgeRatio(ratio, direction))}
        handles={
          ratio === 0
            ? [{ icon: 'chevron_right', title: t('divider.showDiff'), onClick: () => onRatioChange(SPLIT_DEFAULT) }]
            : ratio === 1
              ? [
                  {
                    icon: 'chevron_left',
                    title: t('divider.showPreview'),
                    onClick: () => onRatioChange(SPLIT_DEFAULT),
                  },
                ]
              : [
                  { icon: 'chevron_left', title: t('divider.hideDiff'), onClick: () => onRatioChange(0) },
                  { icon: 'chevron_right', title: t('divider.hidePreview'), onClick: () => onRatioChange(1) },
                ]
        }
      />
      <div
        ref={rightRef}
        className={`split-pane${ratio === 1 ? ' collapsed' : ''}`}
        style={{ width: `${(1 - ratio) * 100}%` }}
        aria-hidden={ratio === 1}
      >
        {right}
      </div>
    </div>
  )
}
