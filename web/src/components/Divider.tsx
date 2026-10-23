import { useRef, useState } from 'react'
import { Icon } from './Icon'

/** A handle is the small button riding on the divider that puts one specific
 * pane away, or brings it back, in a single click - the drag and the double
 * click do the same thing but neither is discoverable by looking at the bar. */
interface Handle {
  icon: string
  title: string
  onClick: () => void
}

interface Props {
  /** onDrag receives the pointer position while the divider is dragged. */
  onDrag: (clientX: number) => void
  /** onReset is called on a double click, to go back to a sensible width. */
  onReset?: () => void
  /** onNudge moves the divider by a step with the arrow keys. */
  onNudge?: (direction: -1 | 1) => void
  label: string
  /** One handle brings a single pane back; two, either side of a middle
   * divider, each put away the pane on their side. */
  handles?: Handle[]
}

/**
 * Divider is the draggable edge between two panes. Every pane in sbnn is
 * resized with one, so they all behave the same: drag to resize, double click
 * to put the pane away or bring it back, arrow keys once it has focus.
 *
 * The pointer is captured for the whole drag. Without that, dragging towards
 * the Markdown preview stops the moment the pointer crosses into its iframe,
 * because the events are delivered to the framed document instead.
 */
export function Divider({ onDrag, onReset, onNudge, label, handles }: Props) {
  const [dragging, setDragging] = useState(false)
  const pointerID = useRef<number | null>(null)

  const stop = (el: HTMLElement) => {
    if (pointerID.current !== null && el.hasPointerCapture(pointerID.current)) {
      el.releasePointerCapture(pointerID.current)
    }
    pointerID.current = null
    setDragging(false)
    document.body.classList.remove('resizing')
  }

  return (
    <div
      className={`pane-divider${dragging ? ' dragging' : ''}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      tabIndex={0}
      onPointerDown={(ev) => {
        // The handle sits on the divider so it can be seen, not so a drag
        // started on it fights the click - capturing the pointer here would
        // send the handle's own pointerup (and the click after it) to this
        // div instead, and the button would never fire.
        if ((ev.target as HTMLElement).closest('.divider-handle')) return
        ev.preventDefault()
        ev.currentTarget.setPointerCapture(ev.pointerId)
        pointerID.current = ev.pointerId
        setDragging(true)
        // The cursor and the suppressed text selection hold over the whole
        // window while the drag lasts, not just over the divider.
        document.body.classList.add('resizing')
      }}
      onPointerMove={(ev) => {
        if (pointerID.current === null) return
        ev.preventDefault()
        onDrag(ev.clientX)
      }}
      onPointerUp={(ev) => stop(ev.currentTarget)}
      onPointerCancel={(ev) => stop(ev.currentTarget)}
      onLostPointerCapture={(ev) => stop(ev.currentTarget)}
      onDoubleClick={onReset}
      onKeyDown={(ev) => {
        if (ev.key === 'ArrowLeft') {
          ev.preventDefault()
          onNudge?.(-1)
        }
        if (ev.key === 'ArrowRight') {
          ev.preventDefault()
          onNudge?.(1)
        }
        if (ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault()
          onReset?.()
        }
      }}
    >
      {handles && handles.length > 0 && (
        <div className="divider-handles">
          {handles.map((handle, i) => (
            <button
              key={i}
              type="button"
              className="divider-handle"
              title={handle.title}
              aria-label={handle.title}
              onPointerDown={(ev) => ev.stopPropagation()}
              onClick={(ev) => {
                ev.stopPropagation()
                handle.onClick()
              }}
            >
              <Icon name={handle.icon} small />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
