import type { PreviewAssets } from './markdown'
import type { Comment, Group, Preview, Status, Verdict } from './types'

/** groupFromLocation reads the group name out of the URL path. */
export function groupFromLocation(): string {
  const name = window.location.pathname.replace(/^\/+|\/+$/g, '')
  return name === '' ? 'default' : name
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, init)
  if (!resp.ok) {
    const text = (await resp.text()).trim()
    throw new Error(text || resp.statusText)
  }
  return (await resp.json()) as T
}

export function getStatus(): Promise<Status> {
  return request<Status>('/_/api/status')
}

export function getGroup(group: string): Promise<Group> {
  return request<Group>(`/_/api/groups/${encodeURIComponent(group)}`)
}

export function getPreview(group: string, diffId: string, fileId: string): Promise<Preview> {
  return request<Preview>(
    `/_/api/groups/${encodeURIComponent(group)}/diffs/${encodeURIComponent(diffId)}` +
      `/files/${encodeURIComponent(fileId)}/preview`,
  )
}

/** imageURL is what an <img> should be pointed at to show a file's current
 * image content. Unlike the other endpoints here, nothing fetches it: the
 * browser does that itself once it is set as a src. */
export function imageURL(group: string, diffId: string, fileId: string): string {
  return (
    `/_/api/groups/${encodeURIComponent(group)}/diffs/${encodeURIComponent(diffId)}` +
    `/files/${encodeURIComponent(fileId)}/image`
  )
}

export interface FileContent {
  path: string
  source: 'worktree' | 'reconstructed'
  complete: boolean
  content: string
  /** assets is where the images this file points at really are, since a
   * relative src in the preview resolves against the server root instead. */
  assets?: PreviewAssets
}

export function getFileContent(
  group: string,
  diffId: string,
  fileId: string,
): Promise<FileContent> {
  return request<FileContent>(
    `/_/api/groups/${encodeURIComponent(group)}/diffs/${encodeURIComponent(diffId)}` +
      `/files/${encodeURIComponent(fileId)}/content`,
  )
}

export interface NewComment {
  diffId: string
  fileId: string
  path: string
  side: 'new' | 'old'
  startLine: number
  endLine: number
  body: string
  snippet: string
  question?: boolean
}

/** CommentPatch is what can be edited on an existing comment. */
export interface CommentPatch {
  body?: string
  resolved?: boolean
  question?: boolean
}

export function addComment(group: string, comment: NewComment): Promise<Comment> {
  return request<Comment>(`/_/api/groups/${encodeURIComponent(group)}/comments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(comment),
  })
}

export function updateComment(group: string, id: string, patch: CommentPatch): Promise<Comment> {
  return request<Comment>(`/_/api/groups/${encodeURIComponent(group)}/comments/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })
}

export async function deleteComment(group: string, id: string): Promise<void> {
  const resp = await fetch(
    `/_/api/groups/${encodeURIComponent(group)}/comments/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText)
}

export async function deleteGroup(group: string): Promise<void> {
  const resp = await fetch(`/_/api/groups/${encodeURIComponent(group)}`, { method: 'DELETE' })
  if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText)
}

export async function deleteDiff(group: string, diffId: string): Promise<void> {
  const resp = await fetch(
    `/_/api/groups/${encodeURIComponent(group)}/diffs/${encodeURIComponent(diffId)}`,
    { method: 'DELETE' },
  )
  if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText)
}

export function submitReview(group: string, note: string, verdict: Verdict): Promise<Group> {
  return request<Group>(`/_/api/groups/${encodeURIComponent(group)}/review`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ note, verdict }),
  })
}

export async function getPrompt(group: string): Promise<string> {
  const resp = await fetch(`/_/api/groups/${encodeURIComponent(group)}/prompt`)
  if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText)
  return await resp.text()
}

/**
 * subscribe listens to server sent events and calls onChange whenever the
 * given group changed. It returns an unsubscribe function.
 *
 * Events are fire-and-forget: the broker keeps no backlog and stamps no `id:`,
 * so everything published while the stream was down is simply gone. An
 * EventSource reconnects by itself - after a server restart, after a laptop
 * wakes, after any network blip - and a page that listened for messages only
 * would come back attached to a live stream while still showing whatever it
 * held before, with nothing left to tell it otherwise. So a freshly opened
 * connection is itself the signal to refetch. That fires on the first connect
 * too, which costs one duplicate load and in exchange makes "the page is
 * stale forever" unreachable; it is not worth optimising away.
 */
export function subscribe(group: string, onChange: () => void): () => void {
  const source = new EventSource('/_/events')
  source.onopen = () => {
    onChange()
  }
  // EventSource retries on its own, so an error is a gap rather than an end:
  // calling close() here is precisely what would make the staleness permanent.
  // The reconnect that follows lands in onopen above and takes the page with
  // it; the log is here so a stream that never comes back is visible.
  source.onerror = () => {
    console.debug('sbnn: event stream lost, reconnecting')
  }
  source.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data) as { type?: string; group?: string }
      // "change" is a new diff or comment; "review" is the Submit button,
      // which another tab has to hear about too - it is what turns the
      // page from open to reviewed.
      const interesting = data.type === 'change' || data.type === 'review'
      if (interesting && (!data.group || data.group === group)) onChange()
    } catch {
      onChange()
    }
  }
  return () => source.close()
}
