import { useMemo, useRef, useState } from 'react'
import type { Comment } from '../types'
import { client } from '../client'
import { COMMENT_ID_PREFIX } from '../previewComments'
import { renderComment } from '../markdown'
import { insertSuggestion, originalLines, parseBody, suggestionBlock, suggestions } from '../suggestion'
import { threadsOf, type Thread } from '../threads'
import { dateTime, t, useT } from '../i18n'

interface ThreadProps {
  group: string
  comments: Comment[]
  onChanged: () => void
  /** idPrefix names the comments in the DOM. A pane that draws the same
   * comments as another passes its own, so no id is on the page twice. */
  idPrefix?: string
}

/**
 * CommentThread renders the comments anchored to one line range: each
 * comment that opens a thread, the replies under it, and the form that adds
 * one more. A reply carries the anchor of its thread, so it arrives here in
 * the same list as the comment it answers.
 */
export function CommentThread({ group, comments, onChanged, idPrefix }: ThreadProps) {
  const threads = useMemo(() => threadsOf(comments), [comments])
  return (
    <div className="thread">
      {threads.map((thread) => (
        <ThreadItem key={thread.root.id} group={group} thread={thread} onChanged={onChanged} idPrefix={idPrefix} />
      ))}
    </div>
  )
}

/**
 * ThreadItem is one comment and its replies. Resolving and reopening are
 * the first comment's to offer, since they act on the whole thread; Reply is
 * too, and its form opens under the last reply, where the answer will land.
 */
function ThreadItem({
  group,
  thread,
  onChanged,
  idPrefix,
}: {
  group: string
  thread: Thread
  onChanged: () => void
  idPrefix?: string
}) {
  const [replying, setReplying] = useState(false)
  const replyButton = useRef<HTMLButtonElement>(null)
  const { root, replies } = thread
  // Focus goes back to the button that opened the form, so a keyboard
  // reader who sent or dropped a reply is where they were.
  const close = () => {
    setReplying(false)
    window.setTimeout(() => replyButton.current?.focus(), 0)
  }
  return (
    <div className="comment-thread">
      <CommentItem
        group={group}
        comment={root}
        onChanged={onChanged}
        idPrefix={idPrefix}
        replies={replies.length}
        replyButton={replyButton}
        replying={replying}
        onReply={() => setReplying(true)}
      />
      {replies.length > 0 && (
        <div className="replies" role="list" aria-label={t('comment.replies')}>
          {replies.map((r) => (
            <div key={r.id} role="listitem">
              <CommentItem group={group} comment={r} onChanged={onChanged} idPrefix={idPrefix} />
            </div>
          ))}
        </div>
      )}
      {replying && <ReplyForm group={group} root={root} onSent={onChanged} onClose={close} />}
    </div>
  )
}

/**
 * ReplyForm writes a reply to the thread root opens: the comment editor,
 * suggestions included where the thread is on lines of the new file, and
 * the same keys as a new comment - ⌘/Ctrl + Enter sends, Escape drops it.
 */
