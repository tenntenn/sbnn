import type { Comment, Diff, Verdict } from './types'
import { suggestions } from './suggestion'

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
  const open = comments.filter((c) => opts.includeResolved || !c.resolved)
  let out = `# Review comments (sbnn group ${JSON.stringify(group)})\n\n`

  // A round nobody has submitted yet says nothing about a verdict: there is
  // no reviewer to attribute one to.
  if (isReviewed(review.reviewedAt)) {
    out += `The reviewer ${verdictSentence(review.reviewVerdict)}.\n\n`
  }
  const note = (review.reviewNote ?? '').trim()
  if (note) out += `The reviewer wrote:\n\n${note}\n\n`

  if (open.length === 0) return out + 'No open review comments.\n'

  const questions = open.filter((c) => c.question).length
  // An approval that came with comments is not a list of things to do, and
  // saying it is sends an agent off to change code nobody asked it to.
  out +=
    review.reviewVerdict === 'approved'
      ? `${open.length} comment(s) came with the approval${asking(questions)}.\n`
      : `${open.length} comment(s) to address${asking(questions)}.\n`

  const titles = new Map(diffs.map((d) => [d.id, d.title]))
  open.forEach((c, i) => {
    out += `\n## ${i + 1}. ${c.path}${lineRange(c)}\n`
    const title = titles.get(c.diffId)
    if (title) out += `\nDiff: ${title}\n`
    if (c.author) out += `\nFrom: ${c.author}\n`
    if (c.question) out += '\nThis one is a question: answer it.\n'
    if (c.resolved) out += '\nStatus: resolved\n'
    const snippet = c.snippet.replace(/\n+$/, '')
    if (snippet) {
      const fence = fenceFor(snippet)
      out += `\n${fence}\n${snippet}\n${fence}\n`
    }
    // The body is Markdown and may carry suggestion blocks, so it goes out
    // as it is rather than quoted line by line.
    const body = c.body.replace(/\n+$/, '')
    if (body) out += `\n${body}\n`
    const blocks = suggestions(c.body)
    if (blocks.length > 0) {
      out += `\nThe suggestion block above replaces ${c.path}${lineRange(c)}.\n`
      if (blocks.length > 1) {
        out += `(${blocks.length} suggestion blocks: apply them in order.)\n`
      }
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
    if (questions > 0) {
      out +=
        '\nA comment marked as a question is asking for an answer, not for a ' +
        'change. Answer it in your reply, in words, and change the code only if your ' +
        'own answer says it should change. Leaving a question unanswered is the one ' +
        'thing that makes the reviewer ask it again.\n'
    }
  }
  return out
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
