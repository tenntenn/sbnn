import { useEffect, useMemo, useState, type CSSProperties, type RefObject } from 'react'
import type { Comment, Diff, FileDiff, Status } from '../types'
import { filePath } from '../types'
import { client } from '../client'
import { readEnumSetting, writeSetting } from '../storage'
import { Icon } from './Icon'
import { sectionKey } from '../sectionKey'
import { MAX_SCANNED_LINES, SEARCH_DEBOUNCE_MS, matchSummary, searchDiffs } from '../search'

/** Layout is how the rounds are shown: stacked, or one tab at a time. */
type Layout = 'list' | 'tabs'

const LAYOUT_KEY = 'sbnn.sidebar.layout'

// A tab is two controls, not one: picking the round, and dropping it. They
// are siblings because a button may not hold another button - nested that
// way the remove control was invalid markup and no keyboard could reach it.
// The wrapper keeps the .diff-tab box, and these hand the buttons back the
// padding and the chrome they used to inherit from it. Inline rather than in
// styles.css, which several other changes are sitting on.
const TAB_BOX: CSSProperties = { padding: '0 var(--space-lg) 0 0' }

const TAB_BODY: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 'var(--space-sm)',
  padding: 'var(--space-xs) 0 var(--space-xs) var(--space-lg)',
  margin: 0,
  border: 0,
  background: 'transparent',
  color: 'inherit',
  font: 'inherit',
  whiteSpace: 'nowrap',
  cursor: 'pointer',
}

