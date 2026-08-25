import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import type { Comment, Diff, FileDiff, Hunk, Line, ViewMode } from '../types'
import { filePath } from '../types'
import { client } from '../client'
import { wordDiff } from '../wordDiff'
import { CommentForm, CommentThread } from './CommentThread'
import { Icon } from './Icon'

interface Props {
  group: string
  diff: Diff
  file: FileDiff
  comments: Comment[]
  /** narrow is set on a phone, where side by side does not fit. */
  narrow?: boolean
  onChanged: () => void
  /** folded is the fully resolved state (override, or the server's default,
   * already forced open when the file carries comments) - this component
   * only renders it, it does not decide it. */
  folded: boolean
  /** foldedByReader says the fold standing on this file is one the reader
   * performed here, keyed by sectionKey, rather than one the sender asked
   * for with --collapse. The two read the same on screen but have very
   * different explanations, and only the sender's comes with a reason. */
  foldedByReader?: boolean
  onSetFolded: (value: boolean) => void
  /** viewMode is likewise resolved by the caller (an override, or the
   * server's default); a file locked to unified ignores it. */
  viewMode: ViewMode
  onSetViewMode: (mode: ViewMode) => void
}

type Side = 'new' | 'old'

interface Selection {
  side: Side
  start: number
  end: number
}

/** anchorKey identifies the row a comment is rendered below. */
function anchorKey(side: Side, line: number): string {
  return `${side}:${line}`
}

function lineSide(line: Line): Side {
  return line.kind === 'delete' ? 'old' : 'new'
}

function lineNumber(line: Line): number {
  return line.kind === 'delete' ? line.oldNumber : line.newNumber
}

function marker(kind: Line['kind']): string {
  switch (kind) {
    case 'add':
      return '+'
    case 'delete':
      return '-'
    default:
      return ' '
  }
}

/** foldLabel explains a fold in words the reader can check.
 *
 * foldReason is written by the server, for the folds it performs itself;
 * sbnn never folds a file on its own without one. A fold the reader
 * performed has no reason to state and is not the sender's doing, so it
 * says whose it is instead of borrowing an explanation that is not true.
 * The remaining case - folded, no reason, not by the reader - should not
 * arise, and says only what is certain. */
export function foldLabel(byReader: boolean, foldReason: string | undefined): string {
  if (byReader) return 'Folded by you'
  if (foldReason) return `Folded — ${foldReason}`
  return 'Folded'
}

