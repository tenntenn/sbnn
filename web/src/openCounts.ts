/**
 * How many unresolved comments each file and each round holds, counted in one
 * pass over the comments (#402).
 *
 * The sidebar used to ask "how many open comments are on this file" once per
 * file, by filtering the whole comment list each time: with 3000 files and 1000
 * comments that is three million comparisons on every event, and it was the
 * largest single function in the profile of one comment event.
 *
 * The inner maps are handed back as the very same object when a round's counts
 * did not change, so that a sidebar round memoised on them stays put while a
 * comment lands on a different round.
 */
import type { Comment } from './types'

/** FileCounts maps a file id to its number of unresolved comments; a file with
 * none is absent. */
export type FileCounts = ReadonlyMap<string, number>

export interface OpenCounts {
  /** byRound maps a round (diff) id to its file counts. */
  byRound: ReadonlyMap<string, FileCounts>
  /** totals maps a round id to its number of unresolved comments. */
  totals: ReadonlyMap<string, number>
}

const NONE: FileCounts = new Map()

/** noCounts is the answer for a round nobody commented on; it is one object so
 * that such a round compares equal to itself across renders. */
export function noCounts(): FileCounts {
  return NONE
}

/**
 * openCounts counts the unresolved comments by round and by file. When prev
 * is given, a round whose counts are the same as in prev keeps prev's object,
 * and prev itself is returned when nothing at all changed.
 */
export function openCounts(comments: readonly Comment[], prev?: OpenCounts): OpenCounts {
  const fresh = new Map<string, Map<string, number>>()
  const totals = new Map<string, number>()
  for (const c of comments) {
    if (c.resolved) continue
    let files = fresh.get(c.diffId)
    if (!files) {
      files = new Map()
      fresh.set(c.diffId, files)
    }
    files.set(c.fileId, (files.get(c.fileId) ?? 0) + 1)
    totals.set(c.diffId, (totals.get(c.diffId) ?? 0) + 1)
  }

  let reused = prev !== undefined && prev.byRound.size === fresh.size
  const byRound = new Map<string, FileCounts>()
  for (const [round, files] of fresh) {
    const old = prev?.byRound.get(round)
    if (old !== undefined && sameCounts(old, files)) {
      byRound.set(round, old)
    } else {
      byRound.set(round, files)
      reused = false
    }
  }
  if (prev !== undefined && reused) return prev
  return { byRound, totals }
}

function sameCounts(a: FileCounts, b: FileCounts): boolean {
  if (a.size !== b.size) return false
  for (const [file, n] of b) if (a.get(file) !== n) return false
  return true
}
