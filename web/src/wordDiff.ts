export interface Segment {
  text: string
  changed: boolean
}

// The ASCII pattern. Every ASCII character is a grapheme cluster of its own
// except CRLF, which this groups into the same whitespace run the segmenter
// would produce, so on an ASCII line the two paths agree token for token.
//
// It also carries the u flag, so where it stands in for the segmenter - a
// runtime with no Intl.Segmenter - `.` is at least a whole code point and a
// surrogate pair is never cut in half. It still cannot see a grapheme
// cluster, which is why that is only a fallback.
const tokenPattern = /(\s+|[A-Za-z0-9_$]+|.)/gu

// nonASCII decides which path a line takes. Nearly every line of nearly every
// diff is ASCII, and the segmenter costs an order of magnitude more per line
// than the pattern does, so a line that cannot contain a multi-character
// grapheme cluster does not pay for one.
const nonASCII = /[^\x00-\x7f]/

/** A run of whitespace, or a run of word characters, is one token. */
const wordGrapheme = /^[A-Za-z0-9_$]\p{M}*$/u
const spaceGrapheme = /^\s+$/u

// A cluster that carries a variation selector or an enclosing keycap is a
// picture, whatever it starts with. "1\uFE0F\u20E3" is the emoji 1, not the
// digit 1, and gluing it onto the word beside it - which wordGrapheme alone
// does, because both are combining marks - puts the highlight boundary in the
// wrong place and hides the change.
const pictorial = /[\u{FE0F}\u{20E3}]/u

let graphemes: Intl.Segmenter | null | undefined

/**
 * segmenter returns the grapheme segmenter, or null where the runtime has no
 * Intl.Segmenter and tokenize has to fall back to the pattern above.
 */
function segmenter(): Intl.Segmenter | null {
  if (graphemes === undefined) {
    graphemes =
      typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function'
        ? new Intl.Segmenter(undefined, { granularity: 'grapheme' })
        : null
  }
  return graphemes
}

/**
 * tokenize cuts a line into the units wordDiff may put a boundary between.
 *
 * The unit is a grapheme cluster - what a reader calls one character - so a
 * surrogate pair, a combining accent, a ZWJ family, a regional-indicator flag
 * and a skin-tone modifier each stay whole. Runs of whitespace and runs of
 * word characters are then glued back into single tokens, so the highlight
 * still lands on words rather than on letters.
 */
function tokenize(s: string): string[] {
  const seg = nonASCII.test(s) ? segmenter() : null
  if (!seg) return s.match(tokenPattern) ?? []

  const tokens: string[] = []
  let run: 'word' | 'space' | null = null
  for (const { segment } of seg.segment(s)) {
    const kind = kindOf(segment)
    if (kind !== null && kind === run) tokens[tokens.length - 1] += segment
    else tokens.push(segment)
    run = kind
  }
  return tokens
}

/**
 * kindOf says whether a grapheme cluster joins the run beside it, and as
 * what. Most clusters on a line are one plain character, and answering those
 * from their code point costs a fraction of what the patterns do.
 */
function kindOf(segment: string): 'word' | 'space' | null {
  if (segment.length === 1) {
    const c = segment.charCodeAt(0)
    if (
      (c >= 97 && c <= 122) || // a-z
      (c >= 65 && c <= 90) || // A-Z
      (c >= 48 && c <= 57) || // 0-9
      c === 95 || // _
      c === 36 // $
    ) {
      return 'word'
    }
    if (c > 127) return spaceGrapheme.test(segment) ? 'space' : null
    // Space, tab, CR, LF, form feed, vertical tab.
    return c === 32 || (c >= 9 && c <= 13) ? 'space' : null
  }
  if (wordGrapheme.test(segment) && !pictorial.test(segment)) return 'word'
  return spaceGrapheme.test(segment) ? 'space' : null
}

/**
 * wordDiff highlights what actually changed between a removed and an added
 * line by trimming the common head and tail at token boundaries. It is
 * deliberately simple: a full diff of every line would cost more than it is
 * worth while reviewing.
 */
export function wordDiff(oldLine: string, newLine: string): [Segment[], Segment[]] {
  const a = tokenize(oldLine)
  const b = tokenize(newLine)

  let head = 0
  while (head < a.length && head < b.length && a[head] === b[head]) head++

  let tail = 0
  while (
    tail < a.length - head &&
    tail < b.length - head &&
    a[a.length - 1 - tail] === b[b.length - 1 - tail]
  ) {
    tail++
  }

  const build = (tokens: string[]): Segment[] => {
    const segments: Segment[] = []
    const push = (text: string, changed: boolean) => {
      if (!text) return
      const last = segments[segments.length - 1]
      if (last && last.changed === changed) last.text += text
      else segments.push({ text, changed })
    }
    push(tokens.slice(0, head).join(''), false)
    push(tokens.slice(head, tokens.length - tail).join(''), true)
    push(tokens.slice(tokens.length - tail).join(''), false)
    return segments
  }

  return [build(a), build(b)]
}
