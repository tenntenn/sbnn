/**
 * LoadOrder keeps a slow response from undoing a faster, newer one.
 *
 * A page has two kinds of reload in flight at once since #403: a full one (the
 * diffs, the comments, the review fields, the status) and a comments-only one
 * (the comments and the status). Responses can arrive in any order, so each
 * reload takes a ticket when it starts and asks before applying a part of its
 * answer whether anything newer has started since:
 *
 *   comments and status  - stale once any newer reload, of either kind, has
 *                          started: a newer one read them later
 *   diffs and the review - stale once a newer full reload has started; a
 *                          comments-only reload never reads them, so it does
 *                          not make an older full one stale for these
 *
 * Each part is judged on its own, so an old full response whose comments have
 * been overtaken still delivers the diffs nothing else brought.
 */
export class LoadOrder {
  private started = 0
  private startedFull = 0

  /** begin takes a ticket for a reload that is starting now. */
  begin(full: boolean): LoadTicket {
    const n = ++this.started
    if (full) this.startedFull = n
    return {
      freshComments: () => n === this.started,
      freshDiffs: () => full && n === this.startedFull,
    }
  }
}

export interface LoadTicket {
  /** freshComments says the comments and the status of this response are the
   * newest the page has asked for. */
  freshComments(): boolean
  /** freshDiffs says the same of the diffs and the review fields; a
   * comments-only reload never has any. */
  freshDiffs(): boolean
}
