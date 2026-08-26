import test from 'node:test'
import assert from 'node:assert/strict'

import { resolvePreviewImages, type PreviewAssets } from '../src/markdown'

/**
 * A relative image in a preview used to resolve against the review page's own
 * origin, where there is no such file: the live page answered it with the
 * review page and an exported page had nothing behind it at all. sbnn now
 * resolves each one against the directory the diff was sent from and hands
 * the page a URL - the server's on a live page, a data URL in an exported one
 * - so both draw the same document the same way.
 *
 * The rendering itself cannot be driven here: DOMPurify configures itself
 * against a real DOM the moment it is imported and this process has none (see
 * test/resolve-ts.mjs). resolvePreviewImages runs on the sanitiser's output
 * as a string, which is exactly the part these tests need.
 */

const src = (html: string) => /src="([^"]*)"/.exec(html)?.[1]

test('a relative image is pointed at what sbnn resolved for it', () => {
  const assets: PreviewAssets = {
    'diagram.png': { url: '/_/api/groups/g/diffs/d1/files/f2/asset?path=docs%2Fdiagram.png', status: 'ok' },
  }
  const out = resolvePreviewImages('<p><img src="diagram.png" alt="a"></p>', assets)
  assert.equal(src(out), '/_/api/groups/g/diffs/d1/files/f2/asset?path=docs%2Fdiagram.png')
  assert.match(out, /alt="a"/)
})

test('an exported page draws the same image out of the data URL frozen into it', () => {
  const dataURL = 'data:image/png;base64,iVBORw0KGgo='
  const live = resolvePreviewImages(
    '<p><img src="diagram.png" alt="a"></p>',
    { 'diagram.png': { url: '/_/api/groups/g/diffs/d1/files/f2/asset?path=diagram.png', status: 'ok' } },
  )
  const exported = resolvePreviewImages('<p><img src="diagram.png" alt="a"></p>', {
    'diagram.png': { url: dataURL, status: 'ok' },
  })
  assert.equal(src(exported), dataURL)
  // Same document, same tag, same alt: only where the bytes come from differs.
  assert.equal(live.replace(/src="[^"]*"/, ''), exported.replace(/src="[^"]*"/, ''))
})

test('an image too large to carry becomes a placeholder that names the file', () => {
  const out = resolvePreviewImages('<p><img src="huge.png" alt="a"></p>', {
    'huge.png': { path: 'docs/huge.png', status: 'too-large', size: 4 * 1024 * 1024 },
  })
  assert.doesNotMatch(out, /<img/)
  assert.match(out, /docs\/huge\.png/)
  assert.match(out, /too large/)
  assert.match(out, /4\.0 MB/)
})

test('the reasons a picture is missing are told apart', () => {
  const cases: Array<[string, RegExp]> = [
    ['outside', /outside the directory/],
    ['missing', /not in the working tree/],
    ['unsupported', /not an image/],
    ['over-budget', /keep this page loadable/],
  ]
  for (const [status, want] of cases) {
    const out = resolvePreviewImages('<img src="x.png">', { 'x.png': { path: 'x.png', status } })
    assert.match(out, want, `status ${status}`)
    assert.match(out, /x\.png/, `status ${status} should still name the file`)
  }
})

test('a relative image sbnn resolved nothing for is not left pointing at the page itself', () => {
  const out = resolvePreviewImages('<p><img src="./other.png"></p>', {})
  assert.doesNotMatch(out, /<img/)
  assert.match(out, /\.\/other\.png/)
})

test('a remote image is left exactly as it was written', () => {
  const html = '<p><img src="https://example.com/x.png" alt="remote"></p>'
  assert.equal(resolvePreviewImages(html, {}), html)
  assert.equal(resolvePreviewImages('<img src="//cdn.example/x.png">', {}), '<img src="//cdn.example/x.png">')
  const inline = '<img src="data:image/png;base64,AAAA">'
  assert.equal(resolvePreviewImages(inline, {}), inline)
})

test('a destination the renderer percent-encoded still finds its entry', () => {
  const out = resolvePreviewImages('<img src="spaced%20name.png">', {
    'spaced name.png': { url: 'data:image/png;base64,AAAA', status: 'ok' },
  })
  assert.equal(src(out), 'data:image/png;base64,AAAA')
})

test('a key that is a property of every object is not mistaken for an asset', () => {
  const out = resolvePreviewImages('<img src="constructor">', {})
  assert.match(out, /not part of this review/)
})

test('nothing but the img tags is touched', () => {
  const html = '<div data-ln="1-3"><p>text with a &lt;img&gt; in it</p><img src="a.png"></div>'
  const out = resolvePreviewImages(html, { 'a.png': { url: 'u', status: 'ok' } })
  assert.match(out, /text with a &lt;img&gt; in it/)
  assert.match(out, /data-ln="1-3"/)
})
