import { useEffect, useMemo, useState } from 'react'
import type { FileDiff, PreviewFormat, PreviewKind, Status } from '../types'
import { filePath, hunksOf, previewFormatOf } from '../types'
import { client, type PreviewResult } from '../client'
import { assetTrouble, resolvePreviewLinks, type PreviewLinkTargets } from '../markdown'
import { Icon } from './Icon'
import { MoIcon } from './MoIcon'
import { SourceView } from './SourceView'

interface Props {
  group: string
  diffId: string
  file: FileDiff
  /** linkTargets is where each path this page carries can be reached, so a
   * relative link in a preview leads to the other file's section rather than
   * to the server root. */
  linkTargets?: PreviewLinkTargets
  status: Status | null
  kind: PreviewKind
  /** active gates the fetch: a section far from the viewport has no reason
   * to ask the server (or, worse, mo) to render it yet. Once true it stays
   * true, so scrolling back to an already-loaded file never refetches it. */
  active: boolean
  /** onUserScroll fires when the reader scrolls this section themselves,
   * which is what turns following the diff off. */
  onUserScroll?: () => void
}

/** formatOf is previewFormatOf with this page's answer to whether there is
 * a server to read a source file from. mo only ever renders Markdown - it
 * cannot show an image, a notebook or a .go file at all - so every other
 * format uses sbnn's own renderer regardless of the page's kind toggle. */
function formatOf(file: FileDiff): PreviewFormat {
  return previewFormatOf(file, !client.isStatic)
}

// A framed preview is sized between these two: never so short that a
// heading and two lines sit in a box of their own, never so tall that it
// takes more than the window and turns the page into three nested scrolls.
const MIN_FRAME_HEIGHT = 240
const MAX_FRAME_WINDOW_SHARE = 0.9
// What one line of rendered Markdown costs vertically, and the padding
// around the document, when the frame's own height cannot be read - see
// estimatedFrameHeight.
const FRAME_LINE_HEIGHT = 26
const FRAME_PADDING = 64

/** frameHeightWithin holds a height between the floor and what is left of
 * the window, so neither end of the range can produce a frame that is
 * mostly empty space or one that scrolls inside a scroll. */
function frameHeightWithin(height: number): number {
  const cap = Math.round(window.innerHeight * MAX_FRAME_WINDOW_SHARE)
  return Math.max(MIN_FRAME_HEIGHT, Math.min(cap, Math.round(height)))
}

/**
 * measuredFrameHeight is how tall the framed document actually is, or null
 * when it cannot be reached.
 *
 * sbnn serves mo through a proxy on a loopback port of its own, which is a
 * different origin from this page, so reading contentDocument throws
 * there. It is readable whenever the frame does happen to be same-origin,
 * and then the measurement beats any estimate - so it is tried first, and
 * its failure is not an error.
 */
function measuredFrameHeight(frame: HTMLIFrameElement): number | null {
  try {
    const doc = frame.contentDocument
    const root = doc?.documentElement
    if (!doc || !root) return null
    // offsetHeight, not scrollHeight: the root's scroll height never
    // reports less than the frame it is being viewed in, so a short
    // document measured that way would report whatever height the frame
    // already had and could never shrink it. The offset heights are the
    // boxes themselves, and the body's scroll height catches content that
    // spills out of it.
    const height = Math.max(root.offsetHeight, doc.body?.offsetHeight ?? 0, doc.body?.scrollHeight ?? 0)
    return height > 0 ? height : null
  } catch {
    // Another origin. The estimate below is what is left.
    return null
  }
}

/**
 * estimatedFrameHeight guesses the height of the document mo was handed,
 * from the lines of the file this section is already holding.
 *
 * It is the new side's line count, which is what mo renders: the whole
 * file when the working tree has it, and the reconstruction of it from the
 * diff otherwise. A partial reconstruction - a diff carrying only some
 * hunks - therefore reads short, which the floor above absorbs.
 */
function estimatedFrameHeight(file: FileDiff): number | null {
  let lines = 0
  for (const hunk of hunksOf(file)) {
    for (const line of hunk.lines) {
      if (line.kind !== 'delete') lines++
    }
  }
  return lines > 0 ? FRAME_PADDING + lines * FRAME_LINE_HEIGHT : null
}