export function DiffFileSection({
  group,
  diff,
  file,
  comments,
  narrow = false,
  onChanged,
  folded,
  foldedByReader,
  onSetFolded,
  viewMode,
  onSetViewMode,
}: Props) {
  // A new or deleted file has only one side, so side by side makes no sense
  // for it and the toggle stays locked on unified. A narrow screen has no
  // room for two columns either.
  const locked = narrow || file.status === 'added' || file.status === 'deleted' || file.isBinary
  const [selection, setSelection] = useState<Selection | null>(null)
  const mode: ViewMode = locked ? 'unified' : viewMode

  const commentsByAnchor = useMemo(() => {
    const map = new Map<string, Comment[]>()
    for (const c of comments) {
      const key = anchorKey(c.side, c.endLine)
      const list = map.get(key)
      if (list) list.push(c)
      else map.set(key, [c])
    }
    return map
  }, [comments])

  // A range is selected by dragging over the line numbers, by shift-clicking,
  // or - which is what a touch screen has - by tapping another line while the
  // draft is still open. The draft form only appears once the drag is over:
  // inserting it mid-drag would push the rows under the pointer away.
  const dragging = useRef(false)
  const [drafting, setDrafting] = useState(true)

  useEffect(() => {
    const stop = () => {
      if (!dragging.current) return
      dragging.current = false
      setDrafting(true)
    }
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
    return () => {
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
    }
  }, [])

  const extendTo = (side: Side, line: number) => {
    if (line <= 0) return
    setSelection((current) => {
      if (!current || current.side !== side) return { side, start: line, end: line }
      if (line < current.start) return { ...current, start: line }
      return { ...current, end: line }
    })
  }

  const pick = (side: Side, line: number, extend: boolean) => {
    setSelection((current) => {
      const grow = extend || (current !== null && current.side === side)
      if (grow && current && current.side === side) {
        if (line < current.start) return { ...current, start: line }
        return { ...current, end: line }
      }
      return { side, start: line, end: line }
    })
  }

  // Pressing on the gutter may be the start of a drag, so the form waits for
  // the pointer to come up.
  const select = (side: Side, line: number, extend: boolean) => {
    if (line <= 0) return
    dragging.current = true
    setDrafting(false)
    pick(side, line, extend)
  }

  // Clicking the code itself is never a drag - the pointer is already back
  // up by the time a click arrives, so waiting for pointerup would wait
  // forever - and the text stays selectable, which is why it is not a
  // pointerdown.
  const selectLine = (side: Side, line: number, extend: boolean) => {
    if (line <= 0) return
    dragging.current = false
    setDrafting(true)
    pick(side, line, extend)
  }

  // Dragging across the gutter grows the range under the pointer.
  const dragOver = (side: Side, line: number) => {
    if (!dragging.current) return
    extendTo(side, line)
  }

  const submitComment = async (body: string, question: boolean) => {
    if (!selection) return
    await client.addComment(group, {
      diffId: diff.id,
      fileId: file.id,
      path: filePath(file),
      side: selection.side,
      startLine: selection.start,
      endLine: selection.end,
      body,
      question,
      snippet: snippetFor(file, selection),
    })
    setSelection(null)
    onChanged()
  }

  const selectionLabel = selection
    ? `${filePath(file)}:${selection.start}${selection.end > selection.start ? `-${selection.end}` : ''}` +
      `${selection.side === 'old' ? ' (old)' : ''}`
    : ''

  const renderExtras = (side: Side, line: number) => {
    const anchored = commentsByAnchor.get(anchorKey(side, line)) ?? []
    const showForm = drafting && selection !== null && selection.side === side && selection.end === line
    if (anchored.length === 0 && !showForm) return null
    return (
      <>
        {anchored.length > 0 && (
          <CommentThread group={group} comments={anchored} onChanged={onChanged} />
        )}
        {showForm && selection && (
          <CommentForm
            label={selectionLabel}
            seed={currentText(file, selection)}
            canSuggest={selection.side === 'new'}
            hint="Drag or tap another line number to cover more lines"
            onSubmit={submitComment}
            onCancel={() => setSelection(null)}
          />
        )}
      </>
    )
  }

  return (
    <div className="diff">
      <div className="diff-header">
        <button
          className="diff-title"
          onClick={() => onSetFolded(!folded)}
          aria-expanded={!folded}
          title={folded ? 'Show this file' : 'Fold this file away'}
        >
          <span className="disclosure">
            <Icon name={folded ? 'chevron_right' : 'expand_more'} small />
          </span>
          <span className={`status status-${file.status}`}>{file.status}</span>
          <span className="path">
            {file.status === 'renamed' || file.status === 'copied'
              ? `${file.oldPath} → ${file.newPath}`
              : filePath(file)}
          </span>
          <span className="stat add">+{file.additions}</span>
          <span className="stat del">-{file.deletions}</span>
        </button>
        <div className="diff-tools">
          {locked ? (
            <span
              className="hint"
              title={
                narrow
                  ? 'Side by side needs a wider window'
                  : 'A file without an old side is always shown unified'
              }
            >
              unified
            </span>
          ) : (
            <div className="toggle">
              <button
                className={mode === 'split' ? 'active' : ''}
                onClick={() => onSetViewMode('split')}
                title="Old and new side by side"
              >
                <Icon name="view_column" small />
                split
              </button>
              <button
                className={mode === 'unified' ? 'active' : ''}
                onClick={() => onSetViewMode('unified')}
                title="Old and new lines in one column"
              >
                <Icon name="table_rows" small />
                unified
              </button>
            </div>
          )}
        </div>
      </div>

      {folded ? (
        <p className="empty">
          {foldLabel(foldedByReader === true, file.foldReason)} · {file.additions + file.deletions}{' '}
          changed lines
        </p>
      ) : file.isBinary ? (
        <p className="empty">Binary file — no diff to show.</p>
      ) : file.hunks.length === 0 ? (
        <p className="empty">
          No content change{file.oldMode && file.newMode ? ` (mode ${file.oldMode} → ${file.newMode})` : ''}.
        </p>
      ) : mode === 'unified' ? (
        <UnifiedTable
          hunks={file.hunks}
          selection={selection}
          onSelect={select}
          onSelectLine={selectLine}
          onDragOver={dragOver}
          renderExtras={renderExtras}
        />
      ) : (
        <SplitTable
          hunks={file.hunks}
          selection={selection}
          onSelect={select}
          onSelectLine={selectLine}
          onDragOver={dragOver}
          renderExtras={renderExtras}
        />
      )}
    </div>
  )
}

