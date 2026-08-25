import { useEffect, useMemo, useState } from 'react'
import { ensureHighlightStyles, languageOf } from '../highlight'
import { SOURCE_PREVIEW_LINES, sourceLines } from '../sourceLines'
import { Code } from './Code'

interface Props {
  path: string
  content: string
  onUserScroll?: () => void
}

/**
 * SourceView draws a file as its own numbered lines, syntax coloured by the
 * same highlighter the diff pane uses.
 *
 * The line numbers are the file's, counted from one, and they are right
 * whenever the content is the working tree file. They carry no comment
 * anchor: the section header already says whether this is the tree or a
 * reconstruction, and a line sbnn cannot vouch for must not become the
 * anchor of a comment - which is the reason #23 exists.
 */
export function SourceView({ path, content, onUserScroll }: Props) {
  const language = useMemo(() => languageOf(path), [path])
  const lines = useMemo(() => sourceLines(content), [content])
  const [showAll, setShowAll] = useState(false)

  // A section handed a different file starts bounded again: the reader asked
  // for the whole of one file, not for the whole of every file after it.
  useEffect(() => setShowAll(false), [content])

  ensureHighlightStyles()

  const hidden = showAll ? 0 : Math.max(0, lines.length - SOURCE_PREVIEW_LINES)
  const shown = hidden > 0 ? lines.slice(0, SOURCE_PREVIEW_LINES) : lines

  return (
    <div className="source-preview" onWheel={onUserScroll} onTouchMove={onUserScroll}>
      <ol className="source-lines" data-lines={lines.length}>
        {shown.map((line, i) => (
          // A blank line is drawn as an empty row, not as a row holding a
          // space: the pane is the file, and text copied out of it has to be
          // the file. The row keeps its height from the stylesheet instead.
          <li key={i}>{line === '' ? null : <Code content={line} language={language} />}</li>
        ))}
      </ol>
      {hidden > 0 && (
        <div className="source-more">
          <span className="hint">
            {SOURCE_PREVIEW_LINES.toLocaleString()} of {lines.length.toLocaleString()} lines shown.
          </span>
          <button className="ghost" onClick={() => setShowAll(true)}>
            Show the remaining {hidden.toLocaleString()}
          </button>
        </div>
      )}
    </div>
  )
}
