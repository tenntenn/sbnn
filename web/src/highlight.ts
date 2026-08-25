/**
 * A small syntax highlighter for diff lines.
 *
 * It is deliberately not a parser. A diff is a set of disconnected fragments
 * - a hunk starts in the middle of a function and ends in the middle of
 * another - so there is no whole file to parse and no point pretending
 * otherwise. What a reader needs from colour while skimming forty lines is
 * only this: where a string ends, which word is a keyword, whether that tail
 * is a comment. Five kinds answer all three.
 *
 * The rules are per line and carry no state between lines, which is what
 * makes them safe to run one row at a time as the stack mounts it, and what
 * makes them wrong in exactly one known way: the body of a multi-line block
 * comment or template literal is coloured as if it were code, because the
 * line that opened it may not even be in the hunk. Being occasionally plain
 * is a smaller cost than colouring a whole file the wrong way from one
 * stray quote.
 *
 * Nothing is added to package.json for this: the binary embeds its own UI
 * and `go install` has to keep working without Node, so a highlighter that
 * is a dependency is a highlighter that costs every user bundle weight.
 */

/** TokenKind is the whole vocabulary: four things worth colouring, and
 * everything else. Growing this list is how highlighters turn into
 * parsers. */
export type TokenKind = 'comment' | 'string' | 'number' | 'keyword' | 'plain'

export interface Token {
  text: string
  kind: TokenKind
}

/** LanguageId is a family, not a file type: .ts, .tsx, .js and .jsx differ
 * in ways this highlighter cannot see. */
export type LanguageId = 'go' | 'js' | 'json' | 'py' | 'sh' | 'css' | 'md' | 'yaml'

/**
 * EXTENSIONS is the supported set, and it is short on purpose. An extension
 * that is not here gets no colour at all rather than a guess: a wrong colour
 * says something false about the code, and plain text never does.
 */
const EXTENSIONS: Record<string, LanguageId> = {
  go: 'go',
  ts: 'js',
  tsx: 'js',
  js: 'js',
  jsx: 'js',
  json: 'json',
  py: 'py',
  sh: 'sh',
  css: 'css',
  md: 'md',
  yaml: 'yaml',
  yml: 'yaml',
}

/** languageOf picks a language from a path's extension, or nothing. */
export function languageOf(path: string): LanguageId | null {
  const base = path.slice(path.lastIndexOf('/') + 1)
  const dot = base.lastIndexOf('.')
  if (dot <= 0) return null // no extension, or a dotfile with no suffix
  return EXTENSIONS[base.slice(dot + 1).toLowerCase()] ?? null
}

type Rule = [Exclude<TokenKind, 'plain'>, string]

// Order matters only where two rules can start at the same character: the
// scan always takes the leftmost match, and among equals the earlier rule.
// Comments and strings come first everywhere so that a keyword inside a
// string stays part of the string.

const NUMBER = String.raw`\b0[xXbBoO][\da-fA-F_]+\b|\b\d[\d_]*(?:\.[\d_]+)?(?:[eE][+-]?\d+)?\b`

const words = (list: string) => String.raw`\b(?:${list.trim().split(/\s+/).join('|')})\b`

const RULES: Record<LanguageId, Rule[]> = {
  go: [
    ['comment', String.raw`//.*`],
    ['comment', String.raw`/\*[\s\S]*?\*/`],
    ['comment', String.raw`/\*[\s\S]*`],
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['string', '`[^`]*`?'],
    ['string', String.raw`'(?:\\.|[^'\\])*'?`],
    ['number', NUMBER],
    [
      'keyword',
      words(`break case chan const continue default defer else fallthrough for func go goto
             if import interface map package range return select struct switch type var
             nil true false iota`),
    ],
  ],
  js: [
    ['comment', String.raw`//.*`],
    ['comment', String.raw`/\*[\s\S]*?\*/`],
    ['comment', String.raw`/\*[\s\S]*`],
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['string', String.raw`'(?:\\.|[^'\\])*'?`],
    ['string', '`[^`]*`?'],
    ['number', NUMBER],
    [
      'keyword',
      words(`abstract as async await break case catch class const continue debugger declare
             default delete do else enum export extends finally for from function get if
             implements import in instanceof interface keyof let namespace new of private
             protected public readonly return satisfies set static super switch this throw
             try type typeof var void while with yield null undefined true false`),
    ],
  ],
  json: [
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['number', String.raw`-?\b\d[\d_]*(?:\.\d+)?(?:[eE][+-]?\d+)?\b`],
    ['keyword', words('true false null')],
  ],
  py: [
    ['comment', String.raw`#.*`],
    ['string', String.raw`[rbufRBUF]{0,2}"""[\s\S]*?(?:"""|$)`],
    ['string', String.raw`[rbufRBUF]{0,2}'''[\s\S]*?(?:'''|$)`],
    ['string', String.raw`[rbufRBUF]{0,2}"(?:\\.|[^"\\])*"?`],
    ['string', String.raw`[rbufRBUF]{0,2}'(?:\\.|[^'\\])*'?`],
    ['number', NUMBER],
    [
      'keyword',
      words(`and as assert async await break class continue def del elif else except finally
             for from global if import in is lambda nonlocal not or pass raise return try
             while with yield None True False self match case`),
    ],
  ],
  sh: [
    ['comment', String.raw`#.*`],
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['string', String.raw`'[^']*'?`],
    ['number', NUMBER],
    [
      'keyword',
      words(`if then else elif fi for while until do done case esac in function return local
             export readonly declare set unset shift source exit trap echo cd`),
    ],
  ],
  css: [
    ['comment', String.raw`/\*[\s\S]*?\*/`],
    ['comment', String.raw`/\*[\s\S]*`],
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['string', String.raw`'(?:\\.|[^'\\])*'?`],
    // A colour literal is a value, and reads best as one.
    ['number', String.raw`#[\da-fA-F]{3,8}\b`],
    ['number', String.raw`\b\d[\d_]*(?:\.\d+)?(?:px|em|rem|ex|ch|vh|vw|vmin|vmax|%|s|ms|deg|fr)?\b`],
    ['keyword', String.raw`@[a-zA-Z-]+|!important`],
  ],
  md: [
    ['comment', String.raw`<!--[\s\S]*?(?:-->|$)`],
    ['comment', String.raw`^\s*>.*`],
    ['string', '`[^`]*`?'],
    ['keyword', String.raw`^\s*#{1,6}\s.*|^\s*(?:[-*+]|\d+\.)\s|^\s*` + '```' + String.raw`.*`],
    ['number', String.raw`\b\d[\d_]*(?:\.\d+)?\b`],
  ],
  yaml: [
    ['comment', String.raw`#.*`],
    ['string', String.raw`"(?:\\.|[^"\\])*"?`],
    ['string', String.raw`'[^']*'?`],
    // The key is the structure, so it is what the eye should catch first.
    ['keyword', String.raw`^\s*(?:-\s+)?[\w.$/-]+(?=\s*:(?:\s|$))`],
    ['keyword', words('true false null yes no on off')],
    ['number', NUMBER],
  ],
}

