import DOMPurify from 'dompurify'
import { marked } from 'marked'

const MARKED_OPTIONS = { async: false, gfm: true, breaks: false } as const

/**
 * renderMarkdown renders the Markdown of a preview.
 *
 * The live app can leave Markdown to mo instead, but sbnn's own renderer is
 * also what an exported page uses, since that has no server behind it. The
 * result is sanitised: the Markdown comes from a diff, and a diff is not
 * trusted input.
 *
 * Every top level block is wrapped in an element carrying the source lines it
 * came from, so a selection in the preview can be turned back into the line
 * numbers a comment anchors to.
 */
export function renderMarkdown(source: string): string {
  const { frontmatter, body, bodyStartLine } = splitFrontmatter(source)
  const rendered = renderBody(body, bodyStartLine)
  if (frontmatter === '') return rendered
  return `<pre class="frontmatter">${escapeHTML(frontmatter)}</pre>` + rendered
}

/**
 * renderBody renders each top level block on its own, so the exact source
 * lines it consumed can be attached to it in data-ln before the blocks are
 * joined back into one string. startLine is the line body itself starts on.
 *
 * The wrapper goes on after the block has been sanitised rather than before:
 * sanitising still refuses data attributes, so a data-ln written by the diff
 * itself is stripped and cannot lie about which lines a block covers.
 */
function renderBody(body: string, startLine: number): string {
  const parts: string[] = []
  let line = startLine
  for (const token of marked.lexer(body, MARKED_OPTIONS)) {
    const raw = token.raw ?? ''
    const newlines = countChar(raw, '\n')
    const start = line
    // A block whose raw text ends in a newline stops on the line before the
    // one that newline opens.
    const end = raw.endsWith('\n') ? start + newlines - 1 : start + newlines
    line = start + newlines
    const html = marked.parser([token], MARKED_OPTIONS)
    const clean = sanitize(typeof html === 'string' ? html : '')
    // Link definitions and the blank lines between blocks are tokens too,
    // and they render to nothing worth wrapping.
    if (clean.trim() === '') continue
    parts.push(`<div data-ln="${start}-${Math.max(end, start)}">${clean}</div>`)
  }
  return parts.join('')
}

function countChar(s: string, ch: string): number {
  let n = 0
  for (let i = 0; i < s.length; i++) if (s[i] === ch) n++
  return n
}

/** splitFrontmatter peels off the YAML metadata block mo shows separately.
 * bodyStartLine is the line body starts on in the original file, which is 1
 * unless a frontmatter block pushed it down. */
function splitFrontmatter(source: string): {
  frontmatter: string
  body: string
  bodyStartLine: number
} {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(source)
  if (!match) return { frontmatter: '', body: source, bodyStartLine: 1 }
  return {
    frontmatter: match[1],
    body: source.slice(match[0].length),
    bodyStartLine: 1 + countChar(match[0], '\n'),
  }
}

export function escapeHTML(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c] ?? c)
}

/**
 * isExternalHref reports whether a link points somewhere that is genuinely
 * another page, and so is worth opening in a tab of its own.
 *
 * Only an absolute http(s) URL is. A relative href and a bare fragment both
 * resolve against the review page's own URL, which is the server root - not
 * against the directory the previewed file lives in - so opening one in a new
 * tab lands the reader on a second copy of sbnn (or, now that the server
 * answers asset-looking paths with 404, on a blank error tab) rather than on
 * the thing they clicked. A mailto: or tel: href is handed to another
 * application entirely and gains nothing from target either.
 */
function isExternalHref(href: string): boolean {
  return /^https?:\/\//i.test(href) || href.startsWith('//')
}

// Links are the only elements that gain an attribute here, so the hook only
// has to look at them. target is removed rather than merely not set: the
// Markdown comes from a diff and may carry its own <a target="_blank">.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName !== 'A' || !node.hasAttribute('href')) return
  if (isExternalHref(node.getAttribute('href') ?? '')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noreferrer noopener')
    return
  }
  node.removeAttribute('target')
})

/**
 * sanitize strips everything executable out of rendered Markdown.
 *
 * This used to be a hand-written pass over the parsed DOM, which cannot be
 * made correct: DOMParser runs with scripting off, where <noscript> holds
 * markup, and the page that renders the result runs with scripting on,
 * where it holds text. An attribute closing the tag inside its own value -
 * <noscript><p title="</noscript><img src=x onerror=...>"> - therefore
 * passed the check as an attribute and came back as an element. Turning
 * markup into a DOM safely is a job with a maintained answer, so sbnn uses it
 * rather than keeping its own.
 *
 * Exported so the notebook renderer, which sanitises the same way, does not
 * need a second DOMPurify configuration to keep in sync with this one.
 */
export function sanitize(html: string): string {
  return DOMPurify.sanitize(html, {
    // An exported page is one file with the diff frozen into it; nothing in
    // a preview should be reaching for anything else.
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'link', 'meta', 'base'],
    ALLOW_DATA_ATTR: false,
  })
}
