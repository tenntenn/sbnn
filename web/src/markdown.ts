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
export function renderMarkdown(source: string, assets?: PreviewAssets): string {
  const { frontmatter, body, bodyStartLine } = splitFrontmatter(source)
  const rendered = resolvePreviewImages(renderBody(body, bodyStartLine), assets)
  if (frontmatter === '') return rendered
  return `<pre class="frontmatter">${escapeHTML(frontmatter)}</pre>` + rendered
}

/**
 * PreviewAsset is one image of a previewed file, as sbnn resolved it. The
 * live page gets a URL of the server; an exported page gets a data URL. When
 * there is no url there is a status saying why, and both pages say the same
 * thing because internal/asset decided it for both.
 */
export interface PreviewAsset {
  url?: string
  path?: string
  status?: string
  size?: number
}

/** PreviewAssets is those, keyed by the src the Markdown wrote. */
export type PreviewAssets = Record<string, PreviewAsset>

/**
 * resolvePreviewImages points the <img> tags of a rendered preview at
 * something that exists.
 *
 * A relative src is written against the file's own directory in the tree, and
 * the review page is served from neither that directory nor, in an exported
 * page, from anywhere at all: left alone it resolves to the page's own origin
 * and comes back as the review page again. So every relative src is looked up
 * in what sbnn resolved for this file, and an image that is not there - too
 * large to carry, outside the directory the diff came from, not in the tree -
 * is drawn as a plate naming the file instead of as a picture of nothing.
 *
 * An absolute or remote src is left exactly as written: the browser resolves
 * those the same way in a live page and an exported one, so there is nothing
 * here to decide about them.
 *
 * It runs on the output of sanitize, not on the Markdown: raw HTML in a
 * document carries <img> too, and by this point every one of them - written
 * as Markdown or as HTML - is one tag with quoted, escaped attributes. That
 * also makes it a plain string pass, which is what lets it be tested where
 * there is no DOM to render Markdown in.
 */
export function resolvePreviewImages(html: string, assets?: PreviewAssets): string {
  if (!html.includes('<img')) return html
  return html.replace(/<img\b[^>]*>/gi, (tag) => {
    const src = attrOf(tag, 'src')
    if (src === null) return tag
    const entry = lookupAsset(assets, src)
    if (entry?.url) return replaceSrc(tag, entry.url)
    if (entry) return placeholder(entry.path ?? src, entry.status, entry.size)
    if (isAbsoluteURL(src)) return tag
    return placeholder(src, 'unknown')
  })
}

/** lookupAsset finds the entry for a src. marked percent-encodes a
 * destination it had to read out of <angle brackets>, so a src that did not
 * match is tried again decoded. */
function lookupAsset(assets: PreviewAssets | undefined, src: string): PreviewAsset | undefined {
  if (!assets) return undefined
  if (Object.hasOwn(assets, src)) return assets[src]
  try {
    const decoded = decodeURI(src)
    if (Object.hasOwn(assets, decoded)) return assets[decoded]
  } catch {
    // A src that is not valid percent-encoding is not one sbnn wrote a key
    // for either.
  }
  return undefined
}

function isAbsoluteURL(src: string): boolean {
  return /^[a-z][a-z0-9+.-]*:/i.test(src) || src.startsWith('//')
}

/** attrOf reads an attribute out of a sanitised tag, where every value is
 * quoted and every quote inside one is escaped. */
function attrOf(tag: string, name: string): string | null {
  const m = new RegExp(`\\s${name}\\s*=\\s*("[^"]*"|'[^']*'|[^\\s>]+)`, 'i').exec(tag)
  if (!m) return null
  const raw = m[1]
  const value =
    (raw.startsWith('"') && raw.endsWith('"')) || (raw.startsWith("'") && raw.endsWith("'"))
      ? raw.slice(1, -1)
      : raw
  return unescapeHTML(value)
}

function replaceSrc(tag: string, url: string): string {
  return tag.replace(/\ssrc\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/i, ` src="${escapeHTML(url)}"`)
}

/** assetTrouble says, in the reader's words, why there is no picture. The
 * wording is the same on a live page and an exported one because the status
 * behind it is decided in one place for both.
 *
 * Exported because an image that *is* the diff gets the same treatment and
 * the same sentence (#323): it is drawn by PreviewFileSection rather than by
 * a pass over rendered Markdown, so it cannot go through placeholder() below,
 * but a reader should not be able to tell which of the two they are looking
 * at from the words. */
export function assetTrouble(status: string | undefined, size: number | undefined): string {
  switch (status) {
    case 'too-large':
      return `too large to show here (${formatBytes(size)})`
    case 'over-budget':
      return `left out to keep this page loadable (${formatBytes(size)})`
    case 'outside':
      return 'outside the directory this diff was sent from'
    case 'missing':
      return 'not in the working tree'
    case 'unsupported':
      return 'not an image this page can show'
    default:
      return 'not part of this review'
  }
}

