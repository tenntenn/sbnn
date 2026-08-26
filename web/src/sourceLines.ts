/**
 * SOURCE_PREVIEW_LINES is how many lines of a file the preview pane puts in
 * the DOM before the rest is held back behind a button.
 *
 * A preview is the whole file, and a whole file has no bound: one generated
 * .json in a round of five hundred is enough to mount a hundred thousand
 * rows next to the diff. #103 fixed exactly that shape of problem for the
 * diff pane, and an unbounded preview is where it would come back. Two
 * thousand lines is well past what anyone reads around a hunk and is still
 * cheap to mount; asking for the rest is one click, and is the reader's
 * decision rather than a cost every file pays.
 */
export const SOURCE_PREVIEW_LINES = 2000

/**
 * sourceLines splits a file into the lines a preview draws.
 *
 * Two things it is careful about:
 *
 * A text file ends with a newline, and splitting on it leaves one empty
 * element past the end that is not a line of the file - drawing it would put
 * a numbered blank row under every file. Any other empty element is a real
 * blank line and is kept, because a file's blank lines are part of reading
 * it.
 *
 * A CRLF file's carriage returns are dropped. They are not content: they are
 * how the line ended, the line has already ended, and leaving them in puts a
 * stray control character inside every row and inside anything copied out of
 * one. The server drops them for the same reason when it checks the working
 * tree against a diff (trimLineEnd, internal/server/preview.go).
 */
export function sourceLines(content: string): string[] {
  const lines = content.split('\n')
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].endsWith('\r')) lines[i] = lines[i].slice(0, -1)
  }
  return lines
}
