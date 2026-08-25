export type LineKind = 'context' | 'add' | 'delete'

export type FileStatus = 'added' | 'deleted' | 'modified' | 'renamed' | 'copied' | 'mode'

export type ViewMode = 'unified' | 'split'

/** PreviewKind is which of the two previews is showing. sbnn renders one
 * itself; mo is the other, richer one, in a frame. */
export type PreviewKind = 'preview' | 'mo'

export interface Line {
  kind: LineKind
  oldNumber: number
  newNumber: number
  content: string
  noNewline?: boolean
}

export interface Hunk {
  header: string
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  section?: string
  lines: Line[]
}

export interface FileDiff {
  id: string
  oldPath: string
  newPath: string
  status: FileStatus
  isBinary: boolean
  oldMode?: string
  newMode?: string
  additions: number
  deletions: number
  viewMode: ViewMode
  isMarkdown: boolean
  /** isImage reports whether the file can be previewed as an image. */
  isImage: boolean
  /** isNotebook reports whether the file is a Jupyter notebook, previewed by
   * rendering its cells. */
  isNotebook: boolean
  /** folded asks the page to keep the file shut until the reader opens it. */
  folded?: boolean
  /** foldReason says why it is shut, so the reader can disagree. */
  foldReason?: string
  hunks: Hunk[]
}

export interface Diff {
  id: string
  title: string
  baseDir: string
  createdAt: string
  raw: string
  files: FileDiff[]
}

export interface Comment {
  id: string
  group: string
  diffId: string
  fileId: string
  path: string
  /** author is set when the comment came from the command line. */
  author?: string
  side: 'new' | 'old'
  startLine: number
  endLine: number
  /** body is Markdown; a proposed replacement is a fenced "suggestion"
   * block inside it, as on GitHub. */
  body: string
  snippet: string
  /** suggestions are the replacement blocks the server parsed out of body. */
  suggestions?: string[]
  /** question marks a comment that wants an answer, not a change. */
  question?: boolean
  resolved: boolean
  createdAt: string
  updatedAt: string
}

export type Verdict = 'approved' | 'commented' | 'changes-requested'

export interface Group {
  name: string
  diffs: Diff[] | null
  comments: Comment[] | null
  /** reviewedAt is when the review was last submitted. */
  reviewedAt?: string
  reviewNote?: string
  /** reviewVerdict is what the reviewer decided about the change as a
   * whole: approved, commented or changes-requested. */
  reviewVerdict?: Verdict
}

export interface GroupSummary {
  name: string
  url: string
  diffs: number
  files: number
  comments: number
  unresolved: number
  reviewedAt?: string
  /** reviewed is false again once a diff arrives after the last review. */
  reviewed: boolean
  /** hooks is what the server will run when the review is submitted. */
  hooks: number
}

export interface Status {
  app: string
  version: string
  revision?: string
  pid: number
  url: string
  moUrl: string
  moProxyUrl?: string
  moAvailable: boolean
  moError?: string
  groups: GroupSummary[]
}

export interface Preview {
  url: string
  moUrl: string
  path: string
  source: 'worktree' | 'reconstructed'
  complete: boolean
}

/** filePath returns the path a file is identified by. */
export function filePath(file: FileDiff): string {
  return file.newPath || file.oldPath
}

/** PreviewFormat is which of the preview pane's renderers a file uses, or
 * null when the pane has nothing to show for it at all. */
export type PreviewFormat = 'markdown' | 'notebook' | 'image' | 'source' | null

/**
 * previewFormatOf says how the preview pane would draw a file.
 *
 * The first three are the rendered formats and are decided by the flags the
 * server sets. Everything else that is text is 'source': the file's own
 * lines, syntax coloured. A binary that is not an image has nothing to show,
 * and a deleted file has no new side to show - but a deleted Markdown file
 * still answers 'markdown', because it did before this function existed and
 * the section it produces reports the server's refusal rather than
 * disappearing from the pane.
 *
 * hasSource is false where there is no server behind the page. An exported
 * page freezes a preview only for Markdown, notebooks and images
 * (internal/export/export.go), so a source file there would be a section
 * that can only say it has nothing - which is what leaving it out avoids.
 */
export function previewFormatOf(file: FileDiff, hasSource: boolean): PreviewFormat {
  if (file.isMarkdown) return 'markdown'
  if (file.isNotebook) return 'notebook'
  if (file.isImage) return 'image'
  if (!hasSource || file.isBinary || file.status === 'deleted') return null
  return 'source'
}

/** isPreviewable reports whether the preview pane has anything to show for
 * file, regardless of which of its renderers it would use. */
export function isPreviewable(file: FileDiff, hasSource: boolean): boolean {
  return previewFormatOf(file, hasSource) !== null
}
