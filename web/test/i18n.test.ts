import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { catalogs, detectLanguage, LANGUAGES, t, tn } from '../src/i18n'

/**
 * The catalogs are the UI's words, and a language is only as finished as its
 * dictionary. TypeScript already refuses a catalog that lacks a key the
 * English one has, but it reads the values as plain strings: a placeholder
 * renamed in one language and not the other type-checks perfectly, and the
 * reader is then shown a literal "{count}". These hold the dictionaries to
 * the same keys and the same placeholders, whatever the compiler lets by.
 */

const placeholders = (message: string): string[] =>
  [...new Set(Array.from(message.matchAll(/\{(\w+)\}/g), (m) => m[1]))].sort()

describe('the message catalogs', () => {
  const reference = catalogs.en

  for (const lang of LANGUAGES) {
    if (lang === 'en') continue
    const catalog = catalogs[lang]

    it(`${lang} has every key English has, and no other`, () => {
      const missing = Object.keys(reference).filter((key) => !(key in catalog))
      const extra = Object.keys(catalog).filter((key) => !(key in reference))
      assert.deepEqual(missing, [], `${lang} is missing ${missing.join(', ')}`)
      assert.deepEqual(extra, [], `${lang} has keys English does not: ${extra.join(', ')}`)
    })

    it(`${lang} uses the same placeholders as English`, () => {
      for (const [key, english] of Object.entries(reference)) {
        const translated = (catalog as Record<string, string>)[key]
        if (translated === undefined) continue
        assert.deepEqual(
          placeholders(translated),
          placeholders(english),
          `${lang} ${key}: ${JSON.stringify(translated)} does not fill the placeholders of ${JSON.stringify(english)}`,
        )
      }
    })

    it(`${lang} leaves nothing empty`, () => {
      for (const [key, message] of Object.entries(catalog)) {
        assert.notEqual(message.trim(), '', `${lang} ${key} is empty`)
      }
    })
  }

  // A plural is looked up as base.one or base.other, so a base with only one
  // of the two is a lookup that falls through to a missing key.
  it('pairs every .one with an .other', () => {
    for (const key of Object.keys(reference)) {
      if (key.endsWith('.one')) {
        const other = key.replace(/\.one$/, '.other')
        assert.ok(other in reference, `${key} has no ${other}`)
      }
    }
  })
})

describe('detectLanguage', () => {
  it('takes the first language in the list the page speaks', () => {
    assert.equal(detectLanguage(['ja-JP', 'en-US']), 'ja')
    assert.equal(detectLanguage(['ja']), 'ja')
    assert.equal(detectLanguage(['fr-FR', 'ja', 'en']), 'ja')
    assert.equal(detectLanguage(['en-GB', 'ja']), 'en')
  })

  it('falls back to English', () => {
    assert.equal(detectLanguage([]), 'en')
    assert.equal(detectLanguage(['fr', 'de-DE']), 'en')
  })
})

// The test process has no reader whose language could be followed, so the
// page speaks English here whatever the machine's locale is - which is what
// lets the other suites assert English sentences.
describe('t', () => {
  it('speaks English outside a page', () => {
    assert.equal(t('fold.plain'), 'Folded')
  })

  it('fills placeholders and leaves unknown ones alone', () => {
    assert.equal(t('sidebar.found', { found: 2, total: 5 }), '2 of 5')
    assert.equal(t('sidebar.found', { found: 2 }), '2 of {total}')
  })

  it('picks the plural form by the count', () => {
    assert.equal(tn('search.lines', 1), '1 line')
    assert.equal(tn('search.lines', 3), '3 lines')
  })
})
