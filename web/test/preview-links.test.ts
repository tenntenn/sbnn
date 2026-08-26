import test from 'node:test'
import assert from 'node:assert/strict'
import DOMPurify from 'dompurify'

import { resolvePreviewLinks, type PreviewLinkTargets } from '../src/markdown'

/**
 * A relative link in a preview used to resolve against the review page's own
 * origin: `./other.md` became `http://localhost:6399/other.md`, the SPA
 * catch-all answered it with 200 index.html, and the reader landed in a
 * second copy of sbnn that read `other.md` as a group name and said "Waiting
 * for a diff". Every link also carried target="_blank", so it happened in a
 * tab of its own - including for a bare `#fragment`, which addresses the very
 * document being read.
 *
 * Neither half can be driven through renderMarkdown here: DOMPurify
 * configures itself against a real DOM the moment it is imported and this
 * process has none (see test/resolve-ts.mjs). The two halves are reachable
 * without one - the target hook through the callback the stand-in kept, and
 * resolvePreviewLinks because it runs on the sanitiser's output as a string.
 */

/** attrHook is the afterSanitizeAttributes callback markdown.ts registered
 * when it was imported. */
const attrHook = (() => {
  const hooks = (DOMPurify as unknown as { hooks: Record<string, ((node: unknown) => void)[]> }).hooks
  const registered = hooks?.afterSanitizeAttributes
  assert.ok(registered?.length, 'markdown.ts registered no afterSanitizeAttributes hook')
  return registered[0]
})()

/** fakeAnchor is the little of a DOM node the hook touches. */
function fakeAnchor(href: string | null, attrs: Record<string, string> = {}) {
  const node = {
    tagName: 'A',
    attrs: { ...attrs, ...(href === null ? {} : { href }) } as Record<string, string>,
    hasAttribute(name: string) {
      return Object.hasOwn(this.attrs, name)
    },
    getAttribute(name: string) {
      return this.attrs[name] ?? null
    },
    setAttribute(name: string, value: string) {
      this.attrs[name] = value
    },
    removeAttribute(name: string) {
      delete this.attrs[name]
    },
  }
  return node
}

function targetOf(href: string | null, attrs: Record<string, string> = {}): string | null {
  const node = fakeAnchor(href, attrs)
  attrHook(node)
  return node.getAttribute('target')
}

test('an http link opens in a tab of its own', () => {
  assert.equal(targetOf('https://example.com/x'), '_blank')
  assert.equal(targetOf('http://example.com/x'), '_blank')
  assert.equal(targetOf('//example.com/x'), '_blank')
})

test('a relative href does not open a new tab (#89)', () => {
  assert.equal(targetOf('./notes.md'), null)
  assert.equal(targetOf('notes.md'), null)
  assert.equal(targetOf('../README.md'), null)
  assert.equal(targetOf('/docs/notes.md'), null)
})

test('a bare fragment does not open a new tab', () => {
  assert.equal(targetOf('#guide'), null)
})

test('a scheme handed to another application gains nothing from target', () => {
  assert.equal(targetOf('mailto:someone@example.com'), null)
  assert.equal(targetOf('tel:+81300000000'), null)
})

test('a target written by the diff itself is taken off a relative link', () => {
  assert.equal(targetOf('./notes.md', { target: '_blank' }), null)
})

test('an anchor with no href is left alone', () => {
  const node = fakeAnchor(null)
  attrHook(node)
  assert.equal(node.getAttribute('target'), null)
  assert.equal(node.getAttribute('rel'), null)
})

const targets: PreviewLinkTargets = {
  'docs/notes.md': '#d1:f2-abcd1234',
  'README.md': '#d1:f1-0badf00d',
}

const hrefOf = (html: string) => /href="([^"]*)"/.exec(html)?.[1] ?? null

test('a relative link to a file in the review leads to that file section', () => {
  const cases: [string, string, string][] = [
    ['docs/guide.md', './notes.md', '#d1:f2-abcd1234'],
    ['docs/guide.md', 'notes.md', '#d1:f2-abcd1234'],
    ['docs/guide.md', '../README.md', '#d1:f1-0badf00d'],
    ['docs/guide.md', '/README.md', '#d1:f1-0badf00d'],
    ['README.md', 'docs/notes.md', '#d1:f2-abcd1234'],
    // A fragment or a query on the href addresses a place inside the file;
    // what this page can open is the file.
    ['docs/guide.md', './notes.md#top', '#d1:f2-abcd1234'],
    // marked percent-encodes a destination it read out of angle brackets.
    ['docs/guide.md', './notes%2Emd', '#d1:f2-abcd1234'],
  ]
  for (const [path, href, want] of cases) {
    const out = resolvePreviewLinks(`<p><a href="${href}">the other doc</a></p>`, path, targets)
    assert.equal(hrefOf(out), want, `${path} + ${href}`)
    assert.match(out, /the other doc/)
  }
})

