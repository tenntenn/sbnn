import * as api from './api'
import type { Comment, Diff, Status, Verdict } from './types'
import { renderMarkdown, type PreviewAssets } from './markdown'
import { renderNotebook } from './notebook'
import { buildPrompt } from './prompt'
import { suggestions } from './suggestion'

/** PreviewResult is either an embedded mo page or Markdown rendered here. */
export type PreviewResult =
  | {
      kind: 'frame'
      url: string
      moUrl: string
      path: string
      source: string
      complete: boolean
    }
  | {
      kind: 'html'
      html: string
      path: string
      source: string
      complete: boolean
    }

export interface GroupData {
  diffs: Diff[]
  comments: Comment[]
  status: Status | null
  /** reviewedAt is when the review was last submitted, if it was. */
  reviewedAt?: string
  /** reviewVerdict is what that review decided. */
  reviewVerdict?: Verdict
  /** reviewed is whether that verdict still covers what the page shows: it
   * is false again once a diff arrives after the last review. A live page
   * reads that from the status summary; a static one has nothing else to
   * read it from. */
  reviewed?: boolean
}

/**
 * SbnnClient is what the UI talks to. The live client uses the sbnn server; the
 * static one is used by pages written with `sbnn export`, which have no server
 * behind them and keep comments in the browser.
 */
export interface SbnnClient {
  readonly isStatic: boolean
  /** exportedAt is set on static pages and tells when the diff was frozen. */
  readonly exportedAt?: string
  /** sbnnVersion is the sbnn that wrote a static page, where the page says. */
  readonly sbnnVersion?: string
  load(group: string): Promise<GroupData>
  addComment(group: string, comment: api.NewComment): Promise<void>
  updateComment(group: string, id: string, patch: api.CommentPatch): Promise<void>
  deleteComment(group: string, id: string): Promise<void>
  deleteDiff(group: string, diffId: string): Promise<void>
  /** closeReview drops the whole review: its diffs, comments and hooks. */
  closeReview(group: string): Promise<void>
  prompt(group: string): Promise<string>
  /** submitReview tells everyone waiting that the review is done. It only
   * exists where there is a server to tell. */
  submitReview(group: string, note: string, verdict: Verdict): Promise<void>
  /** preview returns the mo page for a file, embedded or linked. */
  preview(group: string, diffId: string, fileId: string): Promise<PreviewResult>
  /** previewMarkdown renders the Markdown in this page instead, which is
   * what a window too narrow for mo's own layout uses. */
  previewMarkdown(group: string, diffId: string, fileId: string): Promise<PreviewResult>
  /** previewNotebook renders a Jupyter notebook's cells. mo cannot show a
   * notebook at all, so this is the only way one is ever previewed. */
  previewNotebook(group: string, diffId: string, fileId: string): Promise<PreviewResult>
  /** imageSrc returns what an <img> should point at to show a file's
   * current image content, or undefined when there is nothing to show. It
   * is synchronous: the browser fetches the image itself once it is set as
   * a src, there is nothing for sbnn to await first. */
  imageSrc(group: string, diffId: string, fileId: string): string | undefined
  subscribe(group: string, onChange: () => void): () => void
}

/** StaticPayload is the data `sbnn export` embeds into the page. */
export interface StaticPayload {
  version: number
  /** sbnnVersion is the version of sbnn that wrote the page. */
  sbnnVersion?: string
  /**
   * saVersion is what sbnnVersion was called before the tool was renamed from
   * sa to sbnn. Pages already written carry it and are still read, so it stays
   * here; new pages are not expected to use it.
   *
   * @deprecated Read through payloadSbnnVersion; prefer sbnnVersion.
   */
  saVersion?: string
  generatedAt: string
  group: string
  diffs: Diff[]
  comments: Comment[]
  /** reviewedAt, reviewNote and reviewVerdict say how the review ended, and
   * are absent when it was never submitted. They carry the same names the
   * live API sends, so a frozen review reads exactly like a live one.
   * Payload version 2 is where they start meaning that: in a version 1 page
   * an absent verdict could equally mean "not reviewed" and "exported by a
   * binary that did not carry the verdict". */
  reviewedAt?: string
  reviewNote?: string
  reviewVerdict?: Verdict
  /** reviewed is false again once a diff arrived after that review, the way
   * the live status summary reports it. */
  reviewed?: boolean
  previews: Record<
    string,
    {
      content: string
      source: string
      complete: boolean
      path?: string
      /** assets is the images the Markdown points at, frozen into the page
       * as data URLs - or, where one was too heavy to carry, the reason it
       * was not. There is no server behind an exported page to fetch a
       * "diagram.png" from. */
      assets?: PreviewAssets
    }
  >
  images: Record<string, { dataUrl: string; path?: string }>
}

