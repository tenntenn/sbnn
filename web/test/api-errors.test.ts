import assert from 'node:assert/strict'
import { afterEach, describe, it } from 'node:test'
import { deleteGroup, getFileContent, getPreview, getPrompt, getStatus } from '../src/api'

/**
 * The API answers a refusal with `{"error": "..."}` and a status. request()
 * used to throw the whole body as the message, and PreviewFileSection renders
 * that message as-is, so the preview pane showed the wire format:
 *
 *   { "error": "no preview for this file: doc.md was deleted" }
 *
 * braces, quoted key and all, where a sentence belongs (#327). Measured
 * against a real server: GET .../content and .../image on a deleted .md both
 * answer exactly that body with HTTP 400.
 */

const real = globalThis.fetch

// answers stands the network in for one call. The body is given as text, the
// way fetch hands it over, so a test can send something that is not JSON.
function answers(status: number, body: string, contentType = 'application/json') {
  globalThis.fetch = (async () =>
    new Response(body, { status, headers: { 'Content-Type': contentType } })) as typeof fetch
}

async function messageOf(call: () => Promise<unknown>): Promise<string> {
  try {
    await call()
  } catch (err) {
    return err instanceof Error ? err.message : String(err)
  }
  assert.fail('the call resolved, but the response was a refusal')
}

afterEach(() => {
  globalThis.fetch = real
})

describe('a refused API response', () => {
  const refusal = JSON.stringify({ error: 'no preview for this file: doc.md was deleted' }, null, 2)

  it('reaches the page as the sentence, not as the JSON envelope', async () => {
    answers(400, refusal)
    const message = await messageOf(() => getFileContent('default', 'd1', 'f1'))
    assert.equal(message, 'no preview for this file: doc.md was deleted')
    assert.doesNotMatch(message, /[{}]/, `the JSON envelope reached the page: ${message}`)
    assert.doesNotMatch(message, /"error"/, `the wire format's key reached the page: ${message}`)
  })

  it('carries the status, so a caller can tell a refusal from a crash', async () => {
    answers(424, JSON.stringify({ error: 'mo is not installed: exec: "mo": not found' }))
    try {
      await getPreview('default', 'd1', 'f1')
      assert.fail('the call resolved')
    } catch (err) {
      assert.equal((err as { status?: number }).status, 424)
    }
  })

  // The endpoints that do not go through request() answered the same way and
  // had the same hole.
  it('is parsed for the endpoints that read the body themselves', async () => {
    answers(409, JSON.stringify({ error: 'the default group cannot be deleted' }))
    assert.equal(await messageOf(() => deleteGroup('default')), 'the default group cannot be deleted')
    answers(500, JSON.stringify({ error: 'building the prompt' }))
    assert.equal(await messageOf(() => getPrompt('default')), 'building the prompt')
  })

  it('falls back to the raw text when the body is not that shape', async () => {
    answers(502, '<html>Bad Gateway</html>', 'text/html')
    assert.equal(await messageOf(() => getStatus()), '<html>Bad Gateway</html>')
  })

  // statusText is not a fallback on its own: HTTP/2 carries no reason phrase,
  // so fetch reports it as '' and the reader is shown an empty error.
  it('falls back to the status when there is no body and no reason phrase', async () => {
    answers(503, '')
    const message = await messageOf(() => getStatus())
    assert.ok(message.length > 0, 'an empty body left the reader with an empty message')
    assert.match(message, /503/, `the message names nothing the reader can act on: ${message}`)
  })

  // JSON that is not the envelope, and an envelope whose error is not a
  // string, are both "not that shape" - showing "[object Object]" would be a
  // second way of putting the wire format on the page.
  it('does not invent a message out of unrelated JSON', async () => {
    answers(400, JSON.stringify({ detail: 'something else' }))
    assert.equal(await messageOf(() => getStatus()), '{"detail":"something else"}')
    answers(400, JSON.stringify({ error: { code: 7 } }))
    assert.equal(await messageOf(() => getStatus()), '{"error":{"code":7}}')
  })
})
