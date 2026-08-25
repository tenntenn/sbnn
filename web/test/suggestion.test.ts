/**
 * The browser and the server must read the same suggestions out of a comment
 * body: one that only the browser sees is offered in the page and refused by
 * the server, and one that only the server sees is applied without ever
 * having been shown. Both sides run internal/model/testdata/suggestions.json,
 * so a fix landing in one implementation alone fails here.
 *
 * Run with `pnpm test` - node strips the types, so this needs no dependency
 * the page does not already have.
 */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'

import { insertSuggestion, parseBody, suggestions, withSuggestion } from '../src/suggestion.ts'

interface Case {
  name: string
  body: string
  want: string[]
}

const corpusPath = fileURLToPath(
  new URL('../../internal/model/testdata/suggestions.json', import.meta.url),
)
const corpus = JSON.parse(readFileSync(corpusPath, 'utf8')) as { cases: Case[] }

assert.ok(corpus.cases.length > 0, 'the corpus is empty')

for (const c of corpus.cases) {
  test(`suggestions: ${c.name}`, () => {
    assert.deepEqual(suggestions(c.body), c.want)
  })
}

// withSuggestion has to survive its own output, and a body that already
// proposes a code block: the fence closing that nested block does not leave
// the body open, and "closing" it would swallow what is appended.
test('withSuggestion round trip', () => {
  const bodies: { body: string; have: string[] }[] = [
    { body: '', have: [] },
    { body: 'note', have: [] },
    { body: '```go\nfoo()', have: [] },
    { body: '```suggestion\nfirst\n```', have: ['first'] },
    { body: '```suggestion\n```go\nx\n```\n```', have: ['```go\nx\n```'] },
    { body: '~~~suggestion\n~~~go\nx\n~~~\n~~~', have: ['~~~go\nx\n~~~'] },
  ]
  for (const { body, have } of bodies) {
    for (const added of ['func parse() {', 'a\n\nb', '```go\nx\n```', '```']) {
      assert.deepEqual(suggestions(withSuggestion(body, added)), [...have, added], `${body} + ${added}`)
    }
  }
})

// The "Suggest a change" button drops its block wherever the cursor is, and
// the cursor is often inside a fenced block the writer has not closed yet -
// a block is typed top down. The block used to land inside that one, where
// it is quoted text: the page offered no change, the server stored none, and
// the button looked broken. What was typed still has to read as code.
test('insertSuggestion escapes a block the writer has not closed', () => {
  const typed = 'Try:\n\n```go\nfoo()\n'
  const { body } = insertSuggestion(typed, typed.length, 'bar()')

  assert.deepEqual(suggestions(body), ['bar()'])
  const prose = parseBody(body).filter((s) => s.kind === 'text')
  assert.ok(
    prose.some((s) => s.text.includes('```go') && s.text.includes('foo()')),
    `the code block the writer typed was lost:\n${body}`,
  )
})

// Inserted mid-body, the text below the cursor was inside that same open
// block, so it is opened again with the info string it had.
test('insertSuggestion reopens the block for what follows the cursor', () => {
  const typed = '```go\nfoo()\nbar()'
  const at = typed.indexOf('bar()')
  const { body } = insertSuggestion(typed, at, 'baz()')

  assert.deepEqual(suggestions(body), ['baz()'])
  const prose = parseBody(body)
    .filter((s) => s.kind === 'text')
    .map((s) => s.text)
    .join('\n')
  assert.ok(prose.includes('foo()'), `foo() was lost:\n${body}`)
  assert.ok(prose.includes('bar()'), `bar() was lost:\n${body}`)
  assert.equal((body.match(/```go/g) ?? []).length, 2, `the block was not reopened:\n${body}`)
})

// Nothing to repair: the ordinary cases stay as they were.
test('insertSuggestion leaves a closed body alone', () => {
  const cases: { body: string; at: number; text: string; want: string[] }[] = [
    { body: '', at: 0, text: 'x', want: ['x'] },
    { body: 'note', at: 4, text: 'x', want: ['x'] },
    { body: 'note', at: 0, text: 'x', want: ['x'] },
    { body: '```go\nfoo()\n```', at: 15, text: 'x', want: ['x'] },
    { body: '```suggestion\nfirst\n```', at: 23, text: 'x', want: ['first', 'x'] },
    // A suggestion that itself proposes a code block is closed, however
    // much its fences look like a block left open.
    { body: '```suggestion\n```go\nx\n```\n```', at: 29, text: 'y', want: ['```go\nx\n```', 'y'] },
  ]
  for (const c of cases) {
    const { body } = insertSuggestion(c.body, c.at, c.text)
    assert.deepEqual(suggestions(body), c.want, `${JSON.stringify(c.body)} at ${c.at}`)
  }
})

// The caller selects the replacement text it just wrote, so the offsets it
// is handed have to point at it.
test('insertSuggestion points at the text it wrote', () => {
  for (const typed of ['', 'note', 'Try:\n\n```go\nfoo()\n', '```suggestion\nfirst\n```']) {
    const { body, block, blockAt } = insertSuggestion(typed, typed.length, 'bar()')
    assert.equal(body.slice(blockAt, blockAt + block.length), block, `block not at blockAt for ${typed}`)
    const start = blockAt + block.indexOf('\n') + 1
    assert.equal(body.slice(start, start + 'bar()'.length), 'bar()', `selection off for ${typed}`)
  }
})
