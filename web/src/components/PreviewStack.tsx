import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import type { Diff, FileDiff, PreviewKind, Status } from '../types'
import { isPreviewable } from '../types'
import { PreviewFileSection } from './PreviewFileSection'
import { sectionKey } from '../sectionKey'
import { Icon } from './Icon'
import { MoIcon } from './MoIcon'
import type { ScrollFraction } from './DiffStack'

// How far ahead of the visible area a section is fetched: generous enough
// that the render is usually ready by the time the reader arrives, small
// enough that scrolling past a large diff does not fetch every file in it
// up front.
const PREFETCH_MARGIN = '600px 0px 600px 0px'

interface Props {
  group: string
  diffs: Diff[]
  status: Status | null
  containerRef: RefObject<HTMLDivElement | null>
  /** scrollTarget is where the diff pane wants the preview to follow to, or
   * null when following is off or there is nothing to follow yet. */
  scrollTarget: ScrollFraction | null
  sync: boolean
  onSync: (on: boolean) => void
  /** kind, forced and onSetKind are the same renderer choice the caller
   * shows in narrow mode too - a page-level pick, not a per-file one, so it
   * lives one level up rather than being owned here. */
  kind: PreviewKind
  forced: boolean
  onSetKind: (kind: PreviewKind) => void
}

export function PreviewStack({
  group,
  diffs,
  status,
  containerRef,
  scrollTarget,
  sync,
  onSync,
  kind,
  forced,
  onSetKind,
}: Props) {
  const [activated, setActivated] = useState<Set<string>>(() => new Set())
  const sectionEls = useRef(new Map<string, HTMLDivElement>())
  const toolbarRef = useRef<HTMLDivElement>(null)
  const [toolbarHeight, setToolbarHeight] = useState(0)

  // Measured, not guessed: the toolbar's own height sets how far down each
  // file's sticky header sits, so the two never overlap regardless of font
  // size, theme, or how the toolbar's contents wrap.
  useLayoutEffect(() => {
    const el = toolbarRef.current
    if (!el) return
    // offsetHeight, not the entry's contentRect, so padding and the border
    // are included - what the next sticky header actually needs to clear.
    const observer = new ResizeObserver(() => setToolbarHeight(el.offsetHeight))
    observer.observe(el)
    setToolbarHeight(el.offsetHeight)
    return () => observer.disconnect()
  }, [])

  // Only a file the preview pane has something to show for gets a section.
  // A round of 500 files where half are code used to mount 500 preview
  // sections, every one of the code files carrying nothing but a header and
  // the line "... has no preview" - half the sections on the page, and half
  // its DOM nodes, saying nothing.
  const rounds = useMemo(
    () =>
      diffs
        .map((d) => ({ diff: d, files: d.files.filter(isPreviewable) as FileDiff[] }))
        .filter((r) => r.files.length > 0),
    [diffs],
  )

  const order = useMemo(
    () => rounds.flatMap((r) => r.files.map((f) => sectionKey(r.diff.id, f.id))),
    [rounds],
  )

  const nothingToPreview = diffs.length > 0 && rounds.length === 0

  // Lazy activation: a section starts fetching (and, for mo, mounting its
  // iframe) once it is near the viewport, and stays activated - scrolling
  // back past it must not re-fetch or reload it.
  useEffect(() => {
    const root = containerRef.current
    if (!root) return
    const observer = new IntersectionObserver(
      (entries) => {
        const arriving = entries.filter((e) => e.isIntersecting).map((e) => (e.target as HTMLElement).dataset.sectionKey)
        if (arriving.length === 0) return
        setActivated((current) => {
          let changed = false
          const next = new Set(current)
          for (const key of arriving) {
            if (key && !next.has(key)) {
              next.add(key)
              changed = true
            }
          }
          return changed ? next : current
        })
      },
      { root, rootMargin: PREFETCH_MARGIN, threshold: 0 },
    )
    for (const key of order) {
      const el = sectionEls.current.get(key)
      if (el) observer.observe(el)
    }
    return () => observer.disconnect()
  }, [containerRef, order])

  // Follow the diff: move to the same file, at the same fraction into its
  // section, that the diff pane is showing.
  useEffect(() => {
    if (!scrollTarget) return
    const root = containerRef.current
    const el = sectionEls.current.get(scrollTarget.key)
    if (!root || !el) return
    const rootRect = root.getBoundingClientRect()
    const elRect = el.getBoundingClientRect()
    const top = elRect.top - rootRect.top + root.scrollTop
    root.scrollTop = Math.max(0, top + scrollTarget.fraction * elRect.height)
  }, [scrollTarget, containerRef])

  return (
    <div
      className="preview-stack"
      style={{ '--preview-toolbar-h': `${toolbarHeight}px` } as React.CSSProperties}
    >
      <div className="preview-toolbar" ref={toolbarRef}>
        {!forced && (
          <div className="toggle">
            <button
              className={kind === 'preview' ? 'active' : ''}
              onClick={() => onSetKind('preview')}
              title="sbnn's own preview - needs nothing installed, follows the diff as it scrolls"
            >
              <Icon name="visibility" small />
              preview
            </button>
            <button
              className={kind === 'mo' ? 'active' : ''}
              onClick={() => onSetKind('mo')}
              title="mo - renders more, in a frame, but does not follow the diff"
            >
              <MoIcon small />
              mo
            </button>
          </div>
        )}
        <span className="spacer" />
        {/* A disabled button swallows hover, so the tooltip explaining why
            lives on a span around it instead. */}
        <span
          title={
            kind !== 'preview'
              ? "Only sbnn's own preview can follow the diff: mo is framed from another origin, where a page may not touch its scrolling"
              : sync
                ? 'The preview follows the diff; scrolling it yourself stops that'
                : 'Follow the diff again'
          }
        >
          <button
            className={`ghost icon-only${sync && kind === 'preview' ? ' active' : ''}`}
            disabled={kind !== 'preview'}
            onClick={() => onSync(!sync)}
          >
            <Icon name="link" />
          </button>
        </span>
      </div>

      {nothingToPreview && (
        <p className="empty">No file in this review has a preview.</p>
      )}

      {rounds.map(({ diff: d, files }) => (
        <div key={d.id}>
          {diffs.length > 1 && (
            <div className="diff-round-divider">
              <span className="diff-round-title">{d.title}</span>
              {/* The count is of the files shown here, which is not the
                  round's file count once the ones with no preview are left
                  out of this pane. */}
              <span className="hint">{files.length}</span>
            </div>
          )}
          {files.map((file) => {
            const key = sectionKey(d.id, file.id)
            return (
              <div
                key={file.id}
                id={key}
                data-section-key={key}
                className="file-section"
                ref={(el) => {
                  if (el) sectionEls.current.set(key, el)
                  else sectionEls.current.delete(key)
                }}
              >
                <PreviewFileSection
                  group={group}
                  diffId={d.id}
                  file={file}
                  status={status}
                  kind={kind}
                  active={activated.has(key)}
                  onUserScroll={() => onSync(false)}
                />
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}