interface TableProps {
  hunks: Hunk[]
  selection: Selection | null
  // onSelect starts a possible drag on the gutter; onSelectLine is a plain
  // click on the code, which cannot become one.
  onSelect: (side: Side, line: number, extend: boolean) => void
  onSelectLine: (side: Side, line: number, extend: boolean) => void
  onDragOver: (side: Side, line: number) => void
  renderExtras: (side: Side, line: number) => React.ReactNode
}

function isSelected(selection: Selection | null, side: Side, line: number): boolean {
  return (
    selection !== null &&
    selection.side === side &&
    line >= selection.start &&
    line <= selection.end
  )
}

function UnifiedTable({ hunks, selection, onSelect, onSelectLine, onDragOver, renderExtras }: TableProps) {
  return (
    <table className="diff-table unified">
      <colgroup>
        <col className="col-num" />
        <col className="col-num" />
        <col className="col-marker" />
        <col />
      </colgroup>
      <tbody>
        {hunks.map((hunk, hi) => (
          <Fragment key={hi}>
            <tr className="hunk">
              <td className="num" />
              <td className="num" />
              <td className="code" colSpan={2}>
                {hunk.header}
              </td>
            </tr>
            {hunk.lines.map((line, li) => {
              const side = lineSide(line)
              const num = lineNumber(line)
              // A context line stands on both sides, so a comment on
              // either of them belongs under it, and the old gutter
              // selects the old side rather than nothing.
              const oldExtras = line.oldNumber > 0 ? renderExtras('old', line.oldNumber) : null
              const newExtras = line.newNumber > 0 ? renderExtras('new', line.newNumber) : null
              const extras =
                oldExtras || newExtras ? (
                  <>
                    {oldExtras}
                    {newExtras}
                  </>
                ) : null
              const selected =
                isSelected(selection, 'old', line.oldNumber) || isSelected(selection, 'new', line.newNumber)
              return (
                <Fragment key={li}>
                  <tr className={`line ${line.kind}${selected ? ' selected' : ''}`}>
                    <td
                      className="num clickable"
                      onPointerDown={(ev) => onSelect('old', line.oldNumber, ev.shiftKey)}
                      onPointerEnter={() => onDragOver('old', line.oldNumber)}
                    >
                      {line.oldNumber > 0 ? line.oldNumber : ''}
                    </td>
                    <td
                      className="num clickable"
                      onPointerDown={(ev) => onSelect('new', line.newNumber, ev.shiftKey)}
                      onPointerEnter={() => onDragOver('new', line.newNumber)}
                    >
                      {line.newNumber > 0 ? line.newNumber : ''}
                    </td>
                    <td className="marker">{marker(line.kind)}</td>
                    <td className="code" onClick={(ev) => onSelectLine(side, num, ev.shiftKey)}>
                      {line.content || ' '}
                      {line.noNewline && <span className="hint"> (no newline at end of file)</span>}
                    </td>
                  </tr>
                  {extras && (
                    <tr className="extras">
                      <td colSpan={4}>{extras}</td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </Fragment>
        ))}
      </tbody>
    </table>
  )
}

interface SplitRow {
  left?: Line
  right?: Line
  /** paired marks a removed/added pair, which gets word level highlighting. */
  paired: boolean
}

/** buildSplitRows lays the lines of a hunk out in two columns. */
function buildSplitRows(lines: Line[]): SplitRow[] {
  const rows: SplitRow[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (line.kind === 'context') {
      rows.push({ left: line, right: line, paired: false })
      i++
      continue
    }
    const removed: Line[] = []
    const added: Line[] = []
    while (i < lines.length && lines[i].kind === 'delete') removed.push(lines[i++])
    while (i < lines.length && lines[i].kind === 'add') added.push(lines[i++])
    const count = Math.max(removed.length, added.length)
    for (let k = 0; k < count; k++) {
      rows.push({
        left: removed[k],
        right: added[k],
        paired: removed[k] !== undefined && added[k] !== undefined,
      })
    }
  }
  return rows
}

function SplitTable({ hunks, selection, onSelect, onSelectLine, onDragOver, renderExtras }: TableProps) {
  return (
    <table className="diff-table side-by-side">
      <colgroup>
        <col className="col-num" />
        <col className="col-side" />
        <col className="col-num" />
        <col className="col-side" />
      </colgroup>
      <tbody>
        {hunks.map((hunk, hi) => (
          <Fragment key={hi}>
            <tr className="hunk">
              <td className="num" />
              <td className="code" colSpan={3}>
                {hunk.header}
              </td>
            </tr>
            {buildSplitRows(hunk.lines).map((row, ri) => {
              const [oldSegments, newSegments] = row.paired
                ? wordDiff(row.left?.content ?? '', row.right?.content ?? '')
                : [null, null]
              const leftExtras = row.left ? renderExtras('old', row.left.oldNumber) : null
              const rightExtras = row.right ? renderExtras('new', row.right.newNumber) : null
              const hasExtras = Boolean(leftExtras || rightExtras)
              return (
                <Fragment key={ri}>
                  <tr className="line">
                    <td
                      className={`num clickable${isSelected(selection, 'old', row.left?.oldNumber ?? -1) ? ' selected' : ''}`}
                      onPointerDown={(ev) => row.left && onSelect('old', row.left.oldNumber, ev.shiftKey)}
                      onPointerEnter={() => row.left && onDragOver('old', row.left.oldNumber)}
                    >
                      {row.left && row.left.oldNumber > 0 ? row.left.oldNumber : ''}
                    </td>
                    <td
                      className={`code side ${row.left ? row.left.kind : 'empty'}${
                        isSelected(selection, 'old', row.left?.oldNumber ?? -1) ? ' selected' : ''
                      }`}
                      onClick={(ev) => row.left && onSelectLine('old', row.left.oldNumber, ev.shiftKey)}
                    >
                      {row.left ? renderSegments(row.left.content, oldSegments) : ''}
                    </td>
                    <td
                      className={`num clickable${isSelected(selection, 'new', row.right?.newNumber ?? -1) ? ' selected' : ''}`}
                      onPointerDown={(ev) => row.right && onSelect('new', row.right.newNumber, ev.shiftKey)}
                      onPointerEnter={() => row.right && onDragOver('new', row.right.newNumber)}
                    >
                      {row.right && row.right.newNumber > 0 ? row.right.newNumber : ''}
                    </td>
                    <td
                      className={`code side ${row.right ? row.right.kind : 'empty'}${
                        isSelected(selection, 'new', row.right?.newNumber ?? -1) ? ' selected' : ''
                      }`}
                      onClick={(ev) => row.right && onSelectLine('new', row.right.newNumber, ev.shiftKey)}
                    >
                      {row.right ? renderSegments(row.right.content, newSegments) : ''}
                    </td>
                  </tr>
                  {hasExtras && (
                    <tr className="extras">
                      <td colSpan={4}>
                        {leftExtras}
                        {rightExtras}
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </Fragment>
        ))}
      </tbody>
    </table>
  )
}

function renderSegments(content: string, segments: { text: string; changed: boolean }[] | null) {
  if (!segments) return content || ' '
  return (
    <>
      {segments.map((seg, i) =>
        seg.changed ? (
          <mark key={i}>{seg.text}</mark>
        ) : (
          <Fragment key={i}>{seg.text}</Fragment>
        ),
      )}
      {content === '' ? ' ' : null}
    </>
  )
}

/** selectedLines returns the lines of the selected range on its side. */
function selectedLines(file: FileDiff, selection: Selection): Line[] {
  const out: Line[] = []
  for (const hunk of file.hunks) {
    for (const line of hunk.lines) {
      const num = selection.side === 'old' ? line.oldNumber : line.newNumber
      if (num < selection.start || num > selection.end) continue
      if (selection.side === 'old' && line.kind === 'add') continue
      if (selection.side === 'new' && line.kind === 'delete') continue
      out.push(line)
    }
  }
  return out
}

/** snippetFor collects the reviewed lines so the comment stays readable
 * outside the browser, for instance in `sbnn comments`. */
function snippetFor(file: FileDiff, selection: Selection): string {
  return selectedLines(file, selection)
    .map((line) => `${marker(line.kind)}${line.content}`)
    .join('\n')
}

/** currentText is what the selected lines say today, which is where a
 * suggested replacement starts from. */
function currentText(file: FileDiff, selection: Selection): string {
  return selectedLines(file, selection)
    .map((line) => line.content)
    .join('\n')
}
