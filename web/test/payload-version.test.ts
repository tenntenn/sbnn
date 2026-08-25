import test from 'node:test'
import assert from 'node:assert/strict'

// client.ts decides on import whether the page has a server behind it, so a
// payload has to be in place first; these tests are about the version field
// alone, so it is otherwise as empty as a payload can be.
;(globalThis as unknown as { window: unknown }).window = {
  __SBNN_DATA__: {
    version: 1,
    generatedAt: '2026-03-04T05:06:07Z',
    group: 'api',
    diffs: [],
    comments: [],
    previews: {},
    images: {},
  },
}

const { payloadSbnnVersion } = await import('../src/client')

/**
 * The field was called saVersion before the tool was renamed. Pages already
 * written carry the old name and have to stay readable, and pages written
 * from now on carry the new one, so a reader has to accept both - and prefer
 * the current name when a page somehow says both.
 */
test('payloadSbnnVersion reads either name, current one first', () => {
  assert.equal(payloadSbnnVersion({ sbnnVersion: '1.2.3' }), '1.2.3')
  assert.equal(payloadSbnnVersion({ saVersion: '0.9.0' }), '0.9.0')
  assert.equal(payloadSbnnVersion({ sbnnVersion: '1.2.3', saVersion: '0.9.0' }), '1.2.3')
})

test('a page that says nothing about it reports nothing', () => {
  assert.equal(payloadSbnnVersion({}), undefined)
  assert.equal(payloadSbnnVersion({ sbnnVersion: '', saVersion: '' }), undefined)
})