/**
 * previewRevision is a short string that changes when what the preview
 * would show changes, and only then.
 *
 * A reload replaces the whole diffs array with freshly parsed JSON, so
 * every FileDiff is a new object after any event on the group - a comment
 * added, one resolved, another diff sent. Keying the loader on the object
 * therefore re-renders (and, with mo, re-execs the binary) for every
 * section on screen, every time. The server carries no per-file version or
 * mtime to key on instead, so this derives one from the file itself: the
 * path and status the preview header shows, and a hash of the hunks the
 * preview content is rebuilt from. Two parses of an unchanged file give
 * the same string; a real edit - even one that keeps the hunk shape, like
 * a fixed typo on one line - gives a different one.
 *
 * A worktree-sourced preview can still go stale without the diff moving,
 * since the file on disk is not part of the diff at all. That is what the
 * header's reload button is for.
 */
function previewRevision(file: FileDiff): string {
  // FNV-1a over the hunks: proportional to the file's own diff text, and
  // only recomputed when a reload hands this section a new object - the
  // same order of work as parsing that JSON in the first place.
  let hash = 0x811c9dc5
  const mix = (text: string) => {
    for (let i = 0; i < text.length; i++) {
      hash ^= text.charCodeAt(i)
      hash = Math.imul(hash, 0x01000193)
    }
  }
  const hunks = hunksOf(file)
  for (const hunk of hunks) {
    mix(hunk.header)
    for (const line of hunk.lines) {
      mix(line.kind)
      mix(line.content)
    }
  }
  return [
    filePath(file),
    file.status,
    file.isBinary ? 'bin' : 'text',
    hunks.length,
    (hash >>> 0).toString(36),
  ].join('|')
}

/**
 * PreviewFileSection shows one file's preview: Markdown, an image, or a
 * Jupyter notebook's rendered cells - whichever applies.
 *
 * In the live app a Markdown preview can be rendered by mo instead. mo
 * forbids framing with "frame-ancestors 'none'", so sbnn serves it through
 * its own loopback proxy, which relaxes that one directive for sbnn's
 * origin. An exported page has no mo behind it and renders everything
 * itself.
 */