/**
 * payloadSbnnVersion reports which sbnn wrote the page. The field was called
 * saVersion before the tool was renamed, and a page carries whichever name the
 * sbnn that exported it wrote, so both are accepted with the current one
 * winning. An empty string is treated as absent: the writer omits the field
 * rather than emitting one, so an empty value only turns up in a page that was
 * edited by hand, and falling through is friendlier than returning "".
 */
export function payloadSbnnVersion(
  payload: Pick<StaticPayload, 'sbnnVersion' | 'saVersion'>,
): string | undefined {
  return payload.sbnnVersion || payload.saVersion || undefined
}

declare global {
  interface Window {
    __SBNN_DATA__?: StaticPayload
  }
}

function createLiveClient(): SbnnClient {
  return {
    isStatic: false,
    async load(group) {
      const [g, status] = await Promise.all([api.getGroup(group), api.getStatus()])
      return {
        diffs: g.diffs ?? [],
        comments: g.comments ?? [],
        reviewedAt: g.reviewedAt,
        reviewVerdict: g.reviewVerdict,
        status,
      }
    },
    async addComment(group, comment) {
      await api.addComment(group, comment)
    },
    async updateComment(group, id, patch) {
      await api.updateComment(group, id, patch)
    },
    async deleteComment(group, id) {
      await api.deleteComment(group, id)
    },
    async deleteDiff(group, diffId) {
      await api.deleteDiff(group, diffId)
    },
    async closeReview(group) {
      await api.deleteGroup(group)
    },
    prompt(group) {
      return api.getPrompt(group)
    },
    async submitReview(group, note, verdict) {
      await api.submitReview(group, note, verdict)
    },
    async preview(group, diffId, fileId) {
      const p = await api.getPreview(group, diffId, fileId)
      return {
        kind: 'frame',
        url: p.url,
        moUrl: p.moUrl,
        path: p.path,
        source: p.source,
        complete: p.complete,
      }
    },
    async previewMarkdown(group, diffId, fileId) {
      const file = await api.getFileContent(group, diffId, fileId)
      return {
        kind: 'html',
        html: renderMarkdown(file.content, file.assets),
        path: file.path,
        source: file.source,
        complete: file.complete,
      }
    },
    async previewNotebook(group, diffId, fileId) {
      const file = await api.getFileContent(group, diffId, fileId)
      return {
        kind: 'html',
        html: renderNotebook(file.content),
        path: file.path,
        source: file.source,
        complete: file.complete,
      }
    },
    imageSrc(group, diffId, fileId) {
      return api.imageURL(group, diffId, fileId)
    },
    subscribe: api.subscribe,
  }
}

/**
 * A static page has nowhere but localStorage to keep the reviewer's comments,
 * and unlike a pane width those comments are the work itself. storage.ts
 * swallows every failure by design, which is right for a setting and wrong
 * here: a full quota, a private window or blocked site data would let someone
 * write a page of review and lose all of it without a word. The static client
 * therefore reads and writes through the path below, which says so instead.
 *
 * The three troubles are told apart because they are not the same news, and
 * one of them must never stand in for another:
 *
 *   unreachable - the browser refuses the storage object itself, so nothing
 *                 will be saved. Known at load, before there is anything to
 *                 copy.
 *   unreadable  - a stored entry could not be parsed. Writing still works, so
 *                 the review carries on saving from here; what is gone is
 *                 whatever that entry held.
 *   unsaved     - a write was refused. This is the one that loses work that is
 *                 already on the screen.
 */
type StorageTrouble = 'unreachable' | 'unreadable' | 'unsaved'

