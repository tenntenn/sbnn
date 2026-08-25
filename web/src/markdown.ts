import DOMPurify from 'dompurify'
import { Marked, marked, type Tokens } from 'marked'

const MARKED_OPTIONS = { async: false, gfm: true, breaks: false } as const

/**
 * commentMarked renders a comment body, and draws every image as the link it
 * was written as instead of as an <img>.
 *
 * Rendering the image would make the browser fetch its URL, and see
 * renderComment for why a comment must never do that. A link is the closest
 * honest thing: the alt text stays readable, the URL is still one click away,
 * and nothing is fetched until the reader asks for it.
 */
const commentMarked = new Marked({
  ...MARKED_OPTIONS,
  renderer: {
    image({ href, title, text }: Tokens.Image): string {
      const label = text.trim() === '' ? href : text
      const attrs = title == null || title === '' ? '' : ` title="${escapeHTML(title)}"`
      return `<a href="${escapeHTML(href)}"${attrs}>${escapeHTML(label)}</a>`
    },
  },
})

/**
 * renderComment renders the Markdown of a comment body.
 *
 * It is not renderMarkdown: a comment is not a file. The review page is opened
 * locally and its comments are written by an agent, so an <img> in one would
 * have the page fetch a URL of the comment's choosing the moment it is opened,
 * telling that host the reader's address, the time and their browser. An
 * exported page carries the comments with it, so it would do the same on every
 * machine it reaches. Nothing about a review is worth announcing, so a comment
 * loads no subresource at all: images become links (see commentMarked), and
 * sanitizeComment drops the tags and attributes that fetch a URL on their own,
 * which is what raw HTML in a body would have to use.
 *
 * The preview pane keeps renderMarkdown, images included: there the Markdown
 * is the file under review and drawing it is the point. Whether the preview
 * should hold back remote images too is #305, and this does not decide it.
 */
export function renderComment(source: string): string {
  const html = commentMarked.parse(source)
  return sanitizeComment(typeof html === 'string' ? html : '')
}

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

// Links open away from the page, and they are the only elements that gain
// an attribute here, so the hook only has to look at them.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && node.hasAttribute('href')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noreferrer noopener')
  }
})

// An exported page is one file with the diff frozen into it; nothing in a
// preview should be reaching for anything else.
const FORBID_TAGS = ['style', 'form', 'input', 'button', 'link', 'meta', 'base']

// Elements the browser fetches a URL for as soon as it parses them, with no
// click and no script. Forbidding the element is what keeps the request from
// being made at all; a referrer policy or a CSP only changes what the request
// looks like once it has been sent.
const SUBRESOURCE_TAGS = [
  'img',
  'picture',
  'source',
  'video',
  'audio',
  'track',
  'iframe',
  'frame',
  'frameset',
  'embed',
  'object',
  'svg',
  'math',
  'image',
  'use',
]

// The same thing spelled as an attribute: url() in an inline style, and the
// URL-bearing attributes an element that is allowed here could still carry.
const SUBRESOURCE_ATTRS = ['style', 'src', 'srcset', 'poster', 'background', 'lowsrc', 'dynsrc']

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
    FORBID_TAGS,
    ALLOW_DATA_ATTR: false,
  })
}

/**
 * sanitizeComment sanitises a comment body: everything sanitize strips, plus
 * everything that would fetch a URL while the page is being drawn. See
 * renderComment for why a comment is held to that and a preview is not.
 */
export function sanitizeComment(html: string): string {
  return DOMPurify.sanitize(html, {
    FORBID_TAGS: [...FORBID_TAGS, ...SUBRESOURCE_TAGS],
    FORBID_ATTR: SUBRESOURCE_ATTRS,
    ALLOW_DATA_ATTR: false,
  })
}
