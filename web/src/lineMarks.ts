import { Lexer, Parser, Renderer, type Token, type Tokens } from 'marked'

/**
 * lineMarks turns Markdown into HTML that remembers which source lines each
 * part of it came from, down to a single line of a paragraph, a list item, a
 * row of a table or a line of a code block.
 *
 * It is kept apart from markdown.ts because it has nothing to do with the
 * sanitiser: it only needs marked, so it can be run - and tested - where there
 * is no DOM.
 *
 * Every mark is an empty element carrying the line it stands at, and sits at
 * the start of that line's content. Because a mark takes no part in the
 * text it is next to, an emphasis or a link that runs across lines is left in
 * one piece; a reader of the page finds the line a point in the text is on by
 * looking for the last mark before it. A whole block also carries the range of
 * its lines, as a fallback for the blocks with no finer structure (a rule, a
 * raw HTML block).
 */

const MARKED_OPTIONS = { async: false, gfm: true, breaks: false } as const

/** LineToken is a token that annotate has told its first source line. */
type LineToken = Token & { ln?: number }

function countChar(s: string, ch: string): number {
  let n = 0
  for (let i = 0; i < s.length; i++) if (s[i] === ch) n++
  return n
}

/** lastLine is the line a token's raw text stops on. A token whose raw text
 * ends in a newline stops on the line before the one that newline opens. */
function lastLine(token: Token, start: number): number {
  const raw = token.raw ?? ''
  const newlines = countChar(raw, '\n')
  return Math.max(raw.endsWith('\n') ? start + newlines - 1 : start + newlines, start)
}

/**
 * annotate records on every token the line it starts on, by counting the
 * newlines of the raw text of everything before it. A child starts where its
 * parent does: the raw text of a nested token has the markers of its
 * container (a bullet, a quote's >, an indent) taken off, but not its
 * newlines.
 */
function annotate(tokens: Token[], start: number): number {
  let line = start
  for (const token of tokens as LineToken[]) {
    token.ln = line
    if (token.type === 'list') {
      annotate((token as Tokens.List).items, line)
    } else if ('tokens' in token && Array.isArray(token.tokens)) {
      annotate(token.tokens, line)
    }
    line += countChar(token.raw ?? '', '\n')
  }
  return line
}

/** MarkRenderer is marked's renderer with a mark added at the start of every
 * line it can tell the source line of. */
class MarkRenderer extends Renderer {
  private readonly attr: string

  constructor(attr: string) {
    super()
    this.attr = attr
  }

  private mark(from: number | undefined, to?: number): string {
    if (from === undefined) return ''
    const value = to === undefined || to <= from ? `${from}` : `${from}-${to}`
    return `<span ${this.attr}="${value}"></span>`
  }

  /** inside puts a mark just after the opening tag of html. */
  private inside(html: string, mark: string): string {
    return html.replace(/^<[a-z][^>]*>/i, (open) => open + mark)
  }

  override paragraph(token: Tokens.Paragraph): string {
    return this.inside(super.paragraph(token), this.mark((token as LineToken).ln))
  }

  override heading(token: Tokens.Heading): string {
    const start = (token as LineToken).ln
    const end = start === undefined ? undefined : lastLine(token, start)
    return this.inside(super.heading(token), this.mark(start, end))
  }

  override listitem(token: Tokens.ListItem): string {
    return this.inside(super.listitem(token), this.mark((token as LineToken).ln))
  }

  override br(token: Tokens.Br): string {
    const ln = (token as LineToken).ln
    return super.br(token) + this.mark(ln === undefined ? undefined : ln + countChar(token.raw ?? '', '\n'))
  }

  /** text marks each line of itself. A block level one (the text of a tight
   * list item) also marks the line it starts on; an inline one starts in
   * the middle of a line its parent has already marked, and only marks the
   * lines after the newlines it holds. */
  override text(token: Tokens.Text | Tokens.Escape): string {
    const html = super.text(token)
    const ln = (token as LineToken).ln
    if (ln === undefined) return html
    if ('tokens' in token && token.tokens) return this.mark(ln) + html
    let line = ln
    return html.replace(/\n/g, () => `\n${this.mark(++line)}`)
  }

  override code(token: Tokens.Code): string {
    const ln = (token as LineToken).ln
    if (ln === undefined) return super.code(token)
    const lang = (token.lang ?? '').match(/^\S*/)?.[0] ?? ''
    // The first line of a fenced block is the fence; an indented one has no
    // fence to skip.
    const first = token.codeBlockStyle === 'indented' ? ln : ln + 1
    const lines = token.text.replace(/\n$/, '').split('\n')
    const body = lines.map((l, i) => this.mark(first + i) + escapeCode(l)).join('\n') + '\n'
    const open = lang ? `<pre><code class="language-${escapeCode(lang)}">` : '<pre><code>'
    return `${open}${body}</code></pre>\n`
  }

  /** table puts a mark in the first cell of every row: the header is the
   * line the table starts on, the delimiter row is the next, and the body
   * rows follow. */
  override table(token: Tokens.Table): string {
    const ln = (token as LineToken).ln
    const row = (cells: Tokens.TableCell[], line: number | undefined): string => {
      const html = cells.map((cell) => this.tablecell(cell))
      if (html.length > 0) html[0] = this.inside(html[0], this.mark(line))
      return this.tablerow({ text: html.join('') })
    }
    const head = row(token.header, ln)
    let body = token.rows.map((cells, i) => row(cells, ln === undefined ? undefined : ln + 2 + i)).join('')
    if (body) body = `<tbody>${body}</tbody>`
    return `<table>\n<thead>\n${head}</thead>\n${body}</table>\n`
  }
}

function escapeCode(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] ?? c)
}

/**
 * renderBlocks renders each top level block of body on its own and hands the
 * result to sanitize before wrapping it in an element carrying the lines the
 * block covers, as `data-ln="start-end"`.
 *
 * markAttr is the name of the attribute the finer marks inside a block are
 * written with. It is not data-ln: sanitizing refuses data attributes, which
 * is what keeps a data-ln written by the diff itself from lying about the
 * lines it covers, so the marks go in under a name the caller makes
 * unguessable, lets sanitize keep, and renames afterwards (see markdown.ts).
 * The wrapper goes on after sanitize for the same reason it always did.
 *
 * startLine is the line body itself starts on.
 */
export function renderBlocks(
  body: string,
  startLine: number,
  markAttr: string,
  sanitize: (html: string) => string,
): string {
  const tokens = Lexer.lex(body, MARKED_OPTIONS)
  annotate(tokens, startLine)
  const renderer = new MarkRenderer(markAttr)
  const parts: string[] = []
  for (const token of tokens as LineToken[]) {
    const start = token.ln ?? startLine
    const end = lastLine(token, start)
    const html = Parser.parse([token], { ...MARKED_OPTIONS, renderer })
    const clean = sanitize(typeof html === 'string' ? html : '')
    // Link definitions and the blank lines between blocks are tokens too,
    // and they render to nothing worth wrapping.
    if (clean.trim() === '') continue
    parts.push(`<div data-ln="${start}-${end}">${clean}</div>`)
  }
  return parts.join('')
}
