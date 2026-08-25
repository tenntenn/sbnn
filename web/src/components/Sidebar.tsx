import { useEffect, useState, type RefObject } from 'react'
import type { Comment, Diff, FileDiff, Status } from '../types'
import { filePath } from '../types'
import { client } from '../client'
import { readSetting, writeSetting } from '../storage'
import { Icon } from './Icon'
import { sectionKey } from '../sectionKey'

/** Layout is how the rounds are shown: stacked, or one tab at a time. */
type Layout = 'list' | 'tabs'

const LAYOUT_KEY = 'sbnn.sidebar.layout'

/**
 * matchesPath reports whether a path answers a search.
 *
 * Every whitespace-separated term has to appear somewhere in the path,
 * ignoring case - so "server go" and "internal/server" both find
 * internal/server/server.go. Nothing turns up that does not contain what
 * was typed. A looser match (the letters in order, anywhere)
 * would find more, and would also find things the reader did not ask for,
 * which in a list you are scanning is worse than finding nothing.
 */
export function matchesPath(path: string, query: string): boolean {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true
  const haystack = path.toLowerCase()
  return terms.every((term) => haystack.includes(term))
}

interface Props {
  /** width in pixels; 0 collapses the file list out of the way and null lets
   * it fill the space, which is what a phone does. */
  width: number | null
  group: string
  diffs: Diff[]
  comments: Comment[]
  status: Status | null
  /** activeKey is the section the reader is currently scrolled to (or, on a
   * phone, the one file shown); activeDiffId is which round it belongs to,
   * for bringing that round's tab forward. */
  activeKey: string | null
  activeDiffId: string | null
  onSelect: (diffId: string, fileId: string) => void
  onChanged: () => void
  /** query narrows the list to the paths that contain it. */
  query: string
  onQuery: (query: string) => void
  searchRef?: RefObject<HTMLInputElement | null>
  /** readKeys holds the diffId:fileId pairs the reader is done with, and
   * readCount is how many of them are still on the page - counted upstream,
   * where the whole file list is, rather than recounted per render here. */
  readKeys: Set<string>
  readCount: number
  onSetRead: (key: string, value: boolean) => void
  onMarkAllUnread: () => void
}