const storageTrouble: Record<StorageTrouble, { lead: string; advice: string }> = {
  unreachable: {
    lead: 'This page cannot reach browser storage, so your comments will not be saved.',
    advice: ' Use "Copy prompt" before closing the tab to keep whatever you write here.',
  },
  unreadable: {
    lead: 'Comments saved on this page earlier could not be read back, so this is the review as it was exported.',
    advice: ' What you write from here on is saved again, over the entry that could not be read.',
  },
  unsaved: {
    lead: 'This browser refused to save your comments (private window, blocked site data, or a full quota).',
    advice: ' Use "Copy prompt" now to keep a copy: nothing you write here will survive this tab.',
  },
}

/**
 * spoken remembers which of the three has been said, one flag each.
 *
 * One flag for all of them would let the least urgent silence the most: a
 * page with site data blocked warns while it loads, when there is nothing to
 * copy yet, and a reviewer who dismisses that would then write comments and
 * have every refused write pass without a word - the very thing this is here
 * to prevent.
 */
const spoken: Record<StorageTrouble, boolean> = {
  unreachable: false,
  unreadable: false,
  unsaved: false,
}

/** warnStorage reports, once per trouble, what the page cannot keep. */
function warnStorage(trouble: StorageTrouble, err: unknown): void {
  if (spoken[trouble]) return
  spoken[trouble] = true
  console.error(`sbnn: ${storageTrouble[trouble].lead}`, err)
  showStorageBanner(trouble)
}

/** banner is the one bar on screen, so a second warning replaces the first
 * rather than stacking on top of it. */
let banner: HTMLElement | null = null

/**
 * showStorageBanner puts the warning on the page from outside React: the
 * static client is constructed before anything mounts and does not own App.
 * styles.css is not extended for it either, so the banner carries its own
 * style and borrows the app's custom properties, which are the ones that
 * exist: --danger-fg and --danger-bg are defined for both themes, a bare
 * --danger is not, and asking for a property nobody defines had the fallback
 * paint dark red text on a dark background.
 */
function showStorageBanner(trouble: StorageTrouble): void {
  if (typeof document === 'undefined' || !document.body) return

  const bar = document.createElement('div')
  bar.setAttribute('role', 'alert')
  bar.style.cssText = [
    'position: fixed',
    // Anchored to the bottom: the advice is to press "Copy prompt", which sits
    // in the header, so the warning must not be covering it.
    'inset: auto 0 0 0',
    'z-index: 2147483647',
    'display: flex',
    'gap: 0.75rem',
    'align-items: flex-start',
    'padding: 0.75rem 1rem',
    'background: var(--danger-bg, #ffebe9)',
    'color: var(--fg, #1f2328)',
    'border-top: 2px solid var(--danger-fg, #a40e26)',
    'box-shadow: 0 -1px 4px rgb(0 0 0 / 0.25)',
    'line-height: 1.4',
  ].join(';')

  const text = document.createElement('div')
  text.style.cssText = 'flex: 1'
  const lead = document.createElement('strong')
  lead.style.cssText = 'color: var(--danger-fg, #a40e26)'
  lead.textContent = storageTrouble[trouble].lead
  const advice = document.createElement('span')
  // Each trouble carries its own advice: telling a reviewer whose writes are
  // still being saved that nothing will survive the tab is its own way of
  // losing their trust, and the next warning that matters.
  advice.textContent = storageTrouble[trouble].advice
  text.append(lead, advice)

  const close = document.createElement('button')
  close.type = 'button'
  close.textContent = 'Dismiss'
  close.style.cssText = [
    'flex: none',
    'padding: 0.125rem 0.5rem',
    'background: var(--bg, transparent)',
    'color: var(--fg-muted, #5a626c)',
    'border: 1px solid var(--border, #d0d7de)',
    'border-radius: 4px',
    'cursor: pointer',
    'font: inherit',
  ].join(';')
  close.addEventListener('click', () => {
    bar.remove()
    if (banner === bar) banner = null
  })

  bar.append(text, close)
  banner?.remove()
  banner = bar
  document.body.appendChild(bar)
}