export function formatBytes(n: number | undefined): string {
  if (n === undefined || n <= 0) return 'unknown size'
  const mb = n / (1024 * 1024)
  if (mb >= 1) return `${mb.toFixed(1)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}

/**
 * placeholder is what stands where a picture cannot be drawn. It names the
 * file, so that a reader who wants to see it knows what to open, and says
 * why - a broken-image icon says neither.
 */
function placeholder(path: string, status?: string, size?: number): string {
  const why = assetTrouble(status, size)
  const label = `${path} - ${why}`
  return (
    `<span class="preview-asset-missing" role="img" aria-label="${escapeHTML(label)}"` +
    ` title="${escapeHTML(label)}">` +
    `<span class="preview-asset-name">${escapeHTML(path)}</span>` +
    `<span class="preview-asset-why">${escapeHTML(why)}</span>` +
    `</span>`
  )
}

/** unescapeHTML undoes what the sanitiser's serialiser did to an attribute
 * value, so that the src is compared as the Markdown wrote it. */
function unescapeHTML(s: string): string {
  return s.replace(
    /&(amp|lt|gt|quot|#39|apos);/g,
    (_, e: string) =>
      ({ amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'", apos: "'" })[e] ?? _,
  )
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

/**
 * PreviewLinkTargets maps a path in the review to the fragment that
 * addresses that file's own section on this page. It is what lets a link
 * between two files of the same diff be followed without leaving sbnn.
 */
export type PreviewLinkTargets = Record<string, string>

/**
 * resolvePreviewLinks points the <a> tags of a rendered preview at something
 * that exists, or stops them pretending to.
 *
 * A relative href in a Markdown file is written against the file's own
 * directory in the tree. The review page is served from the server root, so
 * left alone `./notes.md` resolved to `/notes.md`, which the SPA catch-all
 * answered with the review page again: the reader landed on a second copy of
 * sbnn that read `notes.md` as a group name, found no such group, and said
 * "Waiting for a diff". The link appeared to work and went nowhere.
 *
 * So every relative href is resolved against the previewed file's directory
 * and then looked up in what this page is showing. A file the review carries
 * becomes a link to that file's own section, followed in place. Anything else
 * is not a link at all: it is drawn as the path it resolved to and the plain
 * statement that it is not part of this review, which is the honest answer
 * when there is nothing here to open.
 *
 * A bare fragment is left exactly as written - it addresses this document -
 * and so is an absolute or remote href, which the browser resolves the same
 * way in a live page and an exported one.
 *
 * Like resolvePreviewImages this runs on the sanitiser's output as a string,
 * where every tag has quoted, escaped attributes: raw HTML in a document
 * carries <a> too, and one pass catches both spellings. It is also what lets
 * it be tested where there is no DOM to render Markdown in.
 */
export function resolvePreviewLinks(
  html: string,
  path: string,
  targets?: PreviewLinkTargets,
): string {
  if (!html.includes('<a')) return html
  return html.replace(/<a\b([^>]*)>([\s\S]*?)<\/a>/gi, (tag, attrs: string, text: string) => {
    const href = attrOf(`<a${attrs}>`, 'href')
    if (href === null || href === '') return tag
    if (isAbsoluteURL(href) || href.startsWith('#')) return tag
    const resolved = resolveAgainst(path, href)
    if (resolved === null) return tag
    const fragment = targets?.[resolved]
    if (fragment !== undefined) {
      return (
        `<a href="${escapeHTML(fragment)}" class="preview-link-file"` +
        ` title="${escapeHTML(resolved)}">${text}</a>`
      )
    }
    return outsideReview(resolved, text)
  })
}

/** outsideReview is what stands where a link cannot be followed. It names
 * the path the href resolved to, so a reader who wants the file knows what
 * to open, and says why it is not one click away - which a link that quietly
 * goes nowhere says neither. */
function outsideReview(resolved: string, text: string): string {
  const label = `${resolved} - not part of this review`
  return (
    `<span class="preview-link-outside" title="${escapeHTML(label)}"` +
    ` aria-label="${escapeHTML(label)}">` +
    `<span class="preview-link-text">${text}</span>` +
    `<span class="preview-link-why">not in this review</span>` +
    `</span>`
  )
}

/**
 * resolveAgainst turns an href written inside `path` into a path from the
 * root of the tree the diff came from, dropping any query or fragment: what
 * this page can look up is a file, not a place inside one.
 *
 * A leading slash is read as that same root rather than as the web server's,
 * which is how a repository writes a link to a file at its top level. An
 * href that climbs above the root has nothing to resolve to and is left
 * alone, since rewriting it could only guess.
 */
function resolveAgainst(path: string, href: string): string | null {
  const cut = href.search(/[?#]/)
  const target = cut === -1 ? href : href.slice(0, cut)
  if (target === '') return null
  let decoded = target
  try {
    decoded = decodeURI(target)
  } catch {
    // Not valid percent-encoding; take it as written, the same way
    // lookupAsset does.
  }
  const base = decoded.startsWith('/') ? [] : path.split('/').slice(0, -1)
  const parts = decoded.replace(/^\//, '').split('/')
  const out = [...base]
  for (const part of parts) {
    if (part === '' || part === '.') continue
    if (part === '..') {
      if (out.length === 0) return null
      out.pop()
      continue
    }
    out.push(part)
  }
  return out.length === 0 ? null : out.join('/')
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