interface Compiled {
  pattern: RegExp
  /** kinds[i] is the kind of capture group i + 1. */
  kinds: Exclude<TokenKind, 'plain'>[]
}

const compiled = new Map<LanguageId, Compiled>()

function compile(id: LanguageId): Compiled {
  const cached = compiled.get(id)
  if (cached) return cached
  const rules = RULES[id]
  const built: Compiled = {
    pattern: new RegExp(rules.map(([, source]) => `(${source})`).join('|'), 'g'),
    kinds: rules.map(([kind]) => kind),
  }
  compiled.set(id, built)
  return built
}

/**
 * CACHE_LIMIT bounds the memo. Highlighting is per line and re-run on every
 * render of a row - a selection changes and the whole table renders again -
 * so the same string is asked for many times, but a review is finite and the
 * cache should not outgrow it.
 */
const CACHE_LIMIT = 8192
const cache = new Map<string, Token[]>()

const PLAIN = (text: string): Token[] => [{ text, kind: 'plain' }]

/**
 * highlightLine splits one line into tokens.
 *
 * A language it does not know, or no language at all, comes back as a single
 * plain token holding the line unchanged - which is what the diff looked
 * like before, and is the honest answer for a file this cannot read.
 */
export function highlightLine(content: string, language: LanguageId | null): Token[] {
  if (language === null || content === '') return PLAIN(content)

  const key = `${language}\u0000${content}`
  const hit = cache.get(key)
  if (hit) return hit

  const { pattern, kinds } = compile(language)
  const tokens: Token[] = []
  let at = 0
  pattern.lastIndex = 0
  for (let m = pattern.exec(content); m !== null; m = pattern.exec(content)) {
    // A rule that can match the empty string would spin here forever.
    if (m[0] === '') {
      pattern.lastIndex++
      continue
    }
    if (m.index > at) tokens.push({ text: content.slice(at, m.index), kind: 'plain' })
    let kind: Exclude<TokenKind, 'plain'> = kinds[0]
    for (let g = 0; g < kinds.length; g++) {
      if (m[g + 1] !== undefined) {
        kind = kinds[g]
        break
      }
    }
    tokens.push({ text: m[0], kind })
    at = m.index + m[0].length
  }
  if (at < content.length) tokens.push({ text: content.slice(at), kind: 'plain' })

  const result = tokens.length === 0 ? PLAIN(content) : tokens
  if (cache.size >= CACHE_LIMIT) cache.clear()
  cache.set(key, result)
  return result
}

/** tokenClass is the class a token is drawn with; plain text carries none
 * and keeps the colour it inherits. */
export function tokenClass(kind: TokenKind): string | undefined {
  return kind === 'plain' ? undefined : `tok-${kind}`
}

const STYLE_ID = 'sbnn-highlight'

/**
 * The colours live here rather than in styles.css because they belong to
 * this module: nothing else in the page has a token kind. Each one rides on
 * an existing custom property so a theme that redefines the palette
 * redefines these too, and each carries the current value of that property
 * as its fallback so a host that strips the stylesheet still gets readable
 * code rather than none.
 *
 * The pairings are the foreground tokens, not the diff-row tints: --add-bg
 * and --add-strong are documented in styles.css as colours that "tint whole
 * rows of code and have to stay behind text", and --add-strong as a string
 * colour is #abf2bc on white - about 1.4:1, which is not text. --ok-fg is
 * the same green chosen to clear 4.5:1 as a foreground, which is what a
 * string needs to be.
 */
const CSS = `
.tok-comment { color: var(--fg-muted, #5a626c); font-style: italic; }
.tok-string  { color: var(--ok-fg, #0a5b26); }
.tok-number  { color: var(--warn-fg, #7a4d00); }
.tok-keyword { color: var(--accent-fg, #0969da); }
`

/**
 * ensureHighlightStyles puts the token colours in the document once.
 *
 * Guarded on document because the same modules are imported where there is
 * none, and idempotent because every diff section asks.
 */
export function ensureHighlightStyles(): void {
  if (typeof document === 'undefined') return
  if (document.getElementById(STYLE_ID)) return
  const style = document.createElement('style')
  style.id = STYLE_ID
  style.textContent = CSS
  document.head.appendChild(style)
}