function createStaticClient(data: StaticPayload): SbnnClient {
  const storageKey = `sbnn:comments:${data.group}:${data.generatedAt}`
  const listeners = new Set<() => void>()

  // Once the browser has refused a write, the comments live here for the rest
  // of the page view. They are not saved and the banner says so, but they stay
  // on screen and "Copy prompt" still carries them, which is the whole point of
  // telling the reviewer to run it.
  let memory: Comment[] | null = null

  const read = (): Comment[] => {
    if (memory !== null) return memory
    let stored: string | null = null
    try {
      stored = window.localStorage.getItem(storageKey)
    } catch (err) {
      warnStorage('unreachable', err)
      return data.comments ?? []
    }
    if (stored) {
      try {
        return JSON.parse(stored) as Comment[]
      } catch (err) {
        // Something else wrote there, or the entry was truncated. Falling back
        // to the exported comments quietly would drop the reviewer's work out
        // from under them, so it is said out loud - but as its own trouble:
        // storage is answering, so what comes next is saved.
        warnStorage('unreadable', err)
      }
    }
    return data.comments ?? []
  }

  const write = (comments: Comment[]) => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(comments))
      memory = null
    } catch (err) {
      memory = comments
      warnStorage('unsaved', err)
    }
    listeners.forEach((fn) => fn())
  }

  let nextID = 1
  const newID = () => {
    const existing = new Set(read().map((c) => c.id))
    while (existing.has(`local-${nextID}`)) nextID++
    return `local-${nextID++}`
  }

  return {
    isStatic: true,
    exportedAt: data.generatedAt,
    sbnnVersion: payloadSbnnVersion(data),
    async load() {
      return {
        diffs: data.diffs ?? [],
        comments: read(),
        status: null,
        reviewedAt: data.reviewedAt,
        reviewVerdict: data.reviewVerdict,
        reviewed: data.reviewed,
      }
    },
    async addComment(group, comment) {
      const now = new Date().toISOString()
      const created: Comment = {
        id: newID(),
        group,
        diffId: comment.diffId,
        fileId: comment.fileId,
        path: comment.path,
        side: comment.side,
        startLine: comment.startLine,
        endLine: comment.endLine,
        body: comment.body,
        snippet: comment.snippet,
        suggestions: suggestions(comment.body),
        question: comment.question ?? false,
        resolved: false,
        createdAt: now,
        updatedAt: now,
      }
      write([...read(), created])
    },
    async updateComment(_group, id, patch) {
      write(
        read().map((c) =>
          c.id === id
            ? {
                ...c,
                body: patch.body ?? c.body,
                suggestions: suggestions(patch.body ?? c.body),
                question: patch.question ?? c.question,
                resolved: patch.resolved ?? c.resolved,
                updatedAt: new Date().toISOString(),
              }
            : c,
        ),
      )
    },
    async deleteComment(_group, id) {
      write(read().filter((c) => c.id !== id))
    },
    async deleteDiff() {
      throw new Error('an exported page cannot drop a diff')
    },
    async closeReview() {
      throw new Error('an exported page has no review to close')
    },
    async prompt(group) {
      // The verdict decides what the comments mean, so an agent handed this
      // text has to be told it the way `sbnn comments` tells it.
      return buildPrompt(group, data.diffs ?? [], read(), {
        reviewedAt: data.reviewedAt,
        reviewNote: data.reviewNote,
        reviewVerdict: data.reviewVerdict,
      })
    },
    async submitReview() {
      throw new Error('an exported page has no server to submit the review to')
    },
    async preview(_group, diffId, fileId) {
      const entry = data.previews?.[`${diffId}:${fileId}`]
      if (!entry) throw new Error('this page carries no preview for that file')
      return {
        kind: 'html',
        html: renderMarkdown(entry.content, entry.assets),
        path: entry.path ?? '',
        source: entry.source,
        complete: entry.complete,
      }
    },
    async previewMarkdown(group, diffId, fileId) {
      // An exported page renders its own Markdown either way.
      return this.preview(group, diffId, fileId)
    },
    async previewNotebook(_group, diffId, fileId) {
      const entry = data.previews?.[`${diffId}:${fileId}`]
      if (!entry) throw new Error('this page carries no preview for that file')
      return {
        kind: 'html',
        html: renderNotebook(entry.content),
        path: entry.path ?? '',
        source: entry.source,
        complete: entry.complete,
      }
    },
    imageSrc(_group, diffId, fileId) {
      return data.images?.[`${diffId}:${fileId}`]?.dataUrl
    },
    subscribe(_group, onChange) {
      listeners.add(onChange)
      return () => listeners.delete(onChange)
    },
  }
}

export const client: SbnnClient = window.__SBNN_DATA__
  ? createStaticClient(window.__SBNN_DATA__)
  : createLiveClient()

export type { NewComment } from './api'
