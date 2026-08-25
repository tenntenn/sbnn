import { useRef, useState } from 'react'
import type { Comment } from '../types'
import { client } from '../client'
import { insertSuggestion, originalLines, parseBody, suggestionBlock, suggestions } from '../suggestion'

interface ThreadProps {
  group: string
  comments: Comment[]
  onChanged: () => void
}

/** CommentThread renders the comments anchored to one line range. */
export function CommentThread({ group, comments, onChanged }: ThreadProps) {
  return (
    <div className="thread">
      {comments.map((c) => (
        <CommentItem key={c.id} group={group} comment={c} onChanged={onChanged} />
      ))}
    </div>
  )
}

function rangeLabel(c: Pick<Comment, 'path' | 'side' | 'startLine' | 'endLine'>): string {
  const lines = c.endLine > c.startLine ? `${c.startLine}-${c.endLine}` : `${c.startLine}`
  return `${c.path}:${lines}${c.side === 'old' ? ' (old)' : ''}`
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
        <span>Suggested change — replaces {rangeLabel(comment)}</span>
        <button className="ghost" onClick={() => void copy()}>
          {copied ? 'Copied' : 'Copy'}
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
              <td className="code hint">(the lines are removed)</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

function CommentItem({
  group,
  comment,
  onChanged,
}: {
  group: string
  comment: Comment
  onChanged: () => void
}) {
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

  return (
    <div id={`comment-${comment.id}`} className={`comment${comment.resolved ? ' resolved' : ''}`}>
      <div className="comment-meta">
        {comment.author && <span className="badge author">{comment.author}</span>}
        <span className="comment-range">{rangeLabel(comment)}</span>
        {!editing && suggestions(comment.body).length > 0 && (
          <span className="badge">suggestion</span>
        )}
        {comment.question && <span className="badge question">question</span>}
        {comment.resolved && <span className="badge">resolved</span>}
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
              Save
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
              Cancel
            </button>
          </div>
        </>
      ) : (
        <>
          {segments.map((segment, i) =>
            segment.kind === 'text' ? (
              <div key={i} className="comment-body">
                {segment.text}
              </div>
            ) : (
              <SuggestedChange key={i} comment={comment} suggestion={segment.text} />
            ),
          )}
          <div className="comment-actions">
            <button
              className="ghost"
              disabled={busy}
              onClick={() =>
                run(() => client.updateComment(group, comment.id, { resolved: !comment.resolved }))
              }
            >
              {comment.resolved ? 'Reopen' : 'Resolve'}
            </button>
            <button className="ghost" disabled={busy} onClick={() => setEditing(true)}>
              Edit
            </button>
            <button
              className="ghost danger"
              disabled={busy}
              onClick={() => run(() => client.deleteComment(group, comment.id))}
            >
              Delete
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
}

function CommentEditor({ editorRef, value, onChange, onSubmit, onCancel, autoFocus }: EditorProps) {
  return (
    <textarea
      ref={editorRef}
      className="comment-input"
      autoFocus={autoFocus}
      rows={Math.max(3, Math.min(16, value.split('\n').length + 1))}
      placeholder="What should change here?"
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
    <button className="ghost" disabled={disabled} onClick={insert} title="Insert a suggestion block">
      Suggest a change
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
          {question ? 'Ask' : 'Comment'}
        </button>
        {canSuggest && (
          <SuggestButton editorRef={editor} seed={seed} disabled={busy} onInsert={setBody} />
        )}
        <label className="switch" title="It wants an answer, not a change">
          <input
            type="checkbox"
            checked={question}
            disabled={busy}
            onChange={(ev) => setQuestion(ev.target.checked)}
          />
          Question
        </label>
        <button className="ghost" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <span className="hint">⌘/Ctrl + Enter</span>
      </div>
      {error && <div className="error inline">{error}</div>}
    </div>
  )
}
