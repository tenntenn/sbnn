import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { groupFromLocation } from './api'
import { client } from './client'
import { readSetting, writeSetting } from './storage'
import { isPreviewable, type Comment, type Diff, type FileDiff, type PreviewKind, type Status, type ViewMode, type Verdict } from './types'
import { DiffFileSection } from './components/DiffFileSection'
import { DiffStack, resolveFolded, type DiffStackHandle, type ScrollFraction } from './components/DiffStack'
import { Divider } from './components/Divider'
import { Icon } from './components/Icon'
import { PreviewFileSection } from './components/PreviewFileSection'
import { PreviewSelection } from './components/PreviewSelection'
import { PreviewStack } from './components/PreviewStack'
import { Sidebar } from './components/Sidebar'
import { clampRatio, SplitPane, SPLIT_DEFAULT } from './components/SplitPane'
import { useNarrowLayout } from './useMediaQuery'
import { plainKey, shortcuts, typingInto } from './shortcuts'
import { applyTheme, storedTheme, type Theme } from './theme'
import { sectionKey } from './sectionKey'

/** Pane names the three panes, which a phone shows one at a time. */
type Pane = 'files' | 'diff' | 'preview'

// The file list is a pane like the others: it can be dragged narrow, and
// pulling it past the snapping point puts it away entirely.
const SIDEBAR_DEFAULT = 280
const SIDEBAR_MAX = 720
const SIDEBAR_SNAP = 48
const SIDEBAR_STEP = 24
const SIDEBAR_KEY = 'sbnn.sidebar.width'
const SPLIT_KEY = 'sbnn.split'
const PREVIEW_KIND_KEY = 'sbnn.preview.renderer'

function storedSplitRatio(): number {
  const stored = readSetting(SPLIT_KEY)
  if (stored === null) return SPLIT_DEFAULT
  const ratio = Number(stored)
  if (!Number.isFinite(ratio) || ratio < 0 || ratio > 1) return SPLIT_DEFAULT
  return ratio === 0 || ratio === 1 ? ratio : clampRatio(ratio)
}

function storedSidebarWidth(): number {
  // An unset entry reads as null, which Number() would happily turn into a
  // collapsed sidebar, so the absence is checked before the value.
  const stored = readSetting(SIDEBAR_KEY)
  if (stored === null) return SIDEBAR_DEFAULT
  const width = Number(stored)
  return Number.isFinite(width) && width >= 0 && width <= SIDEBAR_MAX ? width : SIDEBAR_DEFAULT
}

function storedPreviewKind(): PreviewKind {
  return readSetting(PREVIEW_KIND_KEY) === 'mo' ? 'mo' : 'preview'
}