test('a relative link the review does not carry is not a link', () => {
  const out = resolvePreviewLinks(
    '<p><a href="./missing.md">the other doc</a></p>',
    'docs/guide.md',
    targets,
  )
  assert.doesNotMatch(out, /<a\b/, 'it is still an anchor, so it still goes nowhere')
  assert.match(out, /preview-link-outside/)
  // It names the path it resolved to, not the href as written.
  assert.match(out, /docs\/missing\.md - not part of this review/)
  assert.match(out, /not in this review/)
  assert.match(out, /the other doc/)
})

test('with no targets at all every relative link says so', () => {
  const out = resolvePreviewLinks('<p><a href="./notes.md">x</a></p>', 'docs/guide.md')
  assert.match(out, /docs\/notes\.md - not part of this review/)
})

test('an absolute or remote href is left exactly as written', () => {
  for (const href of [
    'https://example.com/x',
    'http://example.com/x',
    '//example.com/x',
    'mailto:someone@example.com',
  ]) {
    const html = `<p><a href="${href}" target="_blank" rel="noreferrer noopener">x</a></p>`
    assert.equal(resolvePreviewLinks(html, 'docs/guide.md', targets), html)
  }
})

test('a bare fragment addresses this document and is left alone', () => {
  const html = '<p><a href="#guide">x</a></p>'
  assert.equal(resolvePreviewLinks(html, 'docs/guide.md', targets), html)
})

test('an href that climbs above the tree is left alone', () => {
  const html = '<p><a href="../../../etc/passwd">x</a></p>'
  assert.equal(resolvePreviewLinks(html, 'docs/guide.md', targets), html)
})

test('the newest round wins when two rounds touch the same file', () => {
  // What PreviewStack builds: the later round overwrites the earlier one.
  const both: PreviewLinkTargets = { 'docs/notes.md': '#d2:f1-abcd1234' }
  const out = resolvePreviewLinks('<p><a href="./notes.md">x</a></p>', 'docs/guide.md', both)
  assert.equal(hrefOf(out), '#d2:f1-abcd1234')
})

test('markup inside the link text survives both branches', () => {
  const inside = resolvePreviewLinks(
    '<p><a href="./notes.md">the <em>other</em> doc</a></p>',
    'docs/guide.md',
    targets,
  )
  assert.match(inside, /the <em>other<\/em> doc/)
  const outside = resolvePreviewLinks(
    '<p><a href="./gone.md">the <em>other</em> doc</a></p>',
    'docs/guide.md',
    targets,
  )
  assert.match(outside, /the <em>other<\/em> doc/)
})

test('html with no links is handed back untouched', () => {
  const html = '<p>nothing to see</p>'
  assert.equal(resolvePreviewLinks(html, 'docs/guide.md', targets), html)
})

/**
 * #339: the base a preview's links resolve against has to be the same kind of
 * path linkTargets is keyed by, and PreviewStack keys it by the diff-relative
 * path. PreviewFileSection used to pass `preview.path || filePath(file)`, and
 * preview.path is the file on disk the bytes were read from - absolute, and
 * empty only when the content was rebuilt from the diff. So the fallback was
 * reached exactly when the file was *missing* from the working tree, and the
 * ordinary case of `git diff | sbnn` inside the repository took the other
 * branch and resolved every relative href into a filesystem path no key can
 * match.
 *
 * These two are the same document and the same href, resolved against the two
 * kinds of base, and they show that one of them cannot work. The call site is
 * pinned in web/previewlinkbase_test.go, which can read both files at once.
 */
test('a diff-relative base finds the other file of the review (#339)', () => {
  const out = resolvePreviewLinks('<p><a href="./notes.md">the other doc</a></p>', 'docs/guide.md', targets)
  assert.equal(hrefOf(out), '#d1:f2-abcd1234')
})

test('an absolute worktree base can never match a target, and leaks the path (#339)', () => {
  const out = resolvePreviewLinks(
    '<p><a href="./notes.md">the other doc</a></p>',
    '/home/reviewer/checkout/docs/guide.md',
    targets,
  )
  assert.doesNotMatch(out, /<a\b/, 'it resolved to a target, so the two path spaces do overlap after all')
  assert.match(
    out,
    /home\/reviewer\/checkout\/docs\/notes\.md - not part of this review/,
    "the label carries the reviewer's absolute path",
  )
})
