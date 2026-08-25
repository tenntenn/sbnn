/**
 * A suggested change lives inside the comment body as a fenced block whose
 * info string is "suggestion", exactly like on GitHub. This module is the
 * one place that knows how to read and write those blocks.
 *
 * It has to read them the way `internal/model` does, or the page offers a
 * change the server will not apply - and refuses one it would.
 */

export type Segment =
  | { kind: 'text'; text: string }
  | { kind: 'suggestion'; text: string }

interface Fence {
  fence: string
  info: string
}

/** openFence reports whether a line opens a fenced block, with which fence
 * and with which info string. A backtick fence may not carry a backtick in
 * its info string, so a line of prose holding two code spans opens nothing. */
function openFence(line: string): Fence | null {
  const trimmed = line.replace(/\r$/, '').trim()
  if (trimmed === '' || (trimmed[0] !== '`' && trimmed[0] !== '~')) return null
  const marker = trimmed[0]
  let n = 0
  while (n < trimmed.length && trimmed[n] === marker) n++
  if (n < 3) return null
  const info = trimmed.slice(n).trim()
  if (marker === '`' && info.includes('`')) return null
  return { fence: trimmed.slice(0, n), info }
}

function isSuggestion(open: Fence): boolean {
  return open.info.toLowerCase() === 'suggestion'
}

/** closesFence reports whether a line closes a block opened with fence: the
 * same character, at least as long, and nothing else on the line. */
function closesFence(line: string, fence: string): boolean {
  const trimmed = line.replace(/\r$/, '').trim()
  if (trimmed.length < fence.length) return false
  const marker = fence[0]
  return trimmed.split('').every((c) => c === marker)
}

/** endsSuggestion reports whether a line ends a suggestion block opened with
 * fence, given the fence of the code block nested inside it - empty when the
 * scan is not inside one.
 *
 * Inside a nested block only that block's own closing fence is read, which is
 * what keeps a proposed code block whole. The exception is a run longer than
 * the nested fence: a close never has to be longer than the fence it closes,
 * while the suggestion's fence is lengthened precisely to hold shorter ones,
 * so the longer run belongs to the suggestion. */
function endsSuggestion(line: string, fence: string, inner: string): boolean {
  if (!closesFence(line, fence)) return false
  if (inner === '') return true
  const run = line.replace(/\r$/, '').trim()
  return !closesFence(line, inner) || run.length > inner.length
}

/** blockEnd returns the index of the line closing the fenced block opened at
 * lines[start], or lines.length when the text never closes it. */
function blockEnd(lines: string[], start: number): number {
  const open = openFence(lines[start])
  if (!open) return start
  if (!isSuggestion(open)) {
    for (let i = start + 1; i < lines.length; i++) {
      if (closesFence(lines[i], open.fence)) return i
    }
    return lines.length
  }
  // inner is the fence of the code block nested inside the suggestion while
  // the scan is inside one, so a suggestion may propose a file that itself
  // contains a code block.
  let inner = ''
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i]
    if (endsSuggestion(line, open.fence, inner)) return i
    if (inner !== '') {
      if (closesFence(line, inner)) inner = ''
      continue
    }
    const nested = openFence(line)
    if (nested) inner = nested.fence
  }
  return lines.length
}

/** parseBody splits a comment body into prose and suggested changes. */
export function parseBody(body: string): Segment[] {
  const segments: Segment[] = []
  const lines = body.split('\n')
  let text: string[] = []

  const flushText = () => {
    const joined = text.join('\n').replace(/^\n+|\n+$/g, '')
    if (joined !== '') segments.push({ kind: 'text', text: joined })
    text = []
  }

  for (let i = 0; i < lines.length; i++) {
    const open = openFence(lines[i])
    if (!open) {
      text.push(lines[i])
      continue
    }
    const end = blockEnd(lines, i)
    if (isSuggestion(open)) {
      flushText()
      const block: string[] = []
      for (let j = i + 1; j < end; j++) block.push(lines[j].replace(/\r$/, ''))
      segments.push({ kind: 'suggestion', text: block.join('\n') })
    } else {
      // What is written inside another fenced block is quoted text, not
      // Markdown, so a suggestion block there was shown rather than
      // proposed. It stays prose, fences and all.
      for (let j = i; j <= end && j < lines.length; j++) text.push(lines[j])
    }
    i = end
  }
  flushText()
  return segments
}

/** suggestions returns just the proposed replacements of a body. */
export function suggestions(body: string): string[] {
  return parseBody(body)
    .filter((s): s is { kind: 'suggestion'; text: string } => s.kind === 'suggestion')
    .map((s) => s.text)
}

/** suggestionBlock writes a suggestion the way a comment body carries it. */
export function suggestionBlock(text: string): string {
  const content = text.replace(/\n+$/, '')
  let fence = '```'
  while (content.includes(fence)) fence += '`'
  return `${fence}suggestion\n${content}\n${fence}`
}

/** danglingFence returns the fence of a block the text opens and never
 * closes, or '' when every block it opens is closed. At most one can be left
 * open, since every line after it belongs to it. */
function danglingFence(text: string): string {
  const lines = text.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const open = openFence(lines[i])
    if (!open) continue
    const end = blockEnd(lines, i)
    if (end === lines.length) return open.fence
    i = end
  }
  return ''
}

/** withSuggestion appends a suggestion block to a body. */
export function withSuggestion(body: string, text: string): string {
  if (text.trim() === '') return body
  const block = suggestionBlock(text)
  if (body.trim() === '') return block
  let head = body.replace(/\n+$/, '')
  // A body that opens a fenced block and never closes it would hold the
  // appended suggestion inside that block, where it reads as quoted rather
  // than proposed. Closing the block first only writes down where it was
  // going to end anyway.
  const dangling = danglingFence(head)
  if (dangling !== '') head += `\n${dangling}`
  return `${head}\n\n${block}`
}

/** originalLines are the lines a suggestion would replace, taken from the
 * snippet stored with the comment (its diff markers removed). */
export function originalLines(snippet: string): string[] {
  if (snippet === '') return []
  return snippet
    .split('\n')
    .filter((line) => !line.startsWith('-'))
    .map((line) => (line.startsWith('+') || line.startsWith(' ') ? line.slice(1) : line))
}
