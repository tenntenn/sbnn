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

import { suggestions, withSuggestion } from '../src/suggestion.ts'

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