export function Sidebar({
  width,
  group,
  diffs,
  comments,
  status,
  activeKey,
  activeDiffId,
  onSelect,
  onChanged,
  query,
  onQuery,
  searchRef,
  readKeys,
  readCount,
  onSetRead,
  onMarkAllUnread,
}: Props) {
  const commentCount = (diffId: string, fileId: string) =>
    comments.filter((c) => c.diffId === diffId && c.fileId === fileId && !c.resolved).length

  // A shut round still says how much is waiting inside it.
  const roundComments = (diff: Diff): number =>
    comments.filter((c) => c.diffId === diff.id && !c.resolved).length

  // Rounds pile up: a review of four diffs is four headings and everything
  // under them. A round can be shut, and the whole list can be turned into
  // tabs, which shows one round at a time.
  const [layout, setLayout] = useState<Layout>(
    () => (readSetting(LAYOUT_KEY) === 'tabs' ? 'tabs' : 'list'),
  )
  const [shutRounds, setShutRounds] = useState<Set<string>>(() => new Set())
  const [tab, setTab] = useState<string | null>(null)

  useEffect(() => {
    writeSetting(LAYOUT_KEY, layout)
  }, [layout])

  const shown = (diff: Diff): FileDiff[] => diff.files.filter((f) => matchesPath(filePath(f), query))
  const total = diffs.reduce((n, d) => n + d.files.length, 0)
  const found = diffs.reduce((n, d) => n + shown(d).length, 0)

  const searching = query !== ''

  // A search is about the whole review, not about one round of it, so the
  // tabs are searched too: a round with nothing matching drops out of the
  // strip, and its count says how much it holds. Losing the tabs during a
  // search - which is what flattening them into a list did - takes the
  // reader out of the layout they chose the moment they look for
  // something.
  const tabbed = diffs.filter((d) => !searching || shown(d).length > 0)

  // The tab in front is the one holding the selected file, so picking a
  // file from anywhere - a keyboard step, a comment - brings its round
  // forward rather than leaving the reader on a tab that shows nothing.
  // When a search empties the current tab, the first one with a match
  // takes over, since a tab strip with nothing behind it is a dead end.
  const preferred = diffs.find((d) => d.id === activeDiffId)?.id ?? tab ?? diffs[0]?.id ?? null
  const activeTab =
    tabbed.some((d) => d.id === preferred) ? preferred : (tabbed[0]?.id ?? null)

  const visible = (diff: Diff): boolean => {
    if (shown(diff).length === 0) return false
    if (layout === 'tabs') return diff.id === activeTab
    return true
  }
  // A search opens every round it matched: a match hidden inside a shut
  // round is a match nobody sees.
  const isShut = (diff: Diff): boolean =>
    layout === 'list' && !searching && shutRounds.has(diff.id)

  const toggleRound = (id: string) =>
    setShutRounds((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  // Enter takes you to the first path still standing, which is the whole
  // point of typing into a list.
  const openFirst = () => {
    for (const diff of diffs) {
      const first = shown(diff)[0]
      if (first) {
        onSelect(diff.id, first.id)
        return
      }
    }
  }

  return (
    <aside
      className={`sidebar${width === 0 ? ' collapsed' : ''}${width === null ? ' fill' : ''}`}
      style={width === null ? undefined : { width }}
      aria-hidden={width === 0}
    >
      {diffs.length === 0 && <p className="empty">No diff yet.</p>}

      {total > 0 && (
        <div className="file-search">
          <Icon name="search" small />
          <input
            ref={searchRef}
            type="search"
            className="file-search-input"
            value={query}
            placeholder="Filter paths ( / )"
            aria-label="Filter files by path"
            onChange={(ev) => onQuery(ev.target.value)}
            onKeyDown={(ev) => {
              if (ev.key === 'Escape') {
                if (query === '') ev.currentTarget.blur()
                else onQuery('')
              }
              if (ev.key === 'Enter') openFirst()
            }}
          />
          {query !== '' && (
            <span className="hint">
              {found} of {total}
            </span>
          )}
          {diffs.length > 1 && (
            <div className="toggle sm">
              <button
                className={layout === 'list' ? 'active' : ''}
                onClick={() => setLayout('list')}
                title="Stack every round in one list"
              >
                list
              </button>
              <button
                className={layout === 'tabs' ? 'active' : ''}
                onClick={() => setLayout('tabs')}
                title="Show one round at a time"
              >
                tabs
              </button>
            </div>
          )}
        </div>
      )}

      {/* Where the reader got to, for the review as a whole. It sits above
          the rounds because that is the question it answers - "how much of
          this is left" - which is not a question about any one round. */}
      {total > 0 && (
        <div className="file-search">
          <Icon name="check" small />
          <span className="hint" style={{ flex: 1 }}>
            {readCount} of {total} read
          </span>
          {readCount > 0 && (
            <button
              className="ghost"
              onClick={onMarkAllUnread}
              title="Clear every read mark in this review"
            >
              <Icon name="refresh" small />
              Mark all unread
            </button>
          )}
        </div>
      )}

      {layout === 'tabs' && diffs.length > 1 && (
        <div className="diff-tabs" role="tablist">
          {tabbed.map((diff) => (
            <button
              key={diff.id}
              role="tab"
              aria-selected={diff.id === activeTab}
              className={`diff-tab${diff.id === activeTab ? ' active' : ''}`}
              title={new Date(diff.createdAt).toLocaleString()}
              onClick={() => {
                setTab(diff.id)
                const first = shown(diff)[0]
                if (first) onSelect(diff.id, first.id)
              }}
            >
              {diff.title}
              {searching && tabbed.length > 1 && (
                <span className="hint" title="paths matching in this round">
                  {shown(diff).length}
                </span>
              )}
              {roundComments(diff) > 0 && <span className="badge sm warn">{roundComments(diff)}</span>}
              {!client.isStatic && diff.id === activeTab && (
                <span
                  className="tab-remove"
                  role="button"
                  title="Remove this round"
                  onClick={(ev) => {
                    ev.stopPropagation()
                    void client.deleteDiff(group, diff.id).then(onChanged)
                  }}
                >
                  <Icon name="close" small />
                </span>
              )}
            </button>
          ))}
        </div>
      )}

      {searching && found === 0 && <p className="empty">No path contains that.</p>}

      {diffs.map((diff) => (
        <div className="diff-round" key={diff.id} hidden={!visible(diff)}>
          <div className="diff-round-header" hidden={layout === 'tabs'}>
            {layout === 'list' ? (
              <button
                className="diff-round-title as-button"
                title={new Date(diff.createdAt).toLocaleString()}
                aria-expanded={!isShut(diff)}
                onClick={() => toggleRound(diff.id)}
              >
                <span className="disclosure">
                  <Icon name={isShut(diff) ? 'chevron_right' : 'expand_more'} small />
                </span>
                {diff.title}
                <span className="hint">{shown(diff).length}</span>
                {roundComments(diff) > 0 && (
                  <span className="badge sm warn">{roundComments(diff)}</span>
                )}
              </button>
            ) : (
              <span className="diff-round-title" title={new Date(diff.createdAt).toLocaleString()}>
                {diff.title}
              </span>
            )}
            {!client.isStatic && (
              <button
                className="ghost danger"
                title="Remove this round"
                onClick={() => {
                  void client.deleteDiff(group, diff.id).then(onChanged)
                }}
              >
                <Icon name="close" small />
              </button>
            )}
          </div>
          <ul className="file-list" hidden={isShut(diff)}>
            {shown(diff).map((file) => {
              const fileKey = sectionKey(diff.id, file.id)
              const active = activeKey === fileKey
              const count = commentCount(diff.id, file.id)
              // A folded file is still listed - the point is that it is out
              // of the way, not out of sight.
              const folded = Boolean(file.folded) && count === 0
              const read = readKeys.has(fileKey)
              const toggleRead = () => onSetRead(fileKey, !read)
              return (
                <li key={file.id}>
                  <button
                    className={`file-item${active ? ' active' : ''}${folded ? ' folded' : ''}`}
                    onClick={() => onSelect(diff.id, file.id)}
                  >
                    {/* A span rather than a button: this sits inside the row's
                        own button, and a button inside a button is not
                        markup a browser will keep. The unread state is drawn
                        faintly rather than left blank so that the target is
                        there to aim at before it has been used. */}
                    <span
                      role="button"
                      tabIndex={0}
                      aria-pressed={read}
                      aria-label={read ? `Mark ${filePath(file)} unread` : `Mark ${filePath(file)} read`}
                      title={read ? 'Mark as unread' : 'Mark as read'}
                      style={{ display: 'inline-flex', opacity: read ? 1 : 0.25 }}
                      onClick={(ev) => {
                        ev.stopPropagation()
                        toggleRead()
                      }}
                      onKeyDown={(ev) => {
                        if (ev.key !== 'Enter' && ev.key !== ' ') return
                        ev.preventDefault()
                        ev.stopPropagation()
                        toggleRead()
                      }}
                    >
                      <Icon name="check" small />
                    </span>
                    <span className={`dot status-${file.status}`} title={file.status} />
                    <span
                      className="file-path"
                      title={filePath(file)}
                      style={read ? { opacity: 0.55 } : undefined}
                    >
                      {filePath(file)}
                    </span>
                    {folded && (
                      <span className="badge sm" title={file.foldReason}>
                        folded
                      </span>
                    )}
                    {file.isMarkdown && <span className="badge sm" title="Previewable with mo">md</span>}
                    {file.isImage && <span className="badge sm" title="Previewable as an image">img</span>}
                    {file.isNotebook && (
                      <span className="badge sm" title="Previewable as a Jupyter notebook">
                        ipynb
                      </span>
                    )}
                    {count > 0 && <span className="badge sm warn">{count}</span>}
                    <span className="stat add">+{file.additions}</span>
                    <span className="stat del">-{file.deletions}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      ))}

      {status && status.groups.some((g) => g.name !== group) && (
        <div className="groups">
          <div className="groups-title">Groups</div>
          <ul>
            {status.groups.map((g) => (
              <li key={g.name}>
                <a className={g.name === group ? 'active' : ''} href={g.url}>
                  {g.name}
                  <span className="hint">
                    {g.diffs} round(s){g.unresolved > 0 ? `, ${g.unresolved} open` : ''}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
    </aside>
  )
}
