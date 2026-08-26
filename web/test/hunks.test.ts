import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it } from 'node:test'
import { estimatedHeight } from '../src/estimatedHeight'
import { searchFile } from '../src/search'
import { hunksOf } from '../src/types'
import type { FileDiff, Hunk } from '../src/types'

/**
 * The bug: a file the diff carries without any hunks - a pure rename, a
 * mode change, a binary blob - arrives from the server as `"hunks": null`,
 * and every reader iterated the field because the type said `Hunk[]`. The
 * throw happened while the whole stack was rendering, so one such file left
 * the page as an empty `<div id="root">` - measured in Chromium as nine DOM
 * nodes and `TypeError: file.hunks is not iterable`, with no file section
 * and no diff table on the page at all.
 */
function file(hunks: Hunk[] | null): FileDiff {
  return {
    id: 'f1-abcdef12',
    oldPath: 'old.txt',
    newPath: 'new.txt',
    status: 'renamed',
    isBinary: false,
    additions: 0,
    deletions: 0,
    viewMode: 'unified',
    isMarkdown: false,
    isImage: false,
    isNotebook: false,
    hunks,
  }
}

const oneHunk: Hunk = {
  header: '@@ -1,1 +1,1 @@',
  oldStart: 1,
  oldLines: 1,
  newStart: 1,
  newLines: 1,
  lines: [{ kind: 'context', oldNumber: 1, newNumber: 1, content: 'hello' }],
}

describe('hunksOf', () => {
  it('answers an empty list for the null the server sends', () => {
    assert.deepEqual(hunksOf(file(null)), [])
  })

  it('hands back the same array a file that has hunks holds', () => {
    const hunks = [oneHunk]
    assert.equal(hunksOf(file(hunks)), hunks)
  })

  it('answers the same value every time, so it is stable as a prop', () => {
    // SplitTable keys a useMemo on the hunks it is given; a fresh [] per
    // call would redo the word diff of every row on every render.
    assert.equal(hunksOf(file(null)), hunksOf(file(null)))
  })
})

describe('estimatedHeight', () => {
  it('does not throw on a file whose hunks are null', () => {
    assert.doesNotThrow(() => estimatedHeight(file(null), false, 0, 'unified'))
    assert.doesNotThrow(() => estimatedHeight(file(null), false, 0, 'split'))
  })

  it('reserves the height of the "No content change" line, not bare chrome', () => {
    const empty = estimatedHeight(file(null), false, 0, 'unified')
    const folded = estimatedHeight(file(null), true, 0, 'unified')
    assert.ok(
      empty > folded,
      `an unfolded hunkless section (${empty}px) draws a line a folded one (${folded}px) does not`,
    )
    // Measured at 122px in Chromium; being far under it is what made the
    // scrollbar of a diff full of renames meaningless.
    assert.ok(empty >= 100, `estimated ${empty}px for a section measured at 122px`)
  })

  it('still counts the rows of a file that has hunks', () => {
    // The empty case is a flat allowance, so it is not comparable to a
    // one-line file; what matters is that a longer file still reads taller.
    const one = estimatedHeight(file([oneHunk]), false, 0, 'unified')
    const three = estimatedHeight(file([oneHunk, oneHunk, oneHunk]), false, 0, 'unified')
    assert.ok(three > one, `three hunks read ${three}px, one reads ${one}px`)
  })

  it('allows for comments on a hunkless file', () => {
    const bare = estimatedHeight(file(null), false, 0, 'unified')
    assert.ok(estimatedHeight(file(null), false, 2, 'unified') > bare)
  })
})

describe('searchFile', () => {
  it('does not throw on a file whose hunks are null', () => {
    const got = searchFile(file(null), ['new'])
    assert.equal(got.inPath, true)
    assert.equal(got.lines, 0)
    assert.equal(got.scanned, 0)
    assert.equal(got.firstLine, null)
  })
})

/**
 * The guard has to hold everywhere, not only where it was first needed.
 * Neither JSX nor a browser is available here - the resolve hook loads .ts
 * and not .tsx - so this reads the sources that have to agree: nothing
 * outside types.ts touches a file's `hunks` field directly.
 */
describe('the sources', () => {
  const srcDir = fileURLToPath(new URL('../src', import.meta.url))

  function modules(dir: string): string[] {
    const out: string[] = []
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) out.push(...modules(path))
      else if (/\.tsx?$/.test(entry.name)) out.push(path)
    }
    return out
  }

  it('read hunks through hunksOf and nowhere else', () => {
    const offenders: string[] = []
    for (const path of modules(srcDir)) {
      if (path.endsWith(`${join('src', 'types.ts')}`)) continue // hunksOf itself lives here.
      const lines = readFileSync(path, 'utf8').split('\n')
      lines.forEach((line, i) => {
        const code = line.replace(/^\s*(\/\/|\*|\/\*).*/, '')
        if (/\.hunks\b/.test(code)) offenders.push(`${path.slice(srcDir.length + 1)}:${i + 1}: ${line.trim()}`)
      })
    }
    assert.deepEqual(
      offenders,
      [],
      `these read .hunks directly; the server sends null for a rename, a mode change or a binary file:\n${offenders.join('\n')}`,
    )
  })
})
