import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

import { buildPrompt } from '../src/prompt'
import type { Comment, Diff, Verdict } from '../src/types'

/**
 * The prompt is rendered twice in this repository: by the server, and again
 * here for pages written with `sbnn export`, which have no server to ask.
 * Both are claimed to produce the same text and drifted apart anyway, so the
 * corpus in internal/server/testdata/prompt writes the contract down: an
 * input group in JSON, the text it must produce beside it. The Go test reads
 * those files, and so does this one - that is the whole point of keeping the
 * corpus in a form neither language owns.
 *
 * A failure here means the two renderers disagree. Whichever is wrong, the
 * fix is to make this file and prompt.go agree with the golden text again,
 * not to edit the golden text alone.
 */
const corpus = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'server', 'testdata', 'prompt')

interface Fixture {
  doc: string
  options?: { includeResolved?: boolean; noInstruction?: boolean }
  group: {
    name: string
    reviewedAt?: string
    reviewNote?: string
    reviewVerdict?: Verdict
    diffs?: Partial<Diff>[]
    comments?: Partial<Comment>[]
  }
}

/** comment fills in what the fixtures leave out, the way Go's zero values do. */
function comment(c: Partial<Comment>): Comment {
  return {
    id: c.id ?? '',
    group: c.group ?? '',
    diffId: c.diffId ?? '',
    fileId: c.fileId ?? '',
    path: c.path ?? '',
    author: c.author,
    side: c.side ?? 'new',
    startLine: c.startLine ?? 0,
    endLine: c.endLine ?? 0,
    body: c.body ?? '',
    snippet: c.snippet ?? '',
    question: c.question ?? false,
    resolved: c.resolved ?? false,
    createdAt: c.createdAt ?? '',
    updatedAt: c.updatedAt ?? '',
  }
}

const names = readdirSync(corpus)
  .filter((f) => f.endsWith('.json'))
  .map((f) => f.slice(0, -'.json'.length))

describe('buildPrompt renders what the server renders', () => {
  it('has a corpus to check against', () => {
    expect(names.length).toBeGreaterThan(0)
  })

  it.each(names)('%s', (name) => {
    const fx = JSON.parse(readFileSync(join(corpus, `${name}.json`), 'utf8')) as Fixture
    const want = readFileSync(join(corpus, `${name}.golden`), 'utf8')
    const g = fx.group
    const got = buildPrompt(
      g.name,
      (g.diffs ?? []) as Diff[],
      (g.comments ?? []).map(comment),
      { reviewedAt: g.reviewedAt, reviewNote: g.reviewNote, reviewVerdict: g.reviewVerdict },
      fx.options ?? {},
    )
    expect(got).toBe(want)
  })
})

describe('the corpus keeps covering what the two renderers disagreed about', () => {
  const all = names.map((n) => readFileSync(join(corpus, `${n}.golden`), 'utf8')).join('')
  it.each([
    'The reviewer approved the change',
    'The reviewer asked for changes',
    'left comments without deciding either way',
    'The reviewer wrote:',
    'came with the approval',
    'to address',
    'The change is approved, so none of this blocks it',
    'Address every comment above',
  ])('pins %j', (phrase) => {
    expect(all).toContain(phrase)
  })
})
