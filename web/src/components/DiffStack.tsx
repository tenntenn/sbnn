import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from 'react'
import type { Comment, Diff, ViewMode } from '../types'
import { sectionKey } from '../sectionKey'
import { DiffFileSection } from './DiffFileSection'
import { Icon } from './Icon'

export interface DiffStackHandle {
  scrollToSection: (key: string) => void
}

/** ScrollFraction is how far into the active file's own section the reader
 * has scrolled - not how far down the whole stack, which would point the
 * preview at the wrong file entirely once more than one file is mounted. */
export interface ScrollFraction {
  key: string
  fraction: number
}

interface Props {
  group: string
  diffs: Diff[]
  comments: Comment[]
  foldOverrides: Map<string, boolean>
  viewModeOverrides: Map<string, ViewMode>
  /** viewModeDefault is every file's view mode until its own toggle says
   * otherwise - null means each file keeps the server's per-file default. */
  viewModeDefault: ViewMode | null
  onSetFolded: (key: string, value: boolean) => void
  onSetViewMode: (key: string, mode: ViewMode) => void
  onSetViewModeDefault: (mode: ViewMode) => void
  onChanged: () => void
  containerRef: RefObject<HTMLDivElement | null>
  onActiveChange: (key: string | null) => void
  onScrollFraction: (payload: ScrollFraction | null) => void
}

// The band a file's header has to cross before it counts as "the one being
// read": the top ACTIVE_BAND share of the pane. Merely being visible is not
// enough - every file on screen already is - it has to have been scrolled
// up into this band.
const ACTIVE_BAND = 0.7

/** resolveFolded settles whether a file is shown folded.
 *
 * The server may fold a file on its own, and that default steps aside when
 * the file carries comments - an automatic fold must never hide one. An
 * override is the reader's own choice, made on this page with the header
 * button or `f`, and it wins outright: a reviewer who is done with a long
 * generated file gets to put it away even after commenting on it. */
export function resolveFolded(
  override: boolean | undefined,
  senderFolded: boolean,
  hasComments: boolean,
): boolean {
  if (override !== undefined) return override
  return senderFolded && !hasComments
}

function clamp01(n: number): number {
  return Math.min(1, Math.max(0, n))
}

