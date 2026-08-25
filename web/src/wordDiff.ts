export interface Segment {
  text: string
  changed: boolean
}

// The fallback pattern carries the u flag, so `.` is a whole code point and a
// surrogate pair is never cut in half. It still cannot see a grapheme cluster,
// which is why it is only the fallback.
const tokenPattern = /(\s+|[A-Za-z0-9_$]+|.)/gu

/** A run of whitespace, or a run of word characters, is one token. */
const wordGrapheme = /^[A-Za-z0-9_$]\p{M}*$/u
const spaceGrapheme = /^\s+$/u

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
  const seg = segmenter()
  if (!seg) return s.match(tokenPattern) ?? []

  const tokens: string[] = []
  let run: 'word' | 'space' | null = null
  for (const { segment } of seg.segment(s)) {
    const kind = wordGrapheme.test(segment) ? 'word' : spaceGrapheme.test(segment) ? 'space' : null
    if (kind !== null && kind === run) tokens[tokens.length - 1] += segment
    else tokens.push(segment)
    run = kind
  }
  return tokens
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