export function App() {
  const group = useMemo(() => (client.isStatic ? staticGroupName() : groupFromLocation()), [])
  const narrow = useNarrowLayout()
  const [diffs, setDiffs] = useState<Diff[]>([])
  const [comments, setComments] = useState<Comment[]>([])
  const [reviewedAt, setReviewedAt] = useState<string | null>(null)
  const [reviewVerdict, setReviewVerdict] = useState<Verdict | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  // activeKey is the file the reader is currently looking at: on a phone
  // the one file shown, on a wide screen whichever the diff pane has been
  // scrolled to. It is a diffId:fileId pair, not a bare fileId - a fileId is
  // only unique within the round it came from, so two rounds that both
  // touch the same path as their Nth file share one.
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [foldOverrides, setFoldOverrides] = useState<Map<string, boolean>>(() => new Map())
  const [viewModeOverrides, setViewModeOverrides] = useState<Map<string, ViewMode>>(() => new Map())
  // viewModeDefault is every file's view mode until its own toggle says
  // otherwise; null respects each file's own server-picked default (added
  // files unified, most modified files split) rather than forcing one.
  const [viewModeDefault, setViewModeDefault] = useState<ViewMode | null>(null)
  const [scrollFraction, setScrollFraction] = useState<ScrollFraction | null>(null)
  const [splitRatio, setSplitRatio] = useState(storedSplitRatio)
  const [pane, setPane] = useState<Pane>('diff')
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [closing, setClosing] = useState(false)
  const [reviewNote, setReviewNote] = useState<string | null>(null)
  const [help, setHelp] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(storedSidebarWidth)
  const [theme, setTheme] = useState<Theme>(storedTheme)
  const [query, setQuery] = useState('')
  const [previewKind, setPreviewKind] = useState<PreviewKind>(storedPreviewKind)
  // Scrolling the diff moves the preview with it, to the same file and the
  // same fraction into that file's own section, rather than by line: the
  // two documents do not agree on lines, and pretending they do lands the
  // reader in the wrong place with more confidence. It is off the moment
  // the reader says so, and the reader says so simply by scrolling the
  // preview themselves.
  const [syncScroll, setSyncScroll] = useState(true)
  const bodyRef = useRef<HTMLDivElement>(null)
  const diffScrollRef = useRef<HTMLDivElement>(null)
  const previewScrollRef = useRef<HTMLDivElement>(null)
  const diffStackRef = useRef<DiffStackHandle>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const settingsRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  // A menu left open is a click away from closing, wherever that click lands.
  useEffect(() => {
    if (!settingsOpen) return
    const onPointerDown = (ev: PointerEvent) => {
      if (!settingsRef.current?.contains(ev.target as Node)) setSettingsOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [settingsOpen])

  useEffect(() => {
    writeSetting(SIDEBAR_KEY, String(sidebarWidth))
  }, [sidebarWidth])

  useEffect(() => {
    writeSetting(PREVIEW_KIND_KEY, previewKind)
  }, [previewKind])

  const resizeSidebar = useCallback((clientX: number) => {
    const rect = bodyRef.current?.getBoundingClientRect()
    if (!rect) return
    const next = clientX - rect.left
    setSidebarWidth(next < SIDEBAR_SNAP ? 0 : Math.min(SIDEBAR_MAX, next))
  }, [])

  const toggleSidebar = () => setSidebarWidth((w) => (w === 0 ? SIDEBAR_DEFAULT : 0))

  const focusSearch = useCallback(() => {
    setSidebarWidth((w) => (w === 0 ? SIDEBAR_DEFAULT : w))
    if (narrow) setPane('files')
    window.setTimeout(() => searchRef.current?.focus(), 0)
  }, [narrow])

  useEffect(() => {
    writeSetting(SPLIT_KEY, String(splitRatio))
  }, [splitRatio])

  const reload = useCallback(async () => {
    try {
      const data = await client.load(group)
      setDiffs(data.diffs)
      setComments(data.comments)
      setReviewedAt(data.reviewedAt ?? null)
      setReviewVerdict(data.reviewVerdict ?? null)
      setStatus(data.status)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [group])

  useEffect(() => {
    void reload()
    return client.subscribe(group, () => {
      void reload()
    })
  }, [group, reload])

  // Every file, in every round, keyed by the pair a comment or an override
  // already has to use.
  const filesByKey = useMemo(() => {
    const map = new Map<string, { diff: Diff; file: FileDiff }>()
    for (const d of diffs) {
      for (const f of d.files) map.set(sectionKey(d.id, f.id), { diff: d, file: f })
    }
    return map
  }, [diffs])

  const flatKeys = useMemo(
    () => diffs.flatMap((d) => d.files.map((f) => sectionKey(d.id, f.id))),
    [diffs],
  )

  // Keep a file active: the newest diff is what the reviewer just sent. On
  // a wide screen this is only the starting point - scrolling the diff pane
  // takes over from here (see DiffStack's onActiveChange below).
  useEffect(() => {
    if (diffs.length === 0) {
      setActiveKey(null)
      return
    }
    setActiveKey((current) => {
      if (current && diffs.some((d) => d.files.some((f) => sectionKey(d.id, f.id) === current))) {
        return current
      }
      const last = diffs[diffs.length - 1]
      const file = last.files[0]
      return file ? sectionKey(last.id, file.id) : null
    })
  }, [diffs])

  const activeEntry = activeKey ? filesByKey.get(activeKey) : undefined
  const activeComments = useMemo(
    () => (activeKey ? comments.filter((c) => sectionKey(c.diffId, c.fileId) === activeKey) : []),
    [comments, activeKey],
  )
  const openComments = comments.filter((c) => !c.resolved).length
  const fileCount = diffs.reduce((total, diff) => total + diff.files.length, 0)

  const setFolded = useCallback((key: string, value: boolean) => {
    setFoldOverrides((prev) => {
      const next = new Map(prev)
      next.set(key, value)
      return next
    })
  }, [])

  const setViewModeFor = useCallback((key: string, mode: ViewMode) => {
    setViewModeOverrides((prev) => {
      const next = new Map(prev)
      next.set(key, mode)
      return next
    })
  }, [])

  // Setting the default for every file at once starts clean: a file toggled
  // on its own before this would otherwise keep ignoring it.
  const setViewModeDefaultFor = useCallback((mode: ViewMode) => {
    setViewModeDefault(mode)
    setViewModeOverrides(new Map())
  }, [])

  const copyPrompt = async () => {
    try {
      const text = await client.prompt(group)
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  // The review is what anything waiting on sbnn is waiting for, so saying "I
  // am done" is an explicit act rather than a guess from the last comment.
  const summary = status?.groups.find((g) => g.name === group)
  const reviewed = summary ? summary.reviewed : reviewedAt !== null
  const hooks = summary?.hooks ?? 0

  const submitReview = async (verdict: Verdict) => {
    setSubmitting(true)
    setError(null)
    try {
      await client.submitReview(group, reviewNote ?? '', verdict)
      setReviewNote(null)
      await reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }

  // Closing is the end of a review: the diffs, the comments and the hooks
  // go, and the page is back to waiting for the next one.
  const closeReview = async () => {
    const open = comments.filter((c) => !c.resolved).length
    const question =
      open > 0
        ? `Close this review? ${open} comment(s) are still open and will go with it.`
        : 'Close this review? Its diffs and comments will go.'
    if (!window.confirm(question)) return
    setClosing(true)
    setError(null)
    try {
      await client.closeReview(group)
      await reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setClosing(false)
    }
  }

  const goToKey = useCallback(
    (key: string) => {
      setActiveKey(key)
      if (narrow) setPane('diff')
      else diffStackRef.current?.scrollToSection(key)
    },
    [narrow],
  )

  const stepFile = useCallback(
    (by: number) => {
      if (flatKeys.length === 0) return
      const at = activeKey ? flatKeys.indexOf(activeKey) : -1
      const next = flatKeys[Math.min(flatKeys.length - 1, Math.max(0, at + by))]
      if (next) goToKey(next)
    },
    [flatKeys, activeKey, goToKey],
  )

  // Comments are what a review is for, so stepping through them is stepping
  // through the review rather than through the files: the next one may be
  // in another file, and going there is the point. Every file's comments
  // are on the page at once now, so landing on the right one is a direct
  // scrollIntoView rather than a guess at "the first .comment on screen".
  const stepComment = useCallback(
    (by: number) => {
      const open = comments.filter((c) => !c.resolved)
      if (open.length === 0) return
      const at = activeKey ? open.findIndex((c) => sectionKey(c.diffId, c.fileId) === activeKey) : -1
      const index = at < 0 ? (by > 0 ? 0 : open.length - 1) : at + by
      const target = open[(index + open.length) % open.length]
      if (!target) return
      goToKey(sectionKey(target.diffId, target.fileId))
      window.setTimeout(() => {
        document.getElementById(`comment-${target.id}`)?.scrollIntoView({ block: 'center' })
      }, 50)
    },
    [comments, activeKey, goToKey],
  )

  // One place where every key is answered. Each is a single unmodified
  // press, which only works because nothing fires while the reader is
  // typing - a comment full of the letter "f" would otherwise fold the file
  // away eleven times.
  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => {
      if (!plainKey(ev)) return
      if (ev.key === 'Escape') {
        setHelp(false)
        setReviewNote(null)
        setSettingsOpen(false)
        return
      }
      if (typingInto(ev.target)) return
      switch (ev.key) {
        case 'j':
          stepFile(1)
          break
        case 'k':
          stepFile(-1)
          break
        case 'n':
          stepComment(1)
          break
        case 'p':
          stepComment(-1)
          break
        case '/':
          focusSearch()
          break
        case 'f': {
          if (!activeKey) break
          const entry = filesByKey.get(activeKey)
          // Toggle away from what is on screen, not from the raw override:
          // the two differ on a commented file the sender had folded, and
          // pressing `f` there used to store a value that changed nothing.
          const hasComments = comments.some((c) => sectionKey(c.diffId, c.fileId) === activeKey)
          const current = resolveFolded(
            foldOverrides.get(activeKey),
            Boolean(entry?.file.folded),
            hasComments,
          )
          setFolded(activeKey, !current)
          break
        }
        case 'v': {
          if (!activeKey) break
          const entry = filesByKey.get(activeKey)
          if (!entry) break
          const current = viewModeOverrides.get(activeKey) ?? viewModeDefault ?? entry.file.viewMode
          setViewModeFor(activeKey, current === 'split' ? 'unified' : 'split')
          break
        }
        case 's':
          setSyncScroll((on) => !on)
          break
        case 'r':
          if (!client.isStatic && diffs.length > 0) setReviewNote((note) => (note === null ? '' : null))
          break
        case '?':
          setHelp((open) => !open)
          break
        default:
          return
      }
      ev.preventDefault()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [
    stepFile,
    stepComment,
    focusSearch,
    diffs.length,
    activeKey,
    comments,
    filesByKey,
    foldOverrides,
    viewModeOverrides,
    viewModeDefault,
    setFolded,
    setViewModeFor,
  ])

  const sidebar = (
    <Sidebar
      width={narrow ? null : sidebarWidth}
      group={group}
      diffs={diffs}
      comments={comments}
      status={status}
      activeKey={activeKey}
      activeDiffId={activeEntry?.diff.id ?? null}
      onSelect={(diffId, fileId) => goToKey(sectionKey(diffId, fileId))}
      onChanged={() => void reload()}
      query={query}
      onQuery={setQuery}
      searchRef={searchRef}
    />
  )

  const previewForced = narrow || client.isStatic
  const resolvedPreviewKind: PreviewKind = previewForced ? 'preview' : previewKind

  const diffPane = narrow ? (
    activeEntry ? (
      <DiffFileSection
        key={activeKey}
        group={group}
        diff={activeEntry.diff}
        file={activeEntry.file}
        comments={activeComments}
        narrow
        onChanged={() => void reload()}
        folded={resolveFolded(
          foldOverrides.get(activeKey!),
          Boolean(activeEntry.file.folded),
          activeComments.length > 0,
        )}
        foldedByReader={foldOverrides.get(activeKey!) === true}
        onSetFolded={(value) => setFolded(activeKey!, value)}
        viewMode={viewModeOverrides.get(activeKey!) ?? viewModeDefault ?? activeEntry.file.viewMode}
        onSetViewMode={(mode) => setViewModeFor(activeKey!, mode)}
      />
    ) : (
      <p className="empty">Select a file.</p>
    )
  ) : (
    <DiffStack
      ref={diffStackRef}
      group={group}
      diffs={diffs}
      comments={comments}
      foldOverrides={foldOverrides}
      viewModeOverrides={viewModeOverrides}
      viewModeDefault={viewModeDefault}
      onSetFolded={setFolded}
      onSetViewMode={setViewModeFor}
      onSetViewModeDefault={setViewModeDefaultFor}
      onChanged={() => void reload()}
      containerRef={diffScrollRef}
      onActiveChange={setActiveKey}
      onScrollFraction={setScrollFraction}
    />
  )

  const previewPane = narrow ? (
    activeEntry ? (
      <PreviewFileSection
        group={group}
        diffId={activeEntry.diff.id}
        file={activeEntry.file}
        status={status}
        kind={resolvedPreviewKind}
        active
      />
    ) : (
      <p className="empty">Select a file.</p>
    )
  ) : (
    <PreviewStack
      group={group}
      diffs={diffs}
      status={status}
      containerRef={previewScrollRef}
      scrollTarget={syncScroll ? scrollFraction : null}
      sync={syncScroll}
      onSync={setSyncScroll}
      kind={resolvedPreviewKind}
      forced={previewForced}
      onSetKind={setPreviewKind}
    />
  )

  // A group with nothing in it yet is still worth a page, since other
  // groups may already have a review waiting - the reader just landed on
  // the wrong one.
  const otherGroups = status?.groups.filter((g) => g.name !== group) ?? []

  const welcome = (
    <div className="welcome">
      <h1>{client.isStatic ? 'This page carries no diff' : 'Waiting for a diff'}</h1>
      <p>Pipe one in — sbnn adds it to this page:</p>
      <pre>
        <code>
          git diff | sbnn{group === 'default' ? '' : ` --target ${group}`}
          {'\n'}diff -u old.md new.md | sbnn{group === 'default' ? '' : ` --target ${group}`}
        </code>
      </pre>
      <p className="hint">
        Comments you leave here are readable from the command line with{' '}
        <code>sbnn comments{group === 'default' ? '' : ` -t ${group}`}</code>.
      </p>
      {otherGroups.length > 0 && (
        <div className="groups">
          <div className="groups-title">Other reviews</div>
          <ul>
            {otherGroups.map((g) => (
              <li key={g.name}>
                <a href={g.url}>
                  {g.name}
                  <span className="hint">
                    {g.diffs} diff(s){g.unresolved > 0 ? `, ${g.unresolved} open` : ''}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )

  return (
    <div className={`app${narrow ? ' narrow' : ''}`}>
      <header className="topbar">
        <span className="brand">sbnn</span>
        <span className="group">{group}</span>
        <span className="hint counts">
          {diffs.length} round(s) · {comments.length} comment(s)
          {openComments > 0 ? ` · ${openComments} open` : ''}
        </span>
        {client.isStatic && (
          <span
            className="badge"
            title={
              'This page was written with `sbnn export`. The diff is frozen and ' +
              'comments are kept in this browser.'
            }
          >
            exported
            {client.exportedAt && (
              <span className="exported-at"> {new Date(client.exportedAt).toLocaleString()}</span>
            )}
          </span>
        )}
        <span className="spacer" />
        <div className="toolbar-group">
          {/* A disabled button swallows hover, so the tooltip that explains
              why it is disabled has to live on a span around it instead. */}
          <span title="Copy the review prompt to paste into an agent">
            <button className="ghost" onClick={() => void copyPrompt()} disabled={comments.length === 0}>
              <Icon name={copied ? 'check' : 'content_copy'} />
              {copied ? 'Copied' : 'Copy prompt'}
            </button>
          </span>
          {!client.isStatic && (
            <button
              className={reviewed ? 'ghost' : ''}
              disabled={submitting || diffs.length === 0}
              onClick={() => setReviewNote((note) => (note === null ? '' : null))}
              title={
                hooks > 0
                  ? `Submitting runs ${hooks} hook(s) on the sbnn server`
                  : 'Tell whoever is waiting that the review is done'
              }
            >
              <Icon name="task_alt" />
              {reviewed ? verdictLabel(reviewVerdict) : 'Submit review'}
            </button>
          )}
        </div>
        <span className="toolbar-divider" />
        <div className="settings-menu" ref={settingsRef}>
          <button
            className="ghost icon-only"
            aria-haspopup="true"
            aria-expanded={settingsOpen}
            onClick={() => setSettingsOpen((open) => !open)}
            title="Settings"
          >
            <Icon name="settings" />
          </button>
          {settingsOpen && (
            <div className="settings-panel" role="menu">
              <div className="settings-row">
                <span className="settings-label">Theme</span>
                <div className="toggle sm">
                  <button
                    className={theme === 'auto' ? 'active' : ''}
                    onClick={() => setTheme('auto')}
                    title="Follow the system"
                  >
                    <Icon name="brightness_auto" small />
                  </button>
                  <button
                    className={theme === 'light' ? 'active' : ''}
                    onClick={() => setTheme('light')}
                    title="Light"
                  >
                    <Icon name="light_mode" small />
                  </button>
                  <button
                    className={theme === 'dark' ? 'active' : ''}
                    onClick={() => setTheme('dark')}
                    title="Dark"
                  >
                    <Icon name="dark_mode" small />
                  </button>
                </div>
              </div>
              {!client.isStatic && diffs.length > 0 && (
                <>
                  <span className="settings-divider" />
                  <button
                    className="settings-item danger"
                    disabled={closing}
                    onClick={() => {
                      setSettingsOpen(false)
                      void closeReview()
                    }}
                    title="Drop this review: its diffs, comments and hooks"
                  >
                    <Icon name="close" small />
                    {closing ? 'Closing…' : 'Close review'}
                  </button>
                </>
              )}
            </div>
          )}
        </div>
      </header>

      {narrow && diffs.length > 0 && (
        <nav className="tabs" aria-label="Panes">
          <button
            className={pane === 'files' ? 'active' : ''}
            aria-pressed={pane === 'files'}
            onClick={() => setPane('files')}
          >
            Files<span className="tab-count">{fileCount}</span>
          </button>
          <button
            className={pane === 'diff' ? 'active' : ''}
            aria-pressed={pane === 'diff'}
            onClick={() => setPane('diff')}
          >
            Diff
            {activeComments.length > 0 && <span className="tab-count">{activeComments.length}</span>}
          </button>
          <button
            className={pane === 'preview' ? 'active' : ''}
            aria-pressed={pane === 'preview'}
            onClick={() => setPane('preview')}
            disabled={!activeEntry || !isPreviewable(activeEntry.file)}
            title={activeEntry && isPreviewable(activeEntry.file) ? undefined : 'This file has no preview'}
          >
            Preview
          </button>
        </nav>
      )}

      {reviewNote !== null && (
        <div className="review-form">
          <label className="field-label" htmlFor="sbnn-review-note">
            Anything to say about the change as a whole? (optional)
          </label>
          <textarea
            id="sbnn-review-note"
            className="comment-input"
            autoFocus
            rows={3}
            value={reviewNote}
            placeholder="Looks good apart from the two comments"
            onChange={(ev) => setReviewNote(ev.target.value)}
            onKeyDown={(ev) => {
              if (ev.key === 'Escape') setReviewNote(null)
              if (ev.key === 'Enter' && (ev.metaKey || ev.ctrlKey)) void submitReview('commented')
            }}
          />
          <div className="comment-actions">
            {/* What the reviewer decided is a separate thing from what any
                comment says, so it is asked here rather than counted. */}
            <button
              className="verdict approve"
              disabled={submitting}
              onClick={() => void submitReview('approved')}
              title="The change can go ahead; comments are worth reading, not blocking"
            >
              Approve
            </button>
            <button
              disabled={submitting}
              onClick={() => void submitReview('commented')}
              title="Say things without deciding either way"
            >
              {submitting ? 'Submitting…' : `Comment (${openComments} open)`}
            </button>
            <button
              className="verdict changes"
              disabled={submitting}
              onClick={() => void submitReview('changes-requested')}
              title="The change should not go ahead as it is"
            >
              Request changes
            </button>
            <button className="ghost" disabled={submitting} onClick={() => setReviewNote(null)}>
              Cancel
            </button>
            <span className="hint">
              {hooks > 0
                ? `${hooks} hook(s) will run on the sbnn server`
                : 'Anything waiting with `sbnn wait` carries on'}
            </span>
          </div>
        </div>
      )}

      {reviewed && reviewNote === null && (
        <div className="review-banner">
          Review submitted{reviewedAt ? ` ${new Date(reviewedAt).toLocaleString()}` : ''}. Send
          another diff to start the next round.
        </div>
      )}

      {error && <div className="error banner">{error}</div>}

      {help && (
        <div className="help-backdrop" onClick={() => setHelp(false)}>
          <div
            className="help"
            role="dialog"
            aria-label="Keyboard shortcuts"
            onClick={(ev) => ev.stopPropagation()}
          >
            <h2>Keys</h2>
            <dl>
              {shortcuts.map((s) => (
                <Fragment key={s.keys.join('+')}>
                  <dt>
                    {s.keys.map((k) => (
                      <kbd key={k}>{k}</kbd>
                    ))}
                  </dt>
                  <dd>{s.what}</dd>
                </Fragment>
              ))}
            </dl>
            <p className="hint">None of them fires while you are typing.</p>
          </div>
        </div>
      )}

      {narrow ? (
        <div className="body">
          {diffs.length === 0 ? (
            <main className="content">{welcome}</main>
          ) : pane === 'files' ? (
            sidebar
          ) : (
            <main className="content">{pane === 'preview' ? previewPane : diffPane}</main>
          )}
        </div>
      ) : (
        <div className="body" ref={bodyRef}>
          {sidebar}
          <Divider
            label="Resize the file list"
            onDrag={resizeSidebar}
            onReset={toggleSidebar}
            onNudge={(direction) =>
              setSidebarWidth((w) => Math.min(SIDEBAR_MAX, Math.max(0, w + direction * SIDEBAR_STEP)))
            }
            handles={[
              {
                icon: sidebarWidth === 0 ? 'chevron_right' : 'chevron_left',
                title: sidebarWidth === 0 ? 'Show the file list' : 'Hide the file list',
                onClick: toggleSidebar,
              },
            ]}
          />
          <main className="content">
            {diffs.length === 0 ? (
              welcome
            ) : (
              <SplitPane
                ratio={splitRatio}
                onRatioChange={setSplitRatio}
                left={diffPane}
                right={previewPane}
                leftRef={diffScrollRef}
                rightRef={previewScrollRef}
              />
            )}
          </main>
        </div>
      )}

      {/* One for the page, not one per preview: a selection is only ever in
          one of them, and it says itself which file it is in. It sits here
          rather than beside the preview because it is pinned to the window,
          and because both layouts - the stack of files and the phone's
          single one - need it. */}
      <PreviewSelection group={group} onChanged={() => void reload()} />
    </div>
  )
}

/** staticGroupName reads the group an exported page was written for. */
function staticGroupName(): string {
  return window.__SBNN_DATA__?.group ?? 'default'
}

/** verdictLabel says what a finished review decided, on the button that
 * used to say only "Reviewed". */
function verdictLabel(verdict: Verdict | null): string {
  switch (verdict) {
    case 'approved':
      return 'Approved'
    case 'changes-requested':
      return 'Changes requested'
    default:
      return 'Reviewed'
  }
}