export function PreviewFileSection({
  group,
  diffId,
  file,
  status,
  kind,
  active,
  linkTargets,
  onUserScroll,
}: Props) {
  const [preview, setPreview] = useState<PreviewResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [openingMo, setOpeningMo] = useState(false)
  const [imageFailed, setImageFailed] = useState(false)
  // The frame element itself, held in state rather than a ref so that
  // attaching to it - and letting go of it again - is an effect.
  const [frame, setFrame] = useState<HTMLIFrameElement | null>(null)
  const [frameHeight, setFrameHeight] = useState<number | null>(null)

  const format = formatOf(file)
  const previewable = format !== null
  // The loader below is keyed on this string rather than on file itself,
  // so an event that only rebuilds the object leaves the preview alone.
  const revision = useMemo(() => previewRevision(file), [file])
  const renderHere = format !== 'markdown' || kind === 'preview'

  // Whether there is a picture to draw at all was decided in Go, by
  // internal/asset, and travels on the file: the live page fetches the bytes
  // from an endpoint and an exported page carries them as a data URL, so
  // without one verdict the same image would be shown on screen and left out
  // of the exported page (#323). A file from an sbnn that predates the field
  // has no status, and that reads as "draw it", which is what it always did.
  const imageTrouble =
    format === 'image' && file.imageStatus !== undefined && file.imageStatus !== 'ok'
      ? assetTrouble(file.imageStatus, file.imageSize)
      : null
  const rawImageSrc =
    format === 'image' && imageTrouble === null ? client.imageSrc(group, diffId, file.id) : undefined
  // The live endpoint answers the same URL every time, so a reload has to
  // change the URL itself to bypass the browser's own cache. A static
  // page's data URL never changes and needs no such busting.
  const imageSrc =
    rawImageSrc && !client.isStatic
      ? `${rawImageSrc}${rawImageSrc.includes('?') ? '&' : '?'}r=${reloadKey}`
      : rawImageSrc

  useEffect(() => {
    setImageFailed(false)
  }, [imageSrc])

  useEffect(() => {
    if (format !== 'markdown' && format !== 'notebook' && format !== 'source') {
      setPreview(null)
      setError(null)
      return
    }
    if (!active) return
    let cancelled = false
    setLoading(true)
    setError(null)
    const load =
      format === 'notebook'
        ? client.previewNotebook(group, diffId, file.id)
        : format === 'source'
          ? client.previewSource(group, diffId, file.id)
          : renderHere
            ? client.previewMarkdown(group, diffId, file.id)
            : client.preview(group, diffId, file.id)
    load
      .then((p) => {
        if (!cancelled) setPreview(p)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setPreview(null)
          setError(err instanceof Error ? err.message : String(err))
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [group, diffId, file.id, revision, format, renderHere, reloadKey, active])

  // The links are pointed at this page's own sections after the render, not
  // inside it: which files the review carries is known here and not by the
  // renderer, and an exported page has to answer it the same way. It is a
  // string pass over the sanitiser's output, like the images.
  const previewHTML = useMemo(
    () =>
      preview?.kind === 'html'
        ? resolvePreviewLinks(preview.html, preview.path || filePath(file), linkTargets)
        : '',
    [preview, file, linkTargets],
  )

  const frameUrl = preview?.kind === 'frame' ? preview.url : undefined
  const estimatedHeight = useMemo(() => estimatedFrameHeight(file), [file])

  // A framed preview used to be exactly 80vh whatever was in it: empty
  // space under a three-line changelog entry, and a scrollbar of its own
  // inside the pane's inside the page's for anything longer. Size it to
  // what it is showing instead - measured when the frame's document can be
  // reached, estimated from the file's lines when it cannot - and hold it
  // between a floor and most of the window.
  useEffect(() => {
    if (!frame || !frameUrl) {
      setFrameHeight(null)
      return
    }
    let done = false
    let observer: ResizeObserver | null = null
    const apply = () => {
      if (done) return
      const height = measuredFrameHeight(frame) ?? estimatedHeight
      if (height === null) {
        // Nothing to go on: leave the stylesheet's height alone.
        setFrameHeight(null)
        return
      }
      const next = frameHeightWithin(height)
      // A frame sized to its content has no scrollbar, which reflows it;
      // ignoring a pixel of difference keeps that from oscillating.
      setFrameHeight((current) => (current !== null && Math.abs(current - next) <= 2 ? current : next))
    }
    const watch = () => {
      observer?.disconnect()
      observer = null
      try {
        const doc = frame.contentDocument
        if (doc && typeof ResizeObserver !== 'undefined') {
          observer = new ResizeObserver(apply)
          // The root grows with the document; the body is what a stylesheet
          // usually sizes. Watching both catches either.
          observer.observe(doc.documentElement)
          if (doc.body) observer.observe(doc.body)
        }
      } catch {
        // Another origin: there is nothing here to follow, and the
        // estimate does not need following.
      }
      apply()
    }
    frame.addEventListener('load', watch)
    // The frame may already have loaded by the time this runs.
    watch()
    // The cap is a share of the window, so it moves when the window does.
    window.addEventListener('resize', apply)
    return () => {
      done = true
      frame.removeEventListener('load', watch)
      observer?.disconnect()
      window.removeEventListener('resize', apply)
    }
  }, [frame, frameUrl, estimatedHeight])

  const openInMo = async () => {
    if (format !== 'markdown') return
    setOpeningMo(true)
    setError(null)
    try {
      const result = await client.preview(group, diffId, file.id)
      if (result.kind === 'frame' && result.moUrl) {
        window.open(result.moUrl, '_blank', 'noreferrer')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setOpeningMo(false)
    }
  }

  return (
    <section className="preview-section">
      <div className="preview-header">
        <span className="path">{filePath(file)}</span>
        {preview && (
          <>
            <span
              className="badge"
              title={`${preview.path}\n${
                preview.source === 'worktree' ? 'the working tree file' : 'rebuilt from the diff'
              }`}
            >
              {preview.source === 'worktree' ? 'tree' : 'rebuilt'}
            </span>
            {!preview.complete && (
              <span className="badge warn" title="A unified diff only carries the changed hunks">
                partial
              </span>
            )}
          </>
        )}
        <span className="spacer" />
        {!client.isStatic && (
          <span title="Reload the preview">
            <button
              className="ghost icon-only"
              onClick={() => setReloadKey((k) => k + 1)}
              disabled={!previewable}
            >
              <Icon name="refresh" />
            </button>
          </span>
        )}
        {/* mo cannot show an image or a notebook at all, so this whole slot
            - a direct link once mo's frame has loaded, a button that fetches
            and opens it otherwise - only exists for a Markdown file. */}
        {!client.isStatic &&
          format === 'markdown' &&
          (preview?.kind === 'frame' && preview.moUrl ? (
            <a className="ghost button" href={preview.moUrl} target="_blank" rel="noreferrer">
              <MoIcon small />
              mo
              <Icon name="open_in_new" small />
            </a>
          ) : (
            <button className="ghost" onClick={() => void openInMo()} disabled={openingMo}>
              <MoIcon small />
              {openingMo ? 'Opening…' : 'mo'}
              <Icon name="open_in_new" small />
            </button>
          ))}
      </div>

      {!previewable ? (
        <p className="empty">{filePath(file)} has no preview.</p>
      ) : format === 'image' ? (
        !active ? (
          <p className="empty">Not loaded yet…</p>
        ) : imageTrouble !== null ? (
          <span className="preview-asset-missing" role="img" aria-label={`${filePath(file)} - ${imageTrouble}`}
            title={`${filePath(file)} - ${imageTrouble}`}>
            <span className="preview-asset-name">{filePath(file)}</span>
            <span className="preview-asset-why">{imageTrouble}</span>
          </span>
        ) : imageSrc ? (
          <div className="preview-image-wrap" onWheel={onUserScroll} onTouchMove={onUserScroll}>
            <img
              key={imageSrc}
              className="preview-image"
              src={imageSrc}
              alt={filePath(file)}
              onError={() => setImageFailed(true)}
            />
            {imageFailed && <p className="error">The image could not be loaded.</p>}
          </div>
        ) : (
          <p className="empty">
            {filePath(file)} is not in the working tree (it may have been deleted), so there is nothing to
            preview.
          </p>
        )
      ) : !active || loading ? (
        <p className="empty">
          {format === 'markdown' && !renderHere ? 'Asking mo for a preview…' : 'Rendering…'}
        </p>
      ) : error ? (
        <div className="preview-error">
          <p className="error">{error}</p>
          {format === 'markdown' && status && !status.moAvailable && (
            <p className="hint">
              mo renders a richer preview than sbnn's own, and it is not installed here. Install it
              with <code>brew install k1LoW/tap/mo</code> or grab a binary from{' '}
              <a href="https://github.com/k1LoW/mo/releases" target="_blank" rel="noreferrer">
                the releases page
              </a>
              , then reload — or switch back to <strong>preview</strong>, which needs nothing.
            </p>
          )}
        </div>
      ) : preview?.kind === 'source' ? (
        <SourceView path={filePath(file)} content={preview.content} onUserScroll={onUserScroll} />
      ) : preview?.kind === 'html' ? (
        <div
          className={format === 'notebook' ? 'notebook' : 'markdown'}
          // What a selection in here would be a comment on. data-line-anchored
          // is what PreviewSelection looks for: renderMarkdown marked every
          // block with the lines it came from, and a whole preview still
          // numbers them the way the file does - a partial one marks the gaps
          // it skipped instead, so nothing in it can be anchored to a line. A
          // notebook's cells never carry this: its rendered content does not
          // correspond to the raw .ipynb JSON's line numbers at all.
          data-diff-id={diffId}
          data-file-id={file.id}
          data-path={filePath(file)}
          data-line-anchored={format === 'markdown' && preview.complete ? 'true' : undefined}
          onWheel={onUserScroll}
          onTouchMove={onUserScroll}
          dangerouslySetInnerHTML={{ __html: previewHTML }}
        />
      ) : preview?.kind === 'frame' && preview.url ? (
        <iframe
          className="preview-frame"
          src={preview.url}
          title="Markdown preview"
          ref={setFrame}
          style={frameHeight !== null ? { height: `${frameHeight}px` } : undefined}
        />
      ) : preview?.kind === 'frame' && preview.moUrl ? (
        <div className="preview-error">
          <p className="error">The preview cannot be embedded here.</p>
          <p className="hint">
            <a href={preview.moUrl} target="_blank" rel="noreferrer">
              Open it in mo
            </a>{' '}
            instead.
          </p>
        </div>
      ) : (
        <p className="empty">No preview.</p>
      )}
    </section>
  )
}
