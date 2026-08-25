import test from 'node:test'
import assert from 'node:assert/strict'

import type { StaticPayload } from '../src/client'

/**
 * What an exported page knows about its review, it knows from the payload
 * `sbnn export` froze into it: there is no server to ask. These tests stand
 * where the page cannot be driven - the three review fields were carried
 * into the payload and then read by nothing, so the page rendered a
 * submitted review as though it had never been submitted.
 *
 * The payload has to be in place before client.ts is imported, because that
 * is when the module decides whether the page has a server behind it.
 */
const payload: StaticPayload = {
  version: 2,
  saVersion: 'test',
  generatedAt: '2026-03-04T05:06:07Z',
  group: 'api',
  diffs: [{ id: 'd1', title: 'handler: return 404 for unknown ids' } as never],
  comments: [
    {
      id: 'c1',
      group: 'api',
      diffId: 'd1',
      fileId: 'f1',
      path: 'handler.go',
      side: 'new',
      startLine: 12,
      endLine: 12,
      body: '`ids` reads better than `xs` here.',
      snippet: 'for _, xs := range ids {',
      resolved: false,
      createdAt: '2026-03-04T05:00:00Z',
      updatedAt: '2026-03-04T05:00:00Z',
    },
  ],
  reviewedAt: '2026-03-04T05:06:07Z',
  reviewNote: 'Ship it. The naming nits below are optional.',
  reviewVerdict: 'approved',
  reviewed: true,
  previews: {},
  images: {},
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
;(globalThis as unknown as { window: unknown }).window = { __SBNN_DATA__: payload }

const { client } = await import('../src/client')

test('the page is served from the frozen payload', () => {
  assert.equal(client.isStatic, true)
})

test('load returns the review the payload carries', async () => {
  const data = await client.load('api')
  assert.equal(data.reviewedAt, '2026-03-04T05:06:07Z')
  assert.equal(data.reviewVerdict, 'approved')
  assert.equal(data.reviewed, true)
})

test('the copied prompt says the change was approved', async () => {
  const prompt = await client.prompt('api')
  for (const phrase of [
    'The reviewer approved the change',
    'The reviewer wrote:',
    'Ship it. The naming nits below are optional.',
    'came with the approval',
    'The change is approved, so none of this blocks it',
  ]) {
    assert.ok(prompt.includes(phrase), `the prompt does not say ${JSON.stringify(phrase)}:\n${prompt}`)
  }
  // And never the sentence that sends an agent off to change approved code.
  assert.ok(!prompt.includes('Address every comment above'), prompt)
})