function ReplyForm({
  group,
  root,
  onSent,
  onClose,
}: {
  group: string
  root: Comment
  onSent: () => void
  onClose: () => void
}) {
  const t = useT()
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const editor = useRef<HTMLTextAreaElement>(null)
  const canSuggest = root.side !== 'old' && root.startLine > 0

  const submit = async () => {
    if (body.trim() === '' || busy) return
    setBusy(true)
    setError(null)
    try {
      await client.addReply(group, root.id, body)
      setBody('')
      onSent()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="reply-form" role="group" aria-label={t('comment.replyLabel')}>
      <CommentEditor
        editorRef={editor}
        value={body}
        onChange={setBody}
        onSubmit={() => void submit()}
        onCancel={onClose}
        placeholder={t('comment.replyPlaceholder')}
        label={t('comment.replyLabel')}
        autoFocus
      />
      <div className="comment-actions">
        <button disabled={busy || body.trim() === ''} onClick={() => void submit()}>
          {t('comment.sendReply')}
        </button>
        {canSuggest && (
          <SuggestButton
            editorRef={editor}
            seed={originalLines(root.snippet).join('\n')}
            disabled={busy}
            onInsert={setBody}
          />
        )}
        <button className="ghost" disabled={busy} onClick={onClose}>
          {t('common.cancel')}
        </button>
        <span className="hint" title={t('comment.sendHintTitle')}>
          {t('comment.sendHint')}
        </span>
      </div>
      {error && <div className="error inline">{error}</div>}
    </div>
  )
}

/** rangeLabel names what a comment is on. A comment that names no lines is
 * about the file as a whole, and "path:0" is not a place, so it is the bare
 * path - the same rule the prompt follows. */
function rangeLabel(c: Pick<Comment, 'path' | 'side' | 'startLine' | 'endLine'>): string {
  if (c.startLine <= 0) return c.path
  const lines = c.endLine > c.startLine ? `${c.startLine}-${c.endLine}` : `${c.startLine}`
  return `${c.path}:${lines}${c.side === 'old' ? t('diff.old') : ''}`
}

/**
 * SuggestedChange shows a proposed replacement the way a review tool does:
 * the lines as they are today, then the lines as they would read.
 */
function SuggestedChange({
  comment,
  suggestion,
}: {
  comment: Comment
  suggestion: string
}) {
  const [copied, setCopied] = useState(false)
  const before = originalLines(comment.snippet)
  const after = suggestion === '' ? [] : suggestion.split('\n')

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(suggestion)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard access can be refused; the text is on screen anyway.
    }
  }

  return (
    <div className="suggestion">
      <div className="suggestion-head">
        <span>{t('comment.suggestedChange', { range: rangeLabel(comment) })}</span>
        <button className="ghost" onClick={() => void copy()}>
          {copied ? t('common.copied') : t('common.copy')}
        </button>
      </div>
      <table className="suggestion-diff">
        <tbody>
          {before.map((line, i) => (
            <tr key={`b${i}`} className="line delete">
              <td className="marker">-</td>
              <td className="code">{line || ' '}</td>
            </tr>
          ))}
          {after.map((line, i) => (
            <tr key={`a${i}`} className="line add">
              <td className="marker">+</td>
              <td className="code">{line || ' '}</td>
            </tr>
          ))}
          {after.length === 0 && (
            <tr className="line">
              <td className="marker" />
              <td className="code hint">{t('comment.linesRemoved')}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

/**
 * CommentBody draws one prose segment of a comment.
 *
 * A comment body is Markdown - model.Comment says so, and `sbnn comments`
 * hands it to an agent as Markdown - so the reviewer reading the same field
 * in the browser gets it rendered too. renderComment returns HTML that has
 * been through DOMPurify, which is what makes it safe to set as innerHTML
 * here. Nothing else on this path may reach dangerouslySetInnerHTML.
 *
 * It is renderComment and not the preview's renderMarkdown because a comment
 * draws no image: the page is opened locally and the body is written by an
 * agent, so an <img> would turn opening a review into a request to whatever
 * host the comment named. renderComment says the rest.
 *
 * Suggestion blocks never arrive here: parseBody has already peeled them off
 * into their own segments, which SuggestedChange keeps drawing as a diff.
 */
function CommentBody({ text }: { text: string }) {
  const html = renderComment(text)
  return <div className="comment-body" dangerouslySetInnerHTML={{ __html: html }} />
}

function CommentItem({
  group,
  comment,
  onChanged,
  idPrefix = COMMENT_ID_PREFIX,
  replies = 0,
  replyButton,
  replying = false,
  onReply,
}: {
  group: string
  comment: Comment
  onChanged: () => void
  idPrefix?: string
  /** replies is how many replies the comment has, which deleting it takes
   * along. */
  replies?: number
  /** replyButton, replying and onReply are given for the comment that opens
   * a thread, which is the one that offers Reply. */
  replyButton?: React.RefObject<HTMLButtonElement | null>
  replying?: boolean
  onReply?: () => void
}) {
  const t = useT()
  const [editing, setEditing] = useState(false)
  const [body, setBody] = useState(comment.body)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const editor = useRef<HTMLTextAreaElement>(null)

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError(null)
    try {
      await fn()
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const segments = parseBody(comment.body)
  const reply = !!comment.replyTo
  const author = comment.author || t('comment.reviewer')

  return (
    <div
      id={`${idPrefix}${comment.id}`}
      className={`comment${reply ? ' reply' : ''}${comment.resolved ? ' resolved' : ''}`}
      aria-label={reply ? t('comment.replyFrom', { author }) : undefined}
    >
      <div className="comment-meta">
        {reply ? (
          // A reply is on its thread's lines, which the comment above
          // already names; what it needs is who said it, and when.
          <>
            {comment.author ? (
              <span className="badge author">{comment.author}</span>
            ) : (
              <span className="reply-author">{author}</span>
            )}
            <time className="reply-time" dateTime={comment.createdAt}>
              {dateTime(comment.createdAt)}
            </time>
          </>
        ) : (
          <>
            {comment.author && <span className="badge author">{comment.author}</span>}
            <span className="comment-range">{rangeLabel(comment)}</span>
          </>
        )}
        {!editing && suggestions(comment.body).length > 0 && (
          <span className="badge">{t('comment.suggestion')}</span>
        )}
        {comment.question && <span className="badge question">{t('comment.question')}</span>}
        {comment.resolved && !reply && <span className="badge">{t('comment.resolved')}</span>}
      </div>

      {editing ? (
        <>
          <CommentEditor
            editorRef={editor}
            value={body}
            onChange={setBody}
            onSubmit={() =>
              run(async () => {
                await client.updateComment(group, comment.id, { body })
                setEditing(false)
              })
            }
          />
          <div className="comment-actions">
            <button
              disabled={busy || body.trim() === ''}
              onClick={() =>
                run(async () => {
                  await client.updateComment(group, comment.id, { body })
                  setEditing(false)
                })
              }
            >
              {t('comment.save')}
            </button>
            <SuggestButton
              editorRef={editor}
              seed={originalLines(comment.snippet).join('\n')}
              disabled={busy || comment.side === 'old'}
              onInsert={setBody}
            />
            <button
              className="ghost"
              disabled={busy}
              onClick={() => {
                setBody(comment.body)
                setEditing(false)
              }}
            >
              {t('common.cancel')}
            </button>
          </div>
        </>
      ) : (
        <>
          {segments.map((segment, i) =>
            segment.kind === 'text' ? (
              <CommentBody key={i} text={segment.text} />
            ) : (
              <SuggestedChange key={i} comment={comment} suggestion={segment.text} />
            ),
          )}
          <div className="comment-actions">
            {onReply && (
              <button
                ref={replyButton}
                className="ghost"
                disabled={busy || replying}
                title={t('comment.replyTitle')}
                aria-expanded={replying}
                onClick={onReply}
              >
                {t('comment.reply')}
              </button>
            )}
            {!reply && (
              <button
                className="ghost"
                disabled={busy}
                title={comment.resolved ? t('comment.reopenTitle') : t('comment.resolveTitle')}
                onClick={() =>
                  run(() => client.updateComment(group, comment.id, { resolved: !comment.resolved }))
                }
              >
                {comment.resolved ? t('comment.reopen') : t('comment.resolve')}
              </button>
            )}
            <button className="ghost" disabled={busy} onClick={() => setEditing(true)}>
              {t('comment.edit')}
            </button>
            <button
              className="ghost danger"
              disabled={busy}
              title={
                reply
                  ? t('comment.deleteReplyTitle')
                  : replies > 0
                    ? t('comment.deleteThreadTitle')
                    : t('comment.deleteTitle')
              }
              onClick={() => run(() => client.deleteComment(group, comment.id))}
            >
              {t('comment.delete')}
            </button>
          </div>
        </>
      )}
      {error && <div className="error inline">{error}</div>}
    </div>
  )
}

interface EditorProps {
  editorRef: React.RefObject<HTMLTextAreaElement | null>
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  onCancel?: () => void
  autoFocus?: boolean
  /** placeholder and label replace the new comment's, for a reply. */
  placeholder?: string
  label?: string
}

function CommentEditor({ editorRef, value, onChange, onSubmit, onCancel, autoFocus, placeholder, label }: EditorProps) {
  return (
    <textarea
      ref={editorRef}
      className="comment-input"
      autoFocus={autoFocus}
      aria-label={label}
      rows={Math.max(3, Math.min(16, value.split('\n').length + 1))}
      placeholder={placeholder ?? t('comment.placeholder')}
      value={value}
      onChange={(ev) => onChange(ev.target.value)}
      onKeyDown={(ev) => {
        if (ev.key === 'Escape' && onCancel) onCancel()
        if (ev.key === 'Enter' && (ev.metaKey || ev.ctrlKey)) onSubmit()
      }}
    />
  )
}

/**
 * SuggestButton drops a suggestion block into the comment, pre-filled with
 * the lines as they read today — the same move as GitHub's "add a
 * suggestion" button.
 */
function SuggestButton({
  editorRef,
  seed,
  disabled,
  onInsert,
}: {
  editorRef: React.RefObject<HTMLTextAreaElement | null>
  seed: string
  disabled?: boolean
  onInsert: (value: string) => void
}) {
  const insert = () => {
    const textarea = editorRef.current
    const block = suggestionBlock(seed)
    if (!textarea) {
      onInsert(block)
      return
    }
    const value = textarea.value
    const at = textarea.selectionStart ?? value.length
    const { body: next, block: written, blockAt } = insertSuggestion(value, at, seed)
    onInsert(next)
    // Put the cursor inside the block so the text can be edited right away.
    window.setTimeout(() => {
      const start = blockAt + written.indexOf('\n') + 1
      textarea.focus()
      textarea.setSelectionRange(start, start + seed.length)
    }, 0)
  }

  return (
    <button className="ghost" disabled={disabled} onClick={insert} title={t('comment.suggestTitle')}>
      {t('comment.suggest')}
    </button>
  )
}

interface FormProps {
  onSubmit: (body: string, question: boolean) => Promise<void>
  onCancel: () => void
  label: string
  /** seed is the current text of the selected lines, what a suggestion
   * starts from. */
  seed: string
  /** canSuggest is false for lines that only exist in the old file. */
  canSuggest: boolean
  /** hint explains how to grow the selection. */
  hint?: string
}

/** CommentForm writes a new comment, suggestion blocks included. */
export function CommentForm({ onSubmit, onCancel, label, seed, canSuggest, hint }: FormProps) {
  const t = useT()
  const [body, setBody] = useState('')
  // Asking a question and asking for a change are different requests, and
  // the prose does not always tell them apart - "should this be a 404?" can
  // be either. So the writer says which it is.
  const [question, setQuestion] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const editor = useRef<HTMLTextAreaElement>(null)

  const submit = async () => {
    if (body.trim() === '') return
    setBusy(true)
    setError(null)
    try {
      await onSubmit(body, question)
      setBody('')
      setQuestion(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="comment-form">
      <div className="comment-meta">
        <span className="comment-range">{label}</span>
        {hint && <span className="hint">{hint}</span>}
      </div>
      <CommentEditor
        editorRef={editor}
        value={body}
        onChange={setBody}
        onSubmit={() => void submit()}
        onCancel={onCancel}
        autoFocus
      />
      <div className="comment-actions">
        <button disabled={busy || body.trim() === ''} onClick={() => void submit()}>
          {question ? t('comment.ask') : t('comment.comment')}
        </button>
        {canSuggest && (
          <SuggestButton editorRef={editor} seed={seed} disabled={busy} onInsert={setBody} />
        )}
        <label className="switch" title={t('comment.questionTitle')}>
          <input
            type="checkbox"
            checked={question}
            disabled={busy}
            onChange={(ev) => setQuestion(ev.target.checked)}
          />
          {t('comment.questionSwitch')}
        </label>
        <button className="ghost" disabled={busy} onClick={onCancel}>
          {t('common.cancel')}
        </button>
        <span className="hint" title={t('comment.sendHintTitle')}>
          {t('comment.sendHint')}
        </span>
      </div>
      {error && <div className="error inline">{error}</div>}
    </div>
  )
}
