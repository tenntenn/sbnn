import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import type { Comment, Diff, FileDiff, PreviewKind, Status } from '../types'
import { filePath, isPreviewable, previewFormatOf } from '../types'
import { prefetchesForFind } from '../findHold'
import type { PreviewLinkTargets } from '../markdown'
import { PreviewFileSection } from './PreviewFileSection'
import { sectionKey } from '../sectionKey'
import { hashForKey } from '../urlState'
import { Icon } from './Icon'
import { MoIcon } from './MoIcon'
import type { ScrollFraction } from './DiffStack'
import { client } from '../client'
import { useLazyMount, type FindPrefetch } from '../useLazyMount'
import { sameElements } from '../share'
import { syncObserved } from '../observed'

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
  comments: Comment[]
  onChanged: () => void
}

const NO_COMMENTS: Comment[] = []

// What a file gets in place of the link targets when it renders no HTML of its
// own: a source file or an image never reads them, and handing every one of
// them the targets made each new round render every section again (#402).
const NO_TARGETS: PreviewLinkTargets = {}

interface SlotProps {
  sectionKey: string
  group: string
  linkTargets: PreviewLinkTargets
  diffId: string
  file: FileDiff
  status: Status | null
  kind: PreviewKind
  active: boolean
  bodyMounted: boolean
  frameMounted: boolean
  comments: Comment[]
  register: (key: string, el: HTMLDivElement | null) => void
  settled: (key: string) => void
  onSync: (on: boolean) => void
  onChanged: () => void
}

// A section takes the status only to say whether mo is installed, and the
// status is a new object on every event, so it is compared by that answer. The
// comment list of a file is built again on every render of the stack and is
// compared by its elements, which keep their identity (see ../share).
function slotPropsEqual(a: SlotProps, b: SlotProps): boolean {
  for (const k of Object.keys(a) as (keyof SlotProps)[]) {
    if (k === 'comments' || k === 'status') continue
    if (a[k] !== b[k]) return false
  }
  return (
    (a.status === null) === (b.status === null) &&
    a.status?.moAvailable === b.status?.moAvailable &&
    sameElements(a.comments, b.comments)
  )
}

/**
 * One previewed file of the stack, memoised like DiffStack's StackFile: an
 * event used to render every PreviewFileSection again because the status,
 * the comment list and the closures handed to it were new each time (#402).
 */
const PreviewSlot = memo(function PreviewSlot({
  sectionKey: key,
  group,
  linkTargets,
  diffId,
  file,
  status,
  kind,
  active,
  bodyMounted,
  frameMounted,
  comments,
  register,
  settled,
  onSync,
  onChanged,
}: SlotProps) {
  const ref = useCallback((el: HTMLDivElement | null) => register(key, el), [register, key])
  const onSettled = useCallback(() => settled(key), [settled, key])
  const onUserScroll = useCallback(() => onSync(false), [onSync])
  return (
    <div id={key} data-section-key={key} className="file-section" ref={ref}>
      <PreviewFileSection
        group={group}
        linkTargets={linkTargets}
        diffId={diffId}
        file={file}
        status={status}
        kind={kind}
        active={active}
        onSettled={onSettled}
        bodyMounted={bodyMounted}
        frameMounted={frameMounted}
        onUserScroll={onUserScroll}
        comments={comments}
        onChanged={onChanged}
      />
    </div>
  )
}, slotPropsEqual)

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
  comments,
  onChanged,
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
  // An exported page has no server to read a working tree file from, so a
  // source file has nothing to show there and stays out of the pane.
  const hasSource = !client.isStatic

  const rounds = useMemo(
    () =>
      diffs
        .map((d) => ({
          diff: d,
          files: d.files.filter((f) => isPreviewable(f, hasSource)) as FileDiff[],
        }))
        .filter((r) => r.files.length > 0),
    [diffs, hasSource],
  )

  const order = useMemo(
    () => rounds.flatMap((r) => r.files.map((f) => sectionKey(r.diff.id, f.id))),
    [rounds],
  )

  // Where a link between two files of the review leads. A relative href in
  // one previewed file is resolved against that file's directory and looked
  // up here, so it is followed to the other file's section on this page
  // instead of to the server root. A path touched by more than one round
  // resolves to the newest one, which is the copy the reader is being shown.
  const linkTargets = useMemo<PreviewLinkTargets>(() => {
    const targets: PreviewLinkTargets = {}
    for (const round of rounds) {
      for (const file of round.files) {
        const path = filePath(file)
        if (path !== '') targets[path] = hashForKey(sectionKey(round.diff.id, file.id))
      }
    }
    return targets
  }, [rounds])

  const nothingToPreview = diffs.length > 0 && rounds.length === 0

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

  const registerSection = useCallback((key: string, el: HTMLDivElement | null) => {
    if (el) sectionEls.current.set(key, el)
    else sectionEls.current.delete(key)
  }, [])

  // Lazy activation: a section starts fetching (and, for mo, mounting its
  // iframe) once it is near the viewport, and stays activated - scrolling
  // back past it must not re-fetch or reload it.
  const activation = useRef<IntersectionObserver | null>(null)
  const activationEls = useRef(new Map<string, HTMLDivElement>())
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
    activation.current = observer
    return () => {
      observer.disconnect()
      activation.current = null
      activationEls.current.clear()
    }
  }, [containerRef])

  // The observer is kept across rounds and pointed at the sections that exist,
  // rather than rebuilt and given every one of them again (#402).
  useEffect(() => {
    if (activation.current) {
      syncObserved(activation.current, activationEls.current, order, (key) =>
        sectionEls.current.get(key),
      )
    }
  }, [order])

  // Which sections carry their body on the page. The rest are a shell that
  // stands as tall as the body last was (#390).
  //
  // While the reader is searching the page (#398) the hold also fetches the
  // previews whose rendered text the diff does not carry (#400): only the
  // files prefetchesForFind names, a few at a time.
  const activatedRef = useRef(activated)
  activatedRef.current = activated
  const prefetchable = useMemo(() => {
    const keys = new Set<string>()
    for (const r of rounds) {
      for (const f of r.files) {
        const format = previewFormatOf(f, hasSource)
        if (prefetchesForFind(format, format !== 'markdown' || kind === 'preview')) {
          keys.add(sectionKey(r.diff.id, f.id))
        }
      }
    }
    return keys
  }, [rounds, hasSource, kind])
  const prefetchableRef = useRef(prefetchable)
  prefetchableRef.current = prefetchable
  const findPrefetch = useMemo<FindPrefetch>(
    () => ({
      wants: (key) => prefetchableRef.current.has(key) && !activatedRef.current.has(key),
      start: (keys) =>
        setActivated((current) => {
          const next = new Set(current)
          for (const key of keys) next.add(key)
          return next
        }),
    }),
    [],
  )
  const { mounted, lazy, settled } = useLazyMount(
    containerRef,
    order,
    useCallback((key: string) => sectionEls.current.get(key), []),
    findPrefetch,
  )

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
              <PreviewSlot
                key={file.id}
                sectionKey={key}
                group={group}
                linkTargets={file.isMarkdown || file.isNotebook ? linkTargets : NO_TARGETS}
                diffId={d.id}
                file={file}
                status={status}
                kind={kind}
                active={activated.has(key)}
                bodyMounted={mounted.has(key)}
                frameMounted={lazy.has(key)}
                comments={commentsByKey.get(key) ?? NO_COMMENTS}
                register={registerSection}
                settled={settled}
                onSync={onSync}
                onChanged={onChanged}
              />
            )
          })}
        </div>
      ))}
    </div>
  )
}