export const DiffStack = forwardRef<DiffStackHandle, Props>(function DiffStack(
  {
    group,
    diffs,
    comments,
    foldOverrides,
    viewModeOverrides,
    viewModeDefault,
    onSetFolded,
    onSetViewMode,
    onSetViewModeDefault,
    onChanged,
    containerRef,
    onActiveChange,
    onScrollFraction,
  },
  ref,
) {
  const sectionEls = useRef(new Map<string, HTMLDivElement>())
  const intersecting = useRef(new Set<string>())
  const activeKeyRef = useRef<string | null>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)
  const [toolbarHeight, setToolbarHeight] = useState(0)

  // Measured, not guessed: the toolbar's own height sets how far down each
  // file's sticky header sits, so the two never overlap.
  useLayoutEffect(() => {
    const el = toolbarRef.current
    if (!el) return
    const observer = new ResizeObserver(() => setToolbarHeight(el.offsetHeight))
    observer.observe(el)
    setToolbarHeight(el.offsetHeight)
    return () => observer.disconnect()
  }, [])

  const order = useMemo(
    () => diffs.flatMap((d) => d.files.map((f) => sectionKey(d.id, f.id))),
    [diffs],
  )

  const commentsByKey = useMemo(() => {
    const map = new Map<string, Comment[]>()
    for (const c of comments) {
      const key = sectionKey(c.diffId, c.fileId)
      const list = map.get(key)
      if (list) list.push(c)
      else map.set(key, [c])
    }
    return map
  }, [comments])

  // A ref mirror of the latest callbacks, read from inside the observer and
  // scroll listener below: those are only rebuilt when `order` changes, so a
  // callback captured straight from props would go stale between reloads.
  const onActiveChangeRef = useRef(onActiveChange)
  onActiveChangeRef.current = onActiveChange
  const onScrollFractionRef = useRef(onScrollFraction)
  onScrollFractionRef.current = onScrollFraction

  // Lazy one-time default, the pattern React documents for writing a ref
  // during render: before anything has been scrolled, the first file is the
  // active one.
  if (activeKeyRef.current === null && order.length > 0) activeKeyRef.current = order[0]

  useImperativeHandle(ref, () => ({
    scrollToSection(key: string) {
      sectionEls.current.get(key)?.scrollIntoView({ block: 'start' })
    },
  }))

  useEffect(() => {
    onActiveChangeRef.current(activeKeyRef.current)
    // Only on mount: this pushes the lazy default above to the parent
    // before the IntersectionObserver's first callback has had a chance to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const root = containerRef.current
    if (!root) return
    const recomputeActive = () => {
      let found: string | null = null
      for (const key of order) {
        if (intersecting.current.has(key)) found = key
      }
      if (found === null) found = order[0] ?? null
      if (found !== activeKeyRef.current) {
        activeKeyRef.current = found
        onActiveChangeRef.current(found)
      }
    }
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const key = (entry.target as HTMLElement).dataset.sectionKey
          if (!key) continue
          if (entry.isIntersecting) intersecting.current.add(key)
          else intersecting.current.delete(key)
        }
        recomputeActive()
      },
      { root, rootMargin: `0px 0px -${Math.round(ACTIVE_BAND * 100)}% 0px`, threshold: 0 },
    )
    for (const key of order) {
      const el = sectionEls.current.get(key)
      if (el) observer.observe(el)
    }
    return () => observer.disconnect()
  }, [containerRef, order])

  useEffect(() => {
    const root = containerRef.current
    if (!root) return
    let ticking = false
    const onScroll = () => {
      if (ticking) return
      ticking = true
      requestAnimationFrame(() => {
        ticking = false
        const key = activeKeyRef.current
        const el = key ? sectionEls.current.get(key) : null
        if (!key || !el) {
          onScrollFractionRef.current(null)
          return
        }
        const rootRect = root.getBoundingClientRect()
        const elRect = el.getBoundingClientRect()
        const top = elRect.top - rootRect.top + root.scrollTop
        const fraction = elRect.height > 0 ? clamp01((root.scrollTop - top) / elRect.height) : 0
        onScrollFractionRef.current({ key, fraction })
      })
    }
    root.addEventListener('scroll', onScroll, { passive: true })
    return () => root.removeEventListener('scroll', onScroll)
  }, [containerRef])

  return (
    <div className="diff-stack" style={{ '--diff-toolbar-h': `${toolbarHeight}px` } as React.CSSProperties}>
      <div className="diff-toolbar" ref={toolbarRef}>
        <span className="hint">Every file</span>
        <div className="toggle">
          <button
            className={viewModeDefault === 'split' ? 'active' : ''}
            onClick={() => onSetViewModeDefault('split')}
            title="Set every file to split - a file toggled on its own still remembers that choice"
          >
            <Icon name="view_column" small />
            split
          </button>
          <button
            className={viewModeDefault === 'unified' ? 'active' : ''}
            onClick={() => onSetViewModeDefault('unified')}
            title="Set every file to unified - a file toggled on its own still remembers that choice"
          >
            <Icon name="table_rows" small />
            unified
          </button>
        </div>
      </div>
      {diffs.map((d) => (
        <div key={d.id}>
          {diffs.length > 1 && (
            <div className="diff-round-divider">
              <span className="diff-round-title">{d.title}</span>
              <span className="hint">{d.files.length}</span>
            </div>
          )}
          {d.files.map((file) => {
            const key = sectionKey(d.id, file.id)
            const fileComments = commentsByKey.get(key) ?? []
            const folded = resolveFolded(foldOverrides.get(key), Boolean(file.folded), fileComments.length > 0)
            const viewMode = viewModeOverrides.get(key) ?? viewModeDefault ?? file.viewMode
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
                <DiffFileSection
                  group={group}
                  diff={d}
                  file={file}
                  comments={fileComments}
                  onChanged={onChanged}
                  folded={folded}
                  onSetFolded={(value) => onSetFolded(key, value)}
                  viewMode={viewMode}
                  onSetViewMode={(mode) => onSetViewMode(key, mode)}
                />
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
})
