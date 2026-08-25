import { useEffect, useMemo, useState } from 'react'
import type { FileDiff, PreviewKind, Status } from '../types'
import { filePath } from '../types'
import { client, type PreviewResult } from '../client'
import { Icon } from './Icon'
import { MoIcon } from './MoIcon'

interface Props {
  group: string
  diffId: string
  file: FileDiff
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

/** Format is which of sbnn's three renderers a file's preview uses. mo only
 * ever renders Markdown - it cannot show an image or a notebook at all - so
 * those two always use sbnn's own renderer regardless of the page's kind
 * toggle. */
type Format = 'markdown' | 'image' | 'notebook' | null

function formatOf(file: FileDiff): Format {
  if (file.isMarkdown) return 'markdown'
  if (file.isNotebook) return 'notebook'
  if (file.isImage) return 'image'
  return null
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
  for (const hunk of file.hunks) {
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
    file.hunks.length,
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
export function PreviewFileSection({ group, diffId, file, status, kind, active, onUserScroll }: Props) {
  const [preview, setPreview] = useState<PreviewResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [openingMo, setOpeningMo] = useState(false)
  const [imageFailed, setImageFailed] = useState(false)

  const format = formatOf(file)
  const previewable = format !== null
  // The loader below is keyed on this string rather than on file itself,
  // so an event that only rebuilds the object leaves the preview alone.
  const revision = useMemo(() => previewRevision(file), [file])
  const renderHere = format !== 'markdown' || kind === 'preview'

  const rawImageSrc = format === 'image' ? client.imageSrc(group, diffId, file.id) : undefined
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
    if (format !== 'markdown' && format !== 'notebook') {
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
          dangerouslySetInnerHTML={{ __html: preview.html }}
        />
      ) : preview?.kind === 'frame' && preview.url ? (
        <iframe className="preview-frame" src={preview.url} title="Markdown preview" />
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
