import test from 'node:test'
import assert from 'node:assert/strict'

import type { StaticPayload } from '../src/client'

/**
 * An exported page keeps its comments in the browser, so replying there is
 * the static client's job: the same rules the server applies (#423), applied
 * to the list the page holds. The payload has to be in place before
 * client.ts is imported, because that is when it decides there is no server.
 */
const payload: StaticPayload = {
  version: 2,
  generatedAt: '2026-03-04T05:06:07Z',
  group: 'api',
  diffs: [{ id: 'd1', title: 'diff 1' } as never],
  comments: [
    {
      id: 'c1',
      group: 'api',
      diffId: 'd1',
      fileId: 'f1',
      path: 'handler.go',
      side: 'new',
      startLine: 12,
      endLine: 14,
      body: 'Was the retry deliberate?',
      question: true,
      snippet: 'for i := 0; i < 3; i++ {',
      resolved: false,
      createdAt: '2026-03-04T05:00:00Z',
      updatedAt: '2026-03-04T05:00:00Z',
    },
  ],
  previews: {},
  images: {},
}

// A page whose storage works, so that what is written is what is read.
const stored = new Map<string, string>()
;(globalThis as unknown as { window: unknown }).window = {
  __SBNN_DATA__: payload,
  localStorage: {
    getItem: (k: string) => stored.get(k) ?? null,
    setItem: (k: string, v: string) => void stored.set(k, v),
  },
}

const { client } = await import('../src/client')

test('a reply on an exported page lands in the thread, and the prompt carries it', async () => {
  await client.addReply('api', 'c1', 'Yes: the first connection drops.')
  const { comments } = await client.loadComments('api')
  assert.equal(comments.length, 2)
  const reply = comments[1]
  assert.equal(reply.replyTo, 'c1')
  assert.equal(reply.path, 'handler.go')
  assert.equal(reply.startLine, 12)
  assert.equal(reply.endLine, 14)
  assert.equal(reply.question, false)

  const prompt = await client.prompt('api')
  // The reviewer answering their own question is more of the question, so
  // it is still waiting for an answer.
  assert.match(prompt, /1 comment\(s\) to address, 1 of them a question\./)
  assert.match(prompt, /This one is a question: answer it\./)
  assert.match(prompt, /### Reply from the reviewer\n\nYes: the first connection drops\./)
})

test('a reply to a reply joins the first comment', async () => {
  const { comments } = await client.loadComments('api')
  await client.addReply('api', comments[1].id, 'And it is bounded.')
  const after = (await client.loadComments('api')).comments
  assert.equal(after[2].replyTo, 'c1')
})

test('resolving from a reply resolves the thread, and deleting the comment deletes it', async () => {
  const { comments } = await client.loadComments('api')
  await client.updateComment('api', comments[1].id, { resolved: true })
  const resolved = (await client.loadComments('api')).comments
  assert.ok(resolved.every((c) => c.resolved))

  await client.deleteComment('api', 'c1')
  assert.equal((await client.loadComments('api')).comments.length, 0)
})

test('a reply to nothing is refused', async () => {
  await assert.rejects(client.addReply('api', 'c99', 'hello?'))
})
