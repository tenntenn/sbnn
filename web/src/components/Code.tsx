import { Fragment } from 'react'
import { highlightLine, tokenClass, type LanguageId } from '../highlight'

/**
 * Code is one line of source, coloured if the file's extension is one the
 * highlighter knows.
 *
 * Tokens are spans - never a string of HTML - so nothing here can put markup
 * from a diff, or from a file on disk, into the page.
 *
 * It lives on its own because two panes draw source lines: the diff, forty
 * lines at a time out of a hunk, and the preview, a whole file at a time.
 * Both want exactly this, and a second copy of it would be a second place
 * for the escaping to be got wrong.
 */
export function Code({ content, language }: { content: string; language: LanguageId | null }) {
  const tokens = highlightLine(content, language)
  // A line the highlighter had nothing to say about is one text node, not a
  // span wrapping one: an unknown language must cost no more DOM than it did
  // before there was a highlighter. The space stands in for an empty line so
  // that the row still has a height.
  if (tokens.length === 1 && tokens[0].kind === 'plain') return <>{content || ' '}</>
  return (
    <>
      {tokens.map((token, i) =>
        token.kind === 'plain' ? (
          <Fragment key={i}>{token.text}</Fragment>
        ) : (
          <span key={i} className={tokenClass(token.kind)}>
            {token.text}
          </span>
        ),
      )}
    </>
  )
}
