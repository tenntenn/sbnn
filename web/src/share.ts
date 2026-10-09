/**
 * Keeping what did not change the same object (#376).
 *
 * Every SSE event makes the page fetch the whole group again, and a fetch
 * returns freshly parsed objects for every round, file and comment. React
 * decides what to redo by comparing props with ===, so a reload that changed
 * one comment still handed every file section a "new" file, a "new" diff and a
 * "new" comment, and the page rendered all of them again: with 400 files that
 * was 670 ms of main thread per event and a growing heap (measured in #376).
 * Reusing the previous object wherever the content is the same lets
 * everything below stay put.
 *
 * Both functions are applied inside the state updater (App.tsx), so no render
 * reads or writes anything outside its props and state.
 */
import { hunksOf, type Comment, type Diff, type FileDiff } from './types'

/**
 * shareDiffs returns next with every round the page already holds replaced by
 * the object it holds, when the round is the same: same id, title, creation
 * time and size of the raw diff, and every file equal by sameFile.
 *
 * When nothing at all changed, prev itself is returned so that setState bails
 * out and nothing renders.
 */
export function shareDiffs(prev: Diff[], next: Diff[]): Diff[] {
  const byId = new Map(prev.map((d) => [d.id, d]))
  const merged = next.map((d) => {
    const old = byId.get(d.id)
    return old !== undefined && sameDiff(old, d) ? old : d
  })
  return sameElements(prev, merged) ? prev : merged
}

function sameDiff(a: Diff, b: Diff): boolean {
  return (
    a.title === b.title &&
    a.baseDir === b.baseDir &&
    a.createdAt === b.createdAt &&
    (a.raw?.length ?? 0) === (b.raw?.length ?? 0) &&
    a.files.length === b.files.length &&
    a.files.every((f, i) => sameFile(f, b.files[i]))
  )
}

/**
 * sameFile compares every field of a FileDiff the page renders, and the
 * hunks by their number and the number of lines in them: the hunks are the
 * bulk of the payload, and a file whose hunks changed in place with the same
 * shape would also have a different addition or deletion count.
 */
export function sameFile(a: FileDiff, b: FileDiff): boolean {
  return (
    a.id === b.id &&
    a.oldPath === b.oldPath &&
    a.newPath === b.newPath &&
    a.status === b.status &&
    a.isBinary === b.isBinary &&
    a.oldMode === b.oldMode &&
    a.newMode === b.newMode &&
    a.additions === b.additions &&
    a.deletions === b.deletions &&
    a.viewMode === b.viewMode &&
    a.isMarkdown === b.isMarkdown &&
    a.isImage === b.isImage &&
    a.imageStatus === b.imageStatus &&
    a.imageSize === b.imageSize &&
    a.isNotebook === b.isNotebook &&
    a.folded === b.folded &&
    a.foldReason === b.foldReason &&
    hunkShape(a) === hunkShape(b)
  )
}

function hunkShape(f: FileDiff): string {
  const hunks = hunksOf(f)
  let lines = 0
  for (const h of hunks) lines += h.lines.length
  return `${hunks.length}:${lines}`
}

/**
 * shareComments is shareDiffs for comments, which do change (an edit, a
 * resolve), so a comment is reused only when every field is the same.
 */
export function shareComments(prev: Comment[], next: Comment[]): Comment[] {
  const byId = new Map(prev.map((c) => [c.id, c]))
  const merged = next.map((c) => {
    const old = byId.get(c.id)
    return old !== undefined && sameComment(old, c) ? old : c
  })
  return sameElements(prev, merged) ? prev : merged
}

function sameComment(a: Comment, b: Comment): boolean {
  return (
    a.group === b.group &&
    a.diffId === b.diffId &&
    a.fileId === b.fileId &&
    a.path === b.path &&
    a.author === b.author &&
    a.side === b.side &&
    a.startLine === b.startLine &&
    a.endLine === b.endLine &&
    a.body === b.body &&
    a.snippet === b.snippet &&
    a.question === b.question &&
    a.resolved === b.resolved &&
    a.createdAt === b.createdAt &&
    a.updatedAt === b.updatedAt &&
    sameElements(a.suggestions ?? [], b.suggestions ?? [])
  )
}

/** sameElements reports whether two lists hold the same objects in the same order. */
export function sameElements<T>(a: readonly T[], b: readonly T[]): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i])
}
