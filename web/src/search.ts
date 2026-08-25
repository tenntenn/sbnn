import type { Diff, FileDiff } from './types'
import { filePath } from './types'
import { sectionKey } from './sectionKey'

/**
 * SEARCH_DEBOUNCE_MS is how long typing settles before the lines are read.
 *
 * Paths are a few thousand short strings and could be filtered on every
 * keystroke; hunk content is the whole review, so it waits until the reader
 * has stopped typing. The input itself is never debounced - what is shown in
 * the box is always what was just typed.
 */
export const SEARCH_DEBOUNCE_MS = 120

/**
 * MAX_SCANNED_LINES caps how many hunk lines one search may read.
 *
 * A review of five hundred files is a few hundred thousand lines, and a
 * search that walks all of them on a slow machine is a search that stutters.
 * Past the cap the content scan stops and the result says so, so a partial
 * answer is never handed over as if it were the whole one. Paths are cheap
 * and keep being matched either way.
 */
export const MAX_SCANNED_LINES = 200_000

/** queryTerms splits a query the way a search reads it: whitespace-separated,
 * case-folded, empties dropped. */
export function queryTerms(query: string): string[] {
  return query.toLowerCase().split(/\s+/).filter(Boolean)
}

/** containsAll reports whether text holds every term, ignoring case. */
function containsAll(text: string, terms: string[]): boolean {
  if (terms.length === 0) return true
  const haystack = text.toLowerCase()
  return terms.every((term) => haystack.includes(term))
}

/**
 * matchesPath reports whether a path answers a search.
 *
 * Every whitespace-separated term has to appear somewhere in the path,
 * ignoring case - so "server go" and "internal/server" both find
 * internal/server/server.go. Nothing turns up that does not contain what
 * was typed. A looser match (the letters in order, anywhere)
 * would find more, and would also find things the reader did not ask for,
 * which in a list you are scanning is worse than finding nothing.
 */
export function matchesPath(path: string, query: string): boolean {
  return containsAll(path, queryTerms(query))
}

/** FileMatch is why one file answered a search. */
export interface FileMatch {
  /** inPath is whether the file's own path holds every term. */
  inPath: boolean
  /** lines is how many hunk lines hold every term. A line, not a file, is
   * the unit the reader is looking for. */
  lines: number
  /** firstLine is the sectionKey-relative position of the first matching
   * line, kept so a jump has somewhere to aim; it is the hunk index and the
   * line index within it. */
  firstLine: { hunk: number; line: number } | null
  /** scanned is how many lines were actually read, which is fewer than the
   * file holds once the budget runs out. */
  scanned: number
  /** truncated is whether the scan stopped with lines left unread. */
  truncated: boolean
}

/**
 * searchFile answers a search for one file: does the path match, and which
 * of its lines do.
 *
 * The same rule applies to both sides: every term has to be present,
 * ignoring case. For content that means present in one single line, which is
 * what makes a count of lines meaningful - "3 lines" is three places worth
 * opening, not three coincidences spread over a file.
 *
 * lineBudget caps the read; a file that runs out comes back truncated rather
 * than pretending its remaining lines held nothing.
 */
export function searchFile(file: FileDiff, terms: string[], lineBudget = Infinity): FileMatch {
  const inPath = containsAll(filePath(file), terms)
  let lines = 0
  let scanned = 0
  let firstLine: FileMatch['firstLine'] = null
  let truncated = false

  if (terms.length > 0) {
    outer: for (let h = 0; h < file.hunks.length; h++) {
      const hunk = file.hunks[h]
      for (let l = 0; l < hunk.lines.length; l++) {
        if (scanned >= lineBudget) {
          truncated = true
          break outer
        }
        scanned++
        if (containsAll(hunk.lines[l].content, terms)) {
          lines++
          if (firstLine === null) firstLine = { hunk: h, line: l }
        }
      }
    }
  }

  return { inPath, lines, firstLine, scanned, truncated }
}

/** SearchResults is one query answered over a whole review. */
export interface SearchResults {
  /** active is whether anything was actually searched for. An empty query is
   * not a search that found everything; it is no search at all. */
  active: boolean
  terms: string[]
  /** matches holds only the files that answered, keyed by sectionKey so two
   * rounds touching the same path stay apart. */
  matches: Map<string, FileMatch>
  /** files and lines are the totals behind the summary in the header. */
  files: number
  lines: number
  /** truncated is whether the content scan hit its budget. The reader is
   * told; results are never quietly partial. */
  truncated: boolean
}

const EMPTY: SearchResults = {
  active: false,
  terms: [],
  matches: new Map(),
  files: 0,
  lines: 0,
  truncated: false,
}

/**
 * searchDiffs matches a query against every file of every round, by path and
 * by hunk content.
 *
 * Content is already in memory - Diff.files[].hunks[].lines[].content is the
 * full text of every changed and context line the server sent - so asking
 * "which files touch SBNN_TARGET" costs a walk of what is on screen, not a
 * round trip.
 */
export function searchDiffs(
  diffs: Diff[],
  query: string,
  maxLines = MAX_SCANNED_LINES,
): SearchResults {
  const terms = queryTerms(query)
  if (terms.length === 0) return EMPTY

  const matches = new Map<string, FileMatch>()
  let files = 0
  let lines = 0
  let budget = maxLines
  let truncated = false

  for (const diff of diffs) {
    for (const file of diff.files) {
      const match = searchFile(file, terms, budget)
      budget -= match.scanned
      if (match.truncated) truncated = true
      if (!match.inPath && match.lines === 0) continue
      matches.set(sectionKey(diff.id, file.id), match)
      files++
      lines += match.lines
    }
  }

  return { active: true, terms, matches, files, lines, truncated }
}

/**
 * matchSummary says where a file was hit, so a result that came from the
 * content is not mistaken for one that came from the path.
 */
export function matchSummary(match: FileMatch): string {
  const parts: string[] = []
  if (match.inPath) parts.push('path')
  if (match.lines > 0) parts.push(`${match.lines} line${match.lines === 1 ? '' : 's'}`)
  return parts.join(' + ')
}
