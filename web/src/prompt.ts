import type { Comment, Diff, Verdict } from './types'
import { suggestions } from './suggestion'
import { threadsOf, type Thread } from './threads'

/**
 * PromptReview is how the review ended, as far as the prompt is concerned.
 *
 * An exported page carries these in its payload, because the verdict is the
 * first thing an agent reads: the same three remarks block a change or do
 * not, depending on what the reviewer said about the change as a whole.
 */
export interface PromptReview {
  /** reviewedAt is when the review was submitted; unset until it is. */
  reviewedAt?: string
  reviewNote?: string
  reviewVerdict?: Verdict
}

/** PromptOptions matches server.PromptOptions, field for field. */
export interface PromptOptions {
  /** includeResolved keeps comments that were marked as resolved. */
  includeResolved?: boolean
  /** noInstruction drops the closing instruction. */
  noInstruction?: boolean
}

/**
 * buildPrompt renders review comments the same way the sbnn server does, so an
 * exported page produces the same text as `sbnn comments`.
 *
 * "The same" is not a hope here: the exact output of both renderers is pinned
 * by the golden corpus in internal/server/testdata/prompt, which is plain
 * JSON in and plain text out precisely so that this renderer, which is not
 * written in Go, can check itself against the very same files. Changing the
 * wording on either side means regenerating the corpus (go test -update) and
 * bringing the other renderer along.
 */
export function buildPrompt(
  group: string,
  diffs: Diff[],
  comments: Comment[],
  review: PromptReview = {},
  opts: PromptOptions = {},
): string {
  const open = threadsOf(comments).filter((t) => opts.includeResolved || !t.root.resolved)
  let out = `# Review comments (sbnn group ${JSON.stringify(group)})\n\n`

  // A round nobody has submitted yet says nothing about a verdict: there is
  // no reviewer to attribute one to.
  if (isReviewed(review.reviewedAt)) {
    out += `The reviewer ${verdictSentence(review.reviewVerdict)}.\n\n`
  }
  const note = (review.reviewNote ?? '').trim()
  if (note) out += `The reviewer wrote:\n\n${note}\n\n`

  if (open.length === 0) return out + 'No open review comments.\n'

  // A question somebody else has answered is not waiting on the reader any
  // more, so it is not counted among the things to answer.
  const questions = open.filter((t) => t.root.question && !answered(t)).length
  const replies = open.reduce((n, t) => n + t.replies.length, 0)
  // An approval that came with comments is not a list of things to do, and
  // saying it is sends an agent off to change code nobody asked it to.
  out +=
    review.reviewVerdict === 'approved'
      ? `${open.length} comment(s) came with the approval${asking(questions)}.\n`
      : `${open.length} comment(s) to address${asking(questions)}.\n`

  const titles = new Map(diffs.map((d) => [d.id, d.title]))
  // Only the comment that opens a thread is numbered, and its id is printed
  // so that the reader can reply to it.
  open.forEach((t, i) => {
    const c = t.root
    out += `\n## ${i + 1}. ${c.path}${lineRange(c)}\n`
    if (c.id) out += `\nComment ID: ${c.id}\n`
    const title = titles.get(c.diffId)
    if (title) out += `\nDiff: ${title}\n`
    if (c.author) out += `\nFrom: ${c.author}\n`
    if (c.question) {
      out += answered(t)
        ? '\nThis one is a question, and it has been answered below.\n'
        : '\nThis one is a question: answer it.\n'
    }
    if (c.resolved) out += '\nStatus: resolved\n'
    const snippet = c.snippet.replace(/\n+$/, '')
    if (snippet) {
      const fence = fenceFor(snippet)
      out += `\n${fence}\n${snippet}\n${fence}\n`
    }
    out += commentBody(c)
    // A reply is on the lines of its thread, so its snippet would only
    // repeat the one above.
    for (const r of t.replies) {
      out += `\n### Reply from ${r.author || 'the reviewer'}\n`
      out += commentBody(r)
    }
  })

  if (!opts.noInstruction) {
    out += '\n---\n\n'
    out +=
      review.reviewVerdict === 'approved'
        ? 'The change is approved, so none of this blocks it. Act on what is ' +
          'worth acting on, say what you are leaving and why, and carry on.\n'
        : 'Address every comment above. A suggestion block replaces the lines it ' +
          'names, verbatim. When a comment is not worth acting on, say why instead of ' +
          'changing the code.\n'
    if (replies > 0) {
      out +=
        '\nA comment with replies under it is a conversation: read it to the ' +
        'end, because the newest reply is where it stands now.\n'
    }
    if (questions > 0) {
      out +=
        '\nA comment marked as a question is asking for an answer, not for a ' +
        'change. Answer it in words - as a reply to it where you can, with ' +
        '`sbnn comment --reply-to <its comment ID> -m "..."`, so the answer sits ' +
        'next to the line - and change the code only if your own answer says it ' +
        'should change. Leaving a question unanswered is the one thing that makes ' +
        'the reviewer ask it again.\n'
    }
  }
  return out
}

/** commentBody writes what a comment says, and what its suggestion blocks
 * replace. */
function commentBody(c: Comment): string {
  let out = ''
  // The body is Markdown and may carry suggestion blocks, so it goes out as
  // it is rather than quoted line by line.
  const body = c.body.replace(/\n+$/, '')
  if (body) out += `\n${body}\n`
  const blocks = suggestions(c.body)
  if (blocks.length > 0) {
    out += `\nThe suggestion block above replaces ${c.path}${lineRange(c)}.\n`
    if (blocks.length > 1) {
      out += `(${blocks.length} suggestion blocks: apply them in order.)\n`
    }
  }
  return out
}

/** answered says whether a question thread has a reply from someone other
 * than whoever asked it; a follow-up from the asker is more of the question.
 * The rule is server.answered. */
function answered(t: Thread): boolean {
  return t.replies.some((r) => (r.author ?? '') !== (t.root.author ?? ''))
}

/**
 * isReviewed says whether a review was ever submitted.
 *
 * Go writes an unsubmitted time as the zero Time, which reaches the browser
 * either as a missing field or as the year 1 - both mean the same thing, and
 * claiming a verdict for a round nobody submitted is worse than saying
 * nothing.
 */
function isReviewed(reviewedAt?: string): boolean {
  if (!reviewedAt) return false
  return !reviewedAt.startsWith('0001-01-01T00:00:00')
}

/**
 * verdictSentence says what the reviewer decided, in words an agent can act
 * on without counting anything. The wording is server.verdictSentence.
 */
function verdictSentence(v?: Verdict): string {
  switch (v) {
    case 'approved':
      return 'approved the change; anything below is worth reading but does not block it'
    case 'changes-requested':
      return 'asked for changes; the change should not go ahead as it is'
    default:
      return 'left comments without deciding either way'
  }
}

/**
 * asking counts the comments that want an answer rather than a change, for
 * the line that says how much there is to do.
 */
function asking(questions: number): string {
  return questions > 0 ? `, ${questions} of them a question` : ''
}

function lineRange(c: Comment): string {
  if (c.startLine <= 0) return ''
  const side = c.side === 'old' ? ' (old)' : ''
  return c.endLine > c.startLine ? `:${c.startLine}-${c.endLine}${side}` : `:${c.startLine}${side}`
}

function fenceFor(content: string): string {
  let longest = 0
  let current = 0
  for (const ch of content) {
    if (ch === '`') {
      current++
      longest = Math.max(longest, current)
    } else {
      current = 0
    }
  }
  return longest < 3 ? '```' : '`'.repeat(longest + 1)
}
