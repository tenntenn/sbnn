import type { Comment } from './types'

/**
 * Comments and their replies (#423).
 *
 * A reply is an ordinary comment that names, in replyTo, the comment opening
 * its thread. It carries that comment's anchor - diff, file, lines, snippet -
 * so everything that finds comments by where they are finds the replies with
 * them, and the same state, so that a filter on resolved keeps or drops a
 * thread whole. Threads are flat: an answer to a reply names the first
 * comment, not the reply.
 *
 * The rules are model.Threads and the store in internal/server; this file is
 * their copy for the page, and for an exported page, which keeps its comments
 * in the browser and has to apply them itself.
 */

/** Thread is a comment and the replies to it, in the order they were
 * written. */
export interface Thread {
  root: Comment
  replies: Comment[]
}

/** isReply says whether a comment answers another rather than opening a
 * thread. */
export function isReply(c: Pick<Comment, 'replyTo'>): boolean {
  return !!c.replyTo
}

/**
 * threadsOf gathers comments into threads, in the order their first comments
 * appear. A reply whose first comment is not in the list opens a thread of
 * its own rather than disappearing.
 */
export function threadsOf(comments: readonly Comment[]): Thread[] {
  const roots = new Set<string>()
  for (const c of comments) if (!isReply(c)) roots.add(c.id)
  const threads: Thread[] = []
  const at = new Map<string, Thread>()
  for (const c of comments) {
    if (isReply(c) && roots.has(c.replyTo!)) continue
    const t: Thread = { root: c, replies: [] }
    if (!isReply(c)) at.set(c.id, t)
    threads.push(t)
  }
  for (const c of comments) {
    if (isReply(c) && roots.has(c.replyTo!)) at.get(c.replyTo!)!.replies.push(c)
  }
  return threads
}

/** openThreadCount counts the threads still open: what the page means by
 * "open comments", a comment with three replies being one of them. */
export function openThreadCount(comments: readonly Comment[]): number {
  return threadsOf(comments).filter((t) => !t.root.resolved).length
}

/** threadCount counts the threads, open or not. */
export function threadCount(comments: readonly Comment[]): number {
  return threadsOf(comments).length
}

/** threadRoot returns the comment opening the thread that id is in, or
 * undefined when there is no comment id. */
export function threadRoot(comments: readonly Comment[], id: string): Comment | undefined {
  const c = comments.find((x) => x.id === id)
  if (!c || !isReply(c)) return c
  return comments.find((x) => x.id === c.replyTo) ?? c
}

/**
 * replyFields is what a reply to id takes from its thread: the anchor and the
 * state, and the id it names, which is always the first comment's. It is
 * undefined when there is nothing to reply to.
 */
export function replyFields(
  comments: readonly Comment[],
  id: string,
):
  | Pick<
      Comment,
      'replyTo' | 'diffId' | 'fileId' | 'path' | 'side' | 'startLine' | 'endLine' | 'snippet' | 'resolved'
    >
  | undefined {
  const root = threadRoot(comments, id)
  if (!root) return undefined
  return {
    replyTo: root.id,
    diffId: root.diffId,
    fileId: root.fileId,
    path: root.path,
    side: root.side,
    startLine: root.startLine,
    endLine: root.endLine,
    snippet: root.snippet,
    resolved: root.resolved,
  }
}

/** setThreadResolved resolves or reopens the thread id is in: its first
 * comment and every reply to it. */
export function setThreadResolved(comments: readonly Comment[], id: string, resolved: boolean, now: string): Comment[] {
  const root = threadRoot(comments, id)
  if (!root) return [...comments]
  return comments.map((c) =>
    c.id === root.id
      ? { ...c, resolved, updatedAt: now }
      : c.replyTo === root.id
        ? { ...c, resolved }
        : c,
  )
}

/** withoutComment deletes one comment, and the replies with it when it opens
 * a thread. */
export function withoutComment(comments: readonly Comment[], id: string): Comment[] {
  const target = comments.find((c) => c.id === id)
  if (!target) return [...comments]
  const cascade = !isReply(target)
  return comments.filter((c) => c.id !== id && !(cascade && c.replyTo === id))
}