const TAB_REMOVE: CSSProperties = {
  border: 0,
  background: 'transparent',
  font: 'inherit',
  cursor: 'pointer',
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
  /** query narrows the list to the files that contain it, by path or by
   * what the hunks say. */
  query: string
  onQuery: (query: string) => void
  searchRef?: RefObject<HTMLInputElement | null>
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
    () => readEnumSetting<Layout>(LAYOUT_KEY, ['list', 'tabs'], 'list'),
  )
  // Which rounds are shut is about this review rather than about this reader,
  // so it is deliberately not remembered across a reload - see the rule in
  // App.tsx.
  const [shutRounds, setShutRounds] = useState<Set<string>>(() => new Set())
  const [tab, setTab] = useState<string | null>(null)

  useEffect(() => {
    writeSetting(LAYOUT_KEY, layout)
  }, [layout])

  // The box shows every keystroke; the search waits for the reader to stop.
  // Clearing is not typing, so it is answered at once - a reader who wants
  // the whole list back should not watch it arrive a tenth of a second late.
  const [settled, setSettled] = useState(query)
  useEffect(() => {
    if (query === '') {
      setSettled('')
      return
    }
    const timer = window.setTimeout(() => setSettled(query), SEARCH_DEBOUNCE_MS)
    return () => window.clearTimeout(timer)
  }, [query])

  // Same query, same rounds, same answer: the walk over every hunk line
  // happens once, not on each render of the list it feeds.
  const results = useMemo(() => searchDiffs(diffs, settled), [diffs, settled])

  const shownByDiff = useMemo(() => {
    const byDiff = new Map<string, FileDiff[]>()
    for (const diff of diffs) {
      byDiff.set(
        diff.id,
        results.active
          ? diff.files.filter((f) => results.matches.has(sectionKey(diff.id, f.id)))
          : diff.files,
      )
    }
    return byDiff
  }, [diffs, results])

  const shown = (diff: Diff): FileDiff[] => shownByDiff.get(diff.id) ?? []
  const total = diffs.reduce((n, d) => n + d.files.length, 0)
  const found = results.active ? results.files : total

  const searching = results.active

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

  // Going to a file is the parent's business - it owns which pane is up and
  // where the stack is scrolled. The scrollIntoView afterwards is for the
  // section the parent just mounted: on a match found in the content the
  // reader asked to be taken to a specific file, and the node may not have
  // existed when the click was handled.
  const jumpTo = (diffId: string, fileId: string) => {
    onSelect(diffId, fileId)
    const key = sectionKey(diffId, fileId)
    window.setTimeout(() => {
      document.getElementById(key)?.scrollIntoView({ block: 'start' })
    }, 50)
  }

  // Enter takes you to the first file still standing, which is the whole
  // point of typing into a list. It answers what is in the box now, not what
  // the debounce has caught up with, so a fast typist is not sent to the
  // wrong file.
  const openFirst = () => {
    const now = settled === query ? results : searchDiffs(diffs, query)
    for (const diff of diffs) {
      const first = now.active
        ? diff.files.find((f) => now.matches.has(sectionKey(diff.id, f.id)))
        : diff.files[0]
      if (first) {
        jumpTo(diff.id, first.id)
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
            placeholder="Search paths and lines ( / )"
            aria-label="Search files by path and by what the diff lines say"
            onChange={(ev) => onQuery(ev.target.value)}
            onKeyDown={(ev) => {
              if (ev.key === 'Escape') {
                if (query === '') ev.currentTarget.blur()
                else onQuery('')
              }
              if (ev.key === 'Enter') openFirst()
            }}
          />
          {searching && (
            <span className="hint">
              {found} of {total}
              {results.lines > 0 && `, ${results.lines} line${results.lines === 1 ? '' : 's'}`}
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

      {layout === 'tabs' && diffs.length > 1 && (
        <div className="diff-tabs" role="tablist">
          {tabbed.map((diff) => (
            <div
              key={diff.id}
              className={`diff-tab${diff.id === activeTab ? ' active' : ''}`}
              style={TAB_BOX}
            >
              <button type="button"
                role="tab"
                aria-selected={diff.id === activeTab}
                title={new Date(diff.createdAt).toLocaleString()}
                style={TAB_BODY}
                onClick={() => {
                  setTab(diff.id)
                  const first = shown(diff)[0]
                  if (first) onSelect(diff.id, first.id)
                }}
              >
                {diff.title}
                {searching && tabbed.length > 1 && (
                  <span className="hint" title="files matching in this round">
                    {shown(diff).length}
                  </span>
                )}
                {roundComments(diff) > 0 && (
                  <span className="badge sm warn">{roundComments(diff)}</span>
                )}
              </button>
              {!client.isStatic && diff.id === activeTab && (
                <button type="button"
                  className="tab-remove"
                  style={TAB_REMOVE}
                  aria-label="Remove this round"
                  title="Remove this round"
                  onClick={() => {
                    if (!window.confirm(removeRoundQuestion(diff.title, roundSize(comments, diff.id)))) return
                    void client.deleteDiff(group, diff.id).then(onChanged, reportRemoveFailure(diff.title))
                  }}
                >
                  <Icon name="close" small />
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {results.truncated && (
        <p className="empty">
          Stopped reading lines after {MAX_SCANNED_LINES.toLocaleString()}; paths are still
          searched in full. Narrow the search to see the rest.
        </p>
      )}

      {searching && found === 0 && <p className="empty">Nothing matches that.</p>}

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
                  if (!window.confirm(removeRoundQuestion(diff.title, roundSize(comments, diff.id)))) return
                  void client.deleteDiff(group, diff.id).then(onChanged, reportRemoveFailure(diff.title))
                }}
              >
                <Icon name="close" small />
              </button>
            )}
          </div>
          <ul className="file-list" hidden={isShut(diff)}>
            {shown(diff).map((file) => {
              const key = sectionKey(diff.id, file.id)
              const active = activeKey === key
              const count = commentCount(diff.id, file.id)
              // Where the file was hit. A file found only by its content
              // would otherwise look like a path that matched, and the
              // reader would go looking in the name for a word that is in
              // the code.
              const hit = results.matches.get(key)
              // A folded file is still listed - the point is that it is out
              // of the way, not out of sight.
              const folded = Boolean(file.folded) && count === 0
              return (
                <li key={file.id}>
                  <button
                    className={`file-item${active ? ' active' : ''}${folded ? ' folded' : ''}`}
                    onClick={() => jumpTo(diff.id, file.id)}
                  >
                    <span className={`dot status-${file.status}`} title={file.status} />
                    <span className="file-path" title={filePath(file)}>
                      {/* The box clips from the left; the path inside it
                          reads in its own direction. See .file-path. */}
                      <bdi>{filePath(file)}</bdi>
                    </span>
                    {hit && (
                      <span className="hint" title="Go to this file">
                        {'\u2014'} {matchSummary(hit)}
                      </span>
                    )}
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

/** roundSize counts what a round takes with it: every comment on it, the
 * resolved ones included, since the store deletes them all. */
function roundSize(comments: Comment[], diffId: string): number {
  return comments.filter((c) => c.diffId === diffId).length
}

/** removeRoundQuestion asks before a round goes.
 *
 * Removing one deletes the diff and every comment on it, at once and for
 * good; the two ways in are both a small close icon, one of them on a tab
 * the reader clicks to switch rounds. Closing a review - the page's other
 * destructive control - already asks, and counts what goes, so this asks
 * the same way. */
export function removeRoundQuestion(title: string, comments: number): string {
  if (comments === 0) return `Remove ${title}? This cannot be undone.`
  return `Remove ${title}? ${comments} comment(s) on it will be deleted too.`
}

/** reportRemoveFailure says so when the removal did not happen. The round
 * stays on screen either way, and silence there reads as success. */
function reportRemoveFailure(title: string): (err: unknown) => void {
  return (err) =>
    window.alert(`Could not remove ${title}: ${err instanceof Error ? err.message : String(err)}`)
}
