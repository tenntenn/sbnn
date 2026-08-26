import { hunksOf, type FileDiff, type ViewMode } from './types'

// Measured in Chromium at the default font size, not guessed: one rendered
// diff row is 19-20px tall and a file's sticky header is 45px. Both are
// only used to estimate a section's height before it is rendered - see
// estimatedHeight - so a different font size costs scroll accuracy, not
// correctness.
const ROW_HEIGHT = 19
const SECTION_CHROME = 48
// A one-line comment thread measured 94px; one carrying a snippet or a
// suggestion is taller, so this is a deliberately coarse allowance.
const COMMENT_HEIGHT = 120
// A file with nothing to tabulate - a binary blob, a pure rename, a mode
// change - draws one `p.empty` line instead of a table. Measured in
// Chromium at the default font size: such a section is 122px tall, against
// the 48px SECTION_CHROME reserves for a folded one, so its body is worth
// 74px. Counting rows would answer bare chrome for it and make the
// scrollbar of a diff full of renames say less than half the truth.
const EMPTY_BODY_HEIGHT = 74

/**
 * estimatedHeight guesses how tall a file's section will be once rendered,
 * for `contain-intrinsic-size`.
 *
 * Together with `content-visibility: auto` this lets the browser skip the
 * layout and paint of every section that is nowhere near the viewport,
 * while still reserving something close to the right space for it, so the
 * scrollbar means what it says. Being wrong costs scroll accuracy while a
 * section is still unrendered, nothing else - which is why this counts the
 * file's real rows rather than assuming a constant.
 */
export function estimatedHeight(
  file: FileDiff,
  folded: boolean,
  comments: number,
  viewMode: ViewMode,
): number {
  // A folded file renders its header and nothing else until it is opened.
  if (folded) return SECTION_CHROME
  const hunks = hunksOf(file)
  // No hunks at all: DiffFileSection draws "Binary file" or "No content
  // change" and no table. Counting rows would answer bare chrome, and
  // iterating file.hunks would throw, since the server sends null there.
  if (hunks.length === 0) {
    return SECTION_CHROME + EMPTY_BODY_HEIGHT + comments * COMMENT_HEIGHT
  }
  let rows = 0
  for (const hunk of hunks) {
    // The hunk's own @@ header is a row too.
    rows += 1
    if (viewMode === 'split') {
      // Side by side, a removed line and the line replacing it share a row.
      let removed = 0
      let added = 0
      for (const line of hunk.lines) {
        if (line.kind === 'delete') removed++
        else if (line.kind === 'add') added++
        else {
          // A run of changed lines ends: it took as many rows as its
          // longer side, and this context line takes one more.
          rows += Math.max(removed, added) + 1
          removed = 0
          added = 0
        }
      }
      rows += Math.max(removed, added)
    } else {
      rows += hunk.lines.length
    }
  }
  return SECTION_CHROME + rows * ROW_HEIGHT + comments * COMMENT_HEIGHT
}
