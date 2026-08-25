import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { wordDiff } from '../src/wordDiff'

/** changed returns the text wordDiff marks as changed on each side. */
function changed(a: string, b: string): [string, string] {
  const [left, right] = wordDiff(a, b)
  const pick = (segs: { text: string; changed: boolean }[] | null) =>
    (segs ?? []).filter((s) => s.changed).map((s) => s.text).join('')
  return [pick(left), pick(right)]
}

/** whole returns each side rebuilt from its segments; nothing may be lost. */
function whole(a: string, b: string): [string, string] {
  const [left, right] = wordDiff(a, b)
  const join = (segs: { text: string }[] | null) => (segs ?? []).map((s) => s.text).join('')
  return [join(left), join(right)]
}

/** lone reports whether a string contains an unpaired surrogate: half an
 * emoji, which is what issue #150 is about. */
function lone(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i)
    if (c >= 0xd800 && c <= 0xdbff) {
      const next = s.charCodeAt(i + 1)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return true
      i++
    } else if (c >= 0xdc00 && c <= 0xdfff) {
      return true
    }
  }
  return false
}

describe('wordDiff', () => {
  // Issue #150: the tokenizer split on code units, so the trailing common
  // tail could start in the middle of a surrogate pair and each side was cut
  // between the two halves of one emoji. The browser then drew a replacement
  // character where the reader had written an emoji.
  it('never cuts a surrogate pair in half', () => {
    const cases: [string, string][] = [
      ['status: 😀', 'status: 😁'],
      ['a 👨‍👩‍👧 b', 'a 👨‍👩‍👦 b'],
      ['flag 🇯🇵 here', 'flag 🇬🇧 here'],
      ['wave 👋🏽 now', 'wave 👋🏻 now'],
      ['🙂', '🙃'],
      ['x🙂y', 'x🙃y'],
    ]
    for (const [a, b] of cases) {
      const [left, right] = wordDiff(a, b)
      for (const seg of [...(left ?? []), ...(right ?? [])]) {
        assert.ok(!lone(seg.text), `segment ${JSON.stringify(seg.text)} of ${a} / ${b} is half a character`)
      }
    }
  })

  it('rebuilds each side exactly', () => {
    const cases: [string, string][] = [
      ['status: 😀', 'status: 😁'],
      ['  const a = 1', '  const a = 2'],
      ['', 'added'],
      ['removed', ''],
      ['same', 'same'],
      ['é combining', 'é combining'],
    ]
    for (const [a, b] of cases) {
      assert.deepEqual(whole(a, b), [a, b])
    }
  })

  it('still highlights whole words, not letters', () => {
    assert.deepEqual(changed('const alpha = 1', 'const bravo = 1'), ['alpha', 'bravo'])
    assert.deepEqual(changed('  value = compute(a, b)', '  value = compute(a, c)'), ['b', 'c'])
  })

  // A combining accent belongs to the letter it sits on, and the letter
  // belongs to the word: "café" is one token however it is encoded.
  it('keeps a combining accent with its letter and its word', () => {
    assert.deepEqual(changed('le café est', 'le thé est'), ['café', 'thé'])
  })

  // Regression: an enclosing keycap and a variation selector are combining
  // marks, so "1️⃣" matched the word-character rule and was glued
  // onto the word beside it. The emoji is not part of that word, and gluing
  // it there moved the highlight off the text that actually changed.
  it('does not glue a keycap emoji onto the word beside it', () => {
    assert.deepEqual(changed('step 1️⃣ alpha', 'step 1️⃣ bravo'), ['alpha', 'bravo'])
    assert.deepEqual(changed('1️⃣x', '2️⃣x'), ['1️⃣', '2️⃣'])
  })

  it('marks nothing when the lines are the same', () => {
    assert.deepEqual(changed('unchanged 😀', 'unchanged 😀'), ['', ''])
  })

  // Almost every line of almost every diff is ASCII, and those take the
  // pattern rather than the segmenter, so the two paths have to agree. The
  // same pair, with one non-ASCII character appended to both sides to force
  // the segmenter, must be segmented identically up to that character.
  it('segments an ASCII line the same way on either path', () => {
    const a = '  const value = compute(alpha, beta) // note'
    const b = '  const value = compute(alpha, gamma) // note'
    const texts = (pair: [string, string]) => {
      const [left, right] = wordDiff(pair[0], pair[1])
      return [
        (left ?? []).map((s) => `${s.changed ? '!' : ' '}${s.text}`),
        (right ?? []).map((s) => `${s.changed ? '!' : ' '}${s.text}`),
      ]
    }
    const ascii = texts([a, b])
    // 「」 is one grapheme cluster and is not ASCII, so this pair goes
    // through Intl.Segmenter while the pair above does not.
    const wide = texts([`${a}「`, `${b}「`])
    const dropTail = (segs: string[]) => segs.map((t) => t.replace(/「$/, ''))
    assert.deepEqual(dropTail(wide[0]), ascii[0])
    assert.deepEqual(dropTail(wide[1]), ascii[1])
  })
})
