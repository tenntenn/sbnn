/**
 * The words the review page says, in every language it speaks.
 *
 * Every user-visible string in the UI is a key here rather than a literal in
 * a component, so that a language is one more dictionary and not a hunt
 * through the tree. The English catalog is the reference: its keys are the
 * type every other catalog has to match, and web/test/i18n.test.ts fails when
 * a key or a {placeholder} is in one and not the other - TypeScript catches a
 * missing key, but not a placeholder that was renamed in one language only.
 *
 * The language follows the browser's own list (navigator.languages) until
 * the reader picks one in the settings menu, and that pick is remembered the
 * way the theme is. Outside a page - the test runner - there is no reader to
 * follow, and the page speaks English, so a test that asserts a sentence
 * does not depend on the locale of the machine it runs on.
 *
 * There is no library behind this on purpose: what the page needs is a
 * lookup, a substitution and a way to re-render on a change, and none of
 * that is worth a dependency in a bundle that ships inside the binary.
 */

import { createElement, Fragment, useSyncExternalStore, type ReactNode } from 'react'
import { readEnumSetting, writeSetting } from './storage'

const en = {
  // The page as a whole.
  'app.selectFile': 'Select a file.',
  'app.counts': '{rounds} round(s) · {comments} comment(s)',
  'app.countsOpen': ' · {count} open',
  'app.countsTitle':
    'A round is one diff sent into this review; the comments are counted over every round, and open is the ones nobody has resolved yet.',
  'app.exported': 'exported',
  'app.exportedTitle':
    'This page was written with `sbnn export`. The diff is frozen and comments are kept in this browser.',
  'app.copyPrompt': 'Copy prompt',
  'app.copyPromptTitle': 'Copy the review prompt to paste into an agent',
  'app.copied': 'Copied',
  'app.submitReview': 'Submit review',
  'app.submitTitle': 'Tell whoever is waiting that the review is done',
  'app.submitTitleHooks': 'Submitting runs {count} hook(s) on the sbnn server',

  'verdict.approved': 'Approved',
  'verdict.changesRequested': 'Changes requested',
  'verdict.reviewed': 'Reviewed',

  'welcome.static': 'This page carries no diff',
  'welcome.waiting': 'Waiting for a diff',
  'welcome.pipe': 'Pipe one in — sbnn adds it to this page:',
  'welcome.comments': 'Comments you leave here are readable from the command line with {command}.',
  'welcome.otherReviews': 'Other reviews',
  'welcome.groupDiffs': '{count} diff(s)',
  'group.open': ', {count} open',
  'group.rounds': '{count} round(s)',

  // The settings menu.
  'settings.title': 'Settings',
  'settings.theme': 'Theme',
  'settings.themeAuto': 'Follow the system',
  'settings.themeLight': 'Light',
  'settings.themeDark': 'Dark',
  'settings.language': 'Language',
  'settings.languageAuto': 'Auto',
  'settings.languageAutoTitle': 'Follow the language of the browser',
  'settings.notify': 'Notify on new diff',
  'settings.notifyTitle':
    'When this tab is in the background, mark its title and show a notification as a new diff arrives',
  'settings.on': 'On',
  'settings.off': 'Off',
  'settings.closeReview': 'Close review',
  'settings.closing': 'Closing…',
  'settings.closeTitle': 'Drop this review: its diffs, comments and hooks',
  'settings.closeConfirm': 'Close this review? Its diffs and comments will go.',
  'settings.closeConfirmOpen': 'Close this review? {count} comment(s) are still open and will go with it.',

  // The phone's pane tabs.
  'tabs.label': 'Panes',
  'tabs.files': 'Files',
  'tabs.diff': 'Diff',
  'tabs.preview': 'Preview',
  'tabs.noPreview': 'This file has no preview',

  // Submitting a review.
  'review.noteLabel': 'Anything to say about the change as a whole? (optional)',
  'review.notePlaceholder': 'Looks good apart from the two comments',
  'review.approve': 'Approve',
  'review.approveTitle': 'The change can go ahead; comments are worth reading, not blocking',
  'review.comment': 'Comment ({count} open)',
  'review.commentTitle': 'Say things without deciding either way',
  'review.submitting': 'Submitting…',
  'review.requestChanges': 'Request changes',
  'review.requestChangesTitle': 'The change should not go ahead as it is',
  'review.hooks': '{count} hook(s) will run on the sbnn server',
  'review.wait': 'Anything waiting with `sbnn wait` carries on',
  'review.submitted': 'Review submitted. Send another diff to start the next round.',
  'review.submittedAt': 'Review submitted {date}. Send another diff to start the next round.',

  'common.cancel': 'Cancel',
  'common.copy': 'Copy',
  'common.copied': 'Copied',
  'common.dismiss': 'Dismiss',

  // The keyboard sheet.
  'help.label': 'Keyboard shortcuts',
  'help.title': 'Keys',
  'help.typing': 'None of them fires while you are typing.',
  'key.nextFile': 'Next file',
  'key.previousFile': 'Previous file',
  'key.search': 'Search paths and lines',
  'key.nextComment': 'Next comment',
  'key.previousComment': 'Previous comment',
  'key.fold': 'Fold or unfold this file',
  'key.viewMode': 'Split or unified',
  'key.sync': 'Follow the diff with the preview',
  'key.commentLine': 'Comment on a line: Tab to its line number, then press. Shift extends the range',
  'key.submit': 'Submit review',
  'key.help': 'This list',
  'key.escape': 'Close what is open',

  // The dividers between panes.
  'divider.sidebar': 'Resize the file list',
  'divider.showFiles': 'Show the file list',
  'divider.hideFiles': 'Hide the file list',
  'divider.split': 'Resize the diff and the preview',
  'divider.showDiff': 'Show the diff',
  'divider.showPreview': 'Show the preview',
  'divider.hideDiff': 'Hide the diff',
  'divider.hidePreview': 'Hide the preview',

  // The file list.
  'sidebar.empty': 'No diff yet.',
  'sidebar.searchPlaceholder': 'Search paths and lines ( / )',
  'sidebar.searchLabel': 'Search files by path and by what the diff lines say',
  'sidebar.found': '{found} of {total}',
  'sidebar.foundLines.one': ', {count} line',
  'sidebar.foundLines.other': ', {count} lines',
  'sidebar.list': 'list',
  'sidebar.listTitle': 'Stack every round in one list',
  'sidebar.tabs': 'tabs',
  'sidebar.tabsTitle': 'Show one round at a time',
  'sidebar.read': '{read} of {total} read',
  'sidebar.markAllUnread': 'Mark all unread',
  'sidebar.markAllUnreadTitle': 'Clear every read mark in this review',
  'sidebar.removeRound': 'Remove this round',
  'sidebar.tabMatches': 'files matching in this round',
  'sidebar.truncated':
    'Stopped reading lines after {max}; paths are still searched in full. Narrow the search to see the rest.',
  'sidebar.noMatch': 'Nothing matches that.',
  'sidebar.groups': 'Groups',
  'sidebar.markRead': 'Mark {path} read',
  'sidebar.markUnread': 'Mark {path} unread',
  'sidebar.markReadTitle': 'Mark as read',
  'sidebar.markUnreadTitle': 'Mark as unread',
  'sidebar.goTo': 'Go to this file',
  'sidebar.folded': 'folded',
  'sidebar.mdTitle': 'Markdown, previewable in the preview pane',
  'sidebar.imgTitle': 'Previewable as an image',
  'sidebar.ipynbTitle': 'Previewable as a Jupyter notebook',
  'sidebar.removeQuestion': 'Remove {title}? This cannot be undone.',
  'sidebar.removeQuestionComments': 'Remove {title}? {count} comment(s) on it will be deleted too.',
  'sidebar.removeFailed': 'Could not remove {title}: {error}',

  'search.path': 'path',
  'search.lines.one': '{count} line',
  'search.lines.other': '{count} lines',

  'status.added': 'added',
  'status.modified': 'modified',
  'status.deleted': 'deleted',
  'status.renamed': 'renamed',
  'status.copied': 'copied',
  'status.mode': 'mode',

  // A file in the diff pane.
  'diff.show': 'Show this file',
  'diff.fold': 'Fold this file away',
  'diff.commentOnFile': 'comment on file',
  'diff.commentOnFileTitle': 'Comment on this file as a whole, without pointing at a line',
  'diff.split': 'split',
  'diff.unified': 'unified',
  'diff.splitTitle': 'Old and new side by side',
  'diff.unifiedTitle': 'Old and new lines in one column',
  'diff.rangeHint': 'Drag across the line numbers, or shift-click one, to cover more lines',
  'diff.fileHint': 'This one is about the file, not about a line of it',
  'diff.foldedLines': '{label} · {count} changed lines',
  'diff.binary': 'Binary file — no diff to show.',
  'diff.noContent': 'No content change.',
  'diff.noContentMode': 'No content change (mode {old} → {new}).',
  'diff.commentOld': 'Comment on old line {line}',
  'diff.commentNew': 'Comment on new line {line}',
  'diff.noNewline': ' (no newline at end of file)',
  'diff.old': ' (old)',
  'diff.everyFile': 'Every file',
  'diff.everySplitTitle': 'Set every file to split - a file toggled on its own still remembers that choice',
  'diff.everyUnifiedTitle': 'Set every file to unified - a file toggled on its own still remembers that choice',
  'diff.stepper': 'Files in this review',
  'diff.previousFile': 'Previous file',
  'diff.nextFile': 'Next file',
  'diff.position': '{at} of {total}',

  'fold.byYouToo': 'Folded by you — the sender had folded it too: {reason}',
  'fold.byYou': 'Folded by you',
  'fold.reason': 'Folded — {reason}',
  'fold.plain': 'Folded',

  // Comments.
  'comment.suggestedChange': 'Suggested change — replaces {range}',
  'comment.linesRemoved': '(the lines are removed)',
  'comment.suggestion': 'suggestion',
  'comment.question': 'question',
  'comment.resolved': 'resolved',
  'comment.save': 'Save',
  'comment.resolve': 'Resolve',
  'comment.reopen': 'Reopen',
  'comment.edit': 'Edit',
  'comment.delete': 'Delete',
  'comment.placeholder': 'What should change here?',
  'comment.suggest': 'Suggest a change',
  'comment.suggestTitle': 'Insert a suggestion block',
  'comment.ask': 'Ask',
  'comment.comment': 'Comment',
  'comment.questionSwitch': 'Question',
  'comment.questionTitle': 'It wants an answer, not a change',
  'comment.sendHint': '⌘/Ctrl + Enter',
  'comment.reply': 'Reply',
  'comment.sendReply': 'Reply',
  'comment.replyPlaceholder': 'Write a reply',
  'comment.replyLabel': 'Reply to this comment',
  'comment.replies': 'Replies',
  'comment.replyFrom': 'Reply from {author}',
  'comment.reviewer': 'reviewer',

  // Commenting from the preview.
  'selection.commentOn': 'Comment on {range}',
  'selection.add': 'Comment',
  'selection.addTitle': 'Add a comment on {range}',
  'selection.hint': 'Selected in the preview; the comment covers the lines it touches',

  // The preview pane.
  'preview.tree': 'tree',
  'preview.treeTitle': 'the working tree file',
  'preview.rebuilt': 'rebuilt',
  'preview.rebuiltTitle': 'rebuilt from the diff',
  'preview.partial': 'partial',
  'preview.partialTitle': 'A unified diff only carries the changed hunks',
  'preview.reload': 'Reload the preview',
  'preview.opening': 'Opening…',
  'preview.none': '{path} has no preview.',
  'preview.notLoaded': 'Not loaded yet…',
  'preview.imageFailed': 'The image could not be loaded.',
  'preview.notInTree': '{path} is not in the working tree (it may have been deleted), so there is nothing to preview.',
  'preview.askingMo': 'Asking mo for a preview…',
  'preview.rendering': 'Rendering…',
  'preview.moMissing':
    "mo renders a richer preview than sbnn's own, and it is not installed here. Install it with {command} or grab a binary from {releases}, then reload — or switch back to {preview}, which needs nothing.",
  'preview.releases': 'the releases page',
  'preview.frameTitle': 'Markdown preview',
  'preview.cannotEmbed': 'The preview cannot be embedded here.',
  'preview.openInstead': '{link} instead.',
  'preview.openItInMo': 'Open it in mo',
  'preview.nothing': 'No preview.',
  'preview.noneInReview': 'No file in this review has a preview.',
  'preview.preview': 'preview',
  'preview.previewTitle': "sbnn's own preview - needs nothing installed, follows the diff as it scrolls",
  'preview.moTitle': 'mo - renders more, in a frame, but does not follow the diff',
  'preview.syncMo':
    "Only sbnn's own preview can follow the diff: mo is framed from another origin, where a page may not touch its scrolling",
  'preview.syncOn': 'The preview follows the diff; scrolling it yourself stops that',
  'preview.syncOff': 'Follow the diff again',
  'preview.linesShown': '{shown} of {total} lines shown.',
  'preview.showRest': 'Show the remaining {count}',
  'preview.notInReview': 'not in this review',
  'preview.removed': 'Removed',
  'preview.marks': 'Show what the diff changed in the preview',
  'preview.marksOn': 'Showing what the diff added (green) and removed (red); click to hide',
  'preview.marksOff': 'Show what the diff added (green) and removed (red)',

  'asset.tooLarge': 'too large to show here ({size})',
  'asset.overBudget': 'left out to keep this page loadable ({size})',
  'asset.outside': 'outside the directory this diff was sent from',
  'asset.missing': 'not in the working tree',
  'asset.unsupported': 'not an image this page can show',
  'asset.notInReview': 'not part of this review',
  'asset.unknownSize': 'unknown size',

  'notebook.invalid': 'This notebook is not valid JSON: {error}',
  'notebook.empty': 'This notebook has no cells to show.',

  // Telling a background tab that a diff arrived.
  'notify.title': '{name}: new diff',
  'notify.body.one': 'A new diff is ready to review.',
  'notify.body.other': '{count} new diffs are ready to review.',

  // What an exported page cannot do, and what it cannot keep.
  'static.noDrop': 'an exported page cannot drop a diff',
  'static.noClose': 'an exported page has no review to close',
  'static.noSubmit': 'an exported page has no server to submit the review to',
  'static.noReplyTarget': 'the comment this replies to is no longer on the page',
  'static.noPreview': 'this page carries no preview for that file',
  'static.noSource': 'an exported page carries no source text',
  'storage.unreachable': 'This page cannot reach browser storage, so your comments will not be saved.',
  'storage.unreachableAdvice': ' Use "Copy prompt" before closing the tab to keep whatever you write here.',
  'storage.unreadable':
    'Comments saved on this page earlier could not be read back, so this is the review as it was exported.',
  'storage.unreadableAdvice': ' What you write from here on is saved again, over the entry that could not be read.',
  'storage.unsaved':
    'This browser refused to save your comments (private window, blocked site data, or a full quota).',
  'storage.unsavedAdvice': ' Use "Copy prompt" now to keep a copy: nothing you write here will survive this tab.',

  // Tooltips for controls whose meaning is not on their face (#409).
  'app.help': 'Help: keys and a tour of the page',
  'help.tour': 'Show the tour again',
  'sidebar.label': 'Files in this review',
  'sidebar.layoutLabel': 'How the rounds are shown',
  'sidebar.readTitle': 'Files you have ticked off as read, out of every file in this review',
  'sidebar.roundFiles.one': '{count} file',
  'sidebar.roundFiles.other': '{count} files',
  'sidebar.roundToggle': 'Show or hide the files of this round, sent {date}',
  'sidebar.openComments': 'Open comments',
  'sidebar.additions': 'Lines added',
  'sidebar.deletions': 'Lines removed',
  'diff.viewModeLabel': 'How this file is laid out',
  'comment.resolveTitle': 'Mark this comment as dealt with; it stays on the page, faded',
  'comment.reopenTitle': 'Mark this comment as still open',
  'comment.deleteTitle': 'Delete this comment for good',
  'comment.deleteThreadTitle': 'Delete this comment and its replies for good',
  'comment.deleteReplyTitle': 'Delete this reply for good',
  'comment.replyTitle': 'Answer this comment; the reply goes under it, on the same lines',
  'comment.sendHintTitle': 'Press ⌘ + Enter (Ctrl + Enter elsewhere) to send',
  'preview.openInMo': 'Open this file in mo, in a new tab',
  'preview.rendererLabel': 'Which renderer draws the preview',

  // The first-run tour.
  'tour.label': 'Tour of the review page',
  'tour.step': '{at} / {total}',
  'tour.back': 'Back',
  'tour.next': 'Next',
  'tour.done': 'Done',
  'tour.skip': 'Skip tour',
  'tour.files.title': 'The files in this review',
  'tour.files.body':
    'Every file the review carries, round by round. Search paths and lines with /, tick a file off as read, and click one to go to it.',
  'tour.diff.title': 'The diff',
  'tour.diff.body':
    "One section per file. Its header folds it away and switches between split and unified; j and k step from file to file.",
  'tour.preview.title': 'The preview',
  'tour.preview.body':
    'Markdown, notebooks, images and source files are rendered here, and follow the diff as you scroll. In Markdown, what the diff added is marked green and what it removed is shown in red where it was; ± in the toolbar turns the marks off.',
  'tour.panes.title': 'Making room',
  'tour.panes.body':
    '< puts the diff away and > puts the preview away, giving the other the whole width; the same button brings it back. Drag the bar between them to resize, or double-click it to go back to the default widths. The < beside the file list puts the list away the same way.',
  'tour.previewComment.title': 'Commenting on the preview',
  'tour.previewComment.body':
    'You can comment on the rendered Markdown too: select a passage, then press Comment beside it. The comment lands on the lines of the file the passage came from, and shows up in the diff as well.',
  'tour.comment.title': 'Commenting on a line',
  'tour.comment.body':
    'Click a line number to comment on that line; drag across several, or shift-click one, to cover a range. From the keyboard, Tab to a line number and press Enter.',
  'tour.review.title': 'Finishing the review',
  'tour.review.body':
    'When you are done, Submit review tells whoever is waiting, with your verdict. Copy prompt puts the open comments on the clipboard for an agent.',
  'tour.review.bodyStatic':
    'Copy prompt puts your comments on the clipboard, to paste into an agent. On an exported page they are kept in this browser only.',
  'tour.help.title': 'Help is always here',
  'tour.help.body':
    'This button, or the ? key, lists every shortcut and starts this tour again. The settings beside it hold the theme and the language.',
} satisfies Record<string, string>

/** MessageKey is every key the page may look up. */
export type MessageKey = keyof typeof en

// Japanese is written for a Japanese reader rather than translated word for
// word: the words a Japanese developer already uses for these things - diff,
// hunk, Markdown, mo, prompt - stay as they are, and the sentences are the
// ones a Japanese tool would say.
const ja: Record<MessageKey, string> = {
  'app.selectFile': 'ファイルを選んでください。',
  'app.counts': 'diff {rounds} 件 · コメント {comments} 件',
  'app.countsOpen': ' · 未解決 {count} 件',
  'app.countsTitle':
    'このレビューに送られた diff の数と、それらに付いたコメントの数です。未解決は、まだ解決されていないコメントです。',
  'app.exported': 'エクスポート版',
  'app.exportedTitle':
    'このページは `sbnn export` で書き出されたものです。diff は固定されていて、コメントはこのブラウザに保存されます。',
  'app.copyPrompt': 'プロンプトをコピー',
  'app.copyPromptTitle': 'エージェントに貼り付けるレビュー用プロンプトをコピーします',
  'app.copied': 'コピーしました',
  'app.submitReview': 'レビューを送信',
  'app.submitTitle': '待っている相手に、レビューが終わったことを伝えます',
  'app.submitTitleHooks': '送信すると sbnn サーバーで hook が {count} 件実行されます',

  'verdict.approved': '承認済み',
  'verdict.changesRequested': '修正依頼済み',
  'verdict.reviewed': 'レビュー済み',

  'welcome.static': 'このページには diff がありません',
  'welcome.waiting': 'diff を待っています',
  'welcome.pipe': 'パイプで渡すと、sbnn がこのページに追加します:',
  'welcome.comments': 'ここに残したコメントは、コマンドラインから {command} で読めます。',
  'welcome.otherReviews': 'ほかのレビュー',
  'welcome.groupDiffs': 'diff {count} 件',
  'group.open': '、未解決 {count} 件',
  'group.rounds': 'diff {count} 件',

  'settings.title': '設定',
  'settings.theme': 'テーマ',
  'settings.themeAuto': 'システムの設定に合わせる',
  'settings.themeLight': 'ライト',
  'settings.themeDark': 'ダーク',
  'settings.language': '言語',
  'settings.languageAuto': '自動',
  'settings.languageAutoTitle': 'ブラウザの言語に合わせる',
  'settings.notify': '新しい diff を通知',
  'settings.notifyTitle':
    'このタブがバックグラウンドにあるとき、新しい diff が届いたらタブのタイトルに印を付けて通知を表示します',
  'settings.on': 'オン',
  'settings.off': 'オフ',
  'settings.closeReview': 'レビューを閉じる',
  'settings.closing': '閉じています…',
  'settings.closeTitle': 'このレビューを破棄します: diff、コメント、hook がすべて消えます',
  'settings.closeConfirm': 'このレビューを閉じますか? diff とコメントは削除されます。',
  'settings.closeConfirmOpen': 'このレビューを閉じますか? 未解決のコメントが {count} 件あり、一緒に削除されます。',

  'tabs.label': 'ペイン',
  'tabs.files': 'ファイル',
  'tabs.diff': 'diff',
  'tabs.preview': 'プレビュー',
  'tabs.noPreview': 'このファイルにはプレビューがありません',

  'review.noteLabel': '変更全体について伝えたいことはありますか?(任意)',
  'review.notePlaceholder': '2 件のコメント以外は問題ありません',
  'review.approve': '承認',
  'review.approveTitle': '変更を進めて構いません。コメントは参考で、マージを止めるものではありません',
  'review.comment': 'コメントのみ(未解決 {count} 件)',
  'review.commentTitle': '可否は決めずに、コメントだけを伝えます',
  'review.submitting': '送信しています…',
  'review.requestChanges': '修正を依頼',
  'review.requestChangesTitle': 'このままでは変更を進めるべきではありません',
  'review.hooks': 'sbnn サーバーで hook が {count} 件実行されます',
  'review.wait': '`sbnn wait` で待っている処理が再開します',
  'review.submitted': 'レビューを送信しました。続けてレビューするには、新しい diff を送ってください。',
  'review.submittedAt': 'レビューを送信しました({date})。次のラウンドを始めるには、新しい diff を送ってください。',

  'common.cancel': 'キャンセル',
  'common.copy': 'コピー',
  'common.copied': 'コピーしました',
  'common.dismiss': '閉じる',

  'help.label': 'キーボードショートカット',
  'help.title': 'キー操作',
  'help.typing': '文字を入力している間は、どのキーも反応しません。',
  'key.nextFile': '次のファイル',
  'key.previousFile': '前のファイル',
  'key.search': 'パスと行を検索',
  'key.nextComment': '次のコメント',
  'key.previousComment': '前のコメント',
  'key.fold': 'このファイルを折りたたむ / 展開する',
  'key.viewMode': '左右に並べる表示と 1 列の表示を切り替え',
  'key.sync': 'プレビューを diff のスクロールに追従させる',
  'key.commentLine': '行にコメント: Tab で行番号に移動してから押します。Shift を併用すると範囲を広げられます',
  'key.submit': 'レビューを送信',
  'key.help': 'この一覧',
  'key.escape': '開いているものを閉じる',

  'divider.sidebar': 'ファイル一覧の幅を変更',
  'divider.showFiles': 'ファイル一覧を表示',
  'divider.hideFiles': 'ファイル一覧を隠す',
  'divider.split': 'diff とプレビューの幅を変更',
  'divider.showDiff': 'diff を表示',
  'divider.showPreview': 'プレビューを表示',
  'divider.hideDiff': 'diff を隠す',
  'divider.hidePreview': 'プレビューを隠す',

  'sidebar.empty': 'まだ diff がありません。',
  'sidebar.searchPlaceholder': 'パスと行を検索 ( / )',
  'sidebar.searchLabel': 'パスと diff の行の内容でファイルを検索',
  'sidebar.found': '{total} 件中 {found} 件',
  'sidebar.foundLines.one': '、{count} 行',
  'sidebar.foundLines.other': '、{count} 行',
  'sidebar.list': '一覧',
  'sidebar.listTitle': 'すべての diff を 1 つの一覧に並べる',
  'sidebar.tabs': 'タブ',
  'sidebar.tabsTitle': 'diff を 1 つずつタブで表示する',
  'sidebar.read': '{total} 件中 {read} 件 既読',
  'sidebar.markAllUnread': 'すべて未読に戻す',
  'sidebar.markAllUnreadTitle': 'このレビューの既読の印をすべて外します',
  'sidebar.removeRound': 'この diff を削除',
  'sidebar.tabMatches': 'この diff で一致したファイル数',
  'sidebar.truncated':
    '{max} 行で行の検索を打ち切りました。パスはすべて検索済みです。残りを見るには検索語を絞り込んでください。',
  'sidebar.noMatch': '一致するものはありません。',
  'sidebar.groups': 'グループ',
  'sidebar.markRead': '{path} を既読にする',
  'sidebar.markUnread': '{path} を未読に戻す',
  'sidebar.markReadTitle': '既読にする',
  'sidebar.markUnreadTitle': '未読に戻す',
  'sidebar.goTo': 'このファイルへ移動',
  'sidebar.folded': '折りたたみ',
  'sidebar.mdTitle': 'Markdown。プレビューペインで表示できます',
  'sidebar.imgTitle': '画像としてプレビューできます',
  'sidebar.ipynbTitle': 'Jupyter notebook としてプレビューできます',
  'sidebar.removeQuestion': '{title} を削除しますか? 元に戻せません。',
  'sidebar.removeQuestionComments': '{title} を削除しますか? この diff に付いたコメント {count} 件も削除されます。',
  'sidebar.removeFailed': '{title} を削除できませんでした: {error}',

  'search.path': 'パス',
  'search.lines.one': '{count} 行',
  'search.lines.other': '{count} 行',

  'status.added': '追加',
  'status.modified': '変更',
  'status.deleted': '削除',
  'status.renamed': '名前変更',
  'status.copied': 'コピー',
  'status.mode': 'モード変更',

  'diff.show': 'このファイルを表示',
  'diff.fold': 'このファイルを折りたたむ',
  'diff.commentOnFile': 'ファイルにコメント',
  'diff.commentOnFileTitle': '行を指定せず、ファイル全体についてコメントします',
  'diff.split': '左右に並べる',
  'diff.unified': '1 列で表示',
  'diff.splitTitle': '変更前と変更後を左右に並べて表示',
  'diff.unifiedTitle': '変更前と変更後の行を 1 列にまとめて表示',
  'diff.rangeHint': '行番号をドラッグするか、Shift を押しながらクリックすると範囲を広げられます',
  'diff.fileHint': '特定の行ではなく、ファイル全体へのコメントです',
  'diff.foldedLines': '{label} · {count} 行の変更',
  'diff.binary': 'バイナリファイルのため、表示できる diff はありません。',
  'diff.noContent': '内容の変更はありません。',
  'diff.noContentMode': '内容の変更はありません(モード {old} → {new})。',
  'diff.commentOld': '変更前の {line} 行目にコメント',
  'diff.commentNew': '変更後の {line} 行目にコメント',
  'diff.noNewline': ' (ファイル末尾に改行なし)',
  'diff.old': ' (変更前)',
  'diff.everyFile': '差分の表示(全ファイル)',
  'diff.everySplitTitle': 'すべてのファイルで、変更前と変更後を左右に並べて表示します。個別に切り替えたファイルはその設定を保ちます',
  'diff.everyUnifiedTitle': 'すべてのファイルで、変更前と変更後を 1 列にまとめて表示します。個別に切り替えたファイルはその設定を保ちます',
  'diff.stepper': 'このレビューのファイル',
  'diff.previousFile': '前のファイル',
  'diff.nextFile': '次のファイル',
  'diff.position': '{at} / {total}',

  'fold.byYouToo': 'あなたが折りたたみました — 送信者も折りたたんでいました: {reason}',
  'fold.byYou': 'あなたが折りたたみました',
  'fold.reason': '折りたたみ — {reason}',
  'fold.plain': '折りたたみ',

  'comment.suggestedChange': '変更の提案 — {range} を置き換えます',
  'comment.linesRemoved': '(この行は削除されます)',
  'comment.suggestion': '提案',
  'comment.question': '質問',
  'comment.resolved': '解決済み',
  'comment.save': '保存',
  'comment.resolve': '解決',
  'comment.reopen': '未解決に戻す',
  'comment.edit': '編集',
  'comment.delete': '削除',
  'comment.placeholder': 'ここで何を変えるべきですか?',
  'comment.suggest': '変更を提案',
  'comment.suggestTitle': 'suggestion ブロックを挿入します',
  'comment.ask': '質問する',
  'comment.comment': 'コメント',
  'comment.questionSwitch': '質問',
  'comment.questionTitle': '変更ではなく、回答を求めるコメントです',
  'comment.sendHint': '⌘/Ctrl + Enter',
  'comment.reply': '返信',
  'comment.sendReply': '返信する',
  'comment.replyPlaceholder': '返信を書く',
  'comment.replyLabel': 'このコメントへの返信',
  'comment.replies': '返信',
  'comment.replyFrom': '{author} からの返信',
  'comment.reviewer': 'レビュアー',

  'selection.commentOn': '{range} にコメント',
  'selection.add': 'コメント',
  'selection.addTitle': '{range} にコメントを追加',
  'selection.hint': 'プレビューで選択した範囲です。コメントは選択にかかる行全体が対象になります',

  'preview.tree': '作業ツリー',
  'preview.treeTitle': '作業ツリーのファイル',
  'preview.rebuilt': '再構成',
  'preview.rebuiltTitle': 'diff から再構成した内容',
  'preview.partial': '一部のみ',
  'preview.partialTitle': 'unified diff には変更のあった hunk しか含まれません',
  'preview.reload': 'プレビューを再読み込み',
  'preview.opening': '開いています…',
  'preview.none': '{path} にはプレビューがありません。',
  'preview.notLoaded': 'まだ読み込んでいません…',
  'preview.imageFailed': '画像を読み込めませんでした。',
  'preview.notInTree': '{path} は作業ツリーにありません(削除された可能性があります)。プレビューできるものがありません。',
  'preview.askingMo': 'mo にプレビューを依頼しています…',
  'preview.rendering': '描画しています…',
  'preview.moMissing':
    'mo は sbnn 自身のプレビューより表現力の高いプレビューを描画しますが、この環境にはインストールされていません。{command} でインストールするか、{releases}からバイナリを入手して再読み込みしてください。何もインストールせずに使える {preview} に戻すこともできます。',
  'preview.releases': 'リリースページ',
  'preview.frameTitle': 'Markdown プレビュー',
  'preview.cannotEmbed': 'ここにはプレビューを埋め込めません。',
  'preview.openInstead': '代わりに{link}。',
  'preview.openItInMo': 'mo で開いてください',
  'preview.nothing': 'プレビューはありません。',
  'preview.noneInReview': 'このレビューにはプレビューできるファイルがありません。',
  'preview.preview': 'プレビュー',
  'preview.previewTitle': 'sbnn 自身のプレビュー。何もインストール不要で、diff のスクロールに追従します',
  'preview.moTitle': 'mo。より多くの記法を描画しますが、フレーム内に表示され diff には追従しません',
  'preview.syncMo':
    'diff に追従できるのは sbnn 自身のプレビューだけです。mo は別オリジンのフレームに表示されるため、スクロールを操作できません',
  'preview.syncOn': 'プレビューは diff に追従しています。自分でスクロールすると追従を止めます',
  'preview.syncOff': 'もう一度 diff に追従させる',
  'preview.linesShown': '{total} 行中 {shown} 行を表示しています。',
  'preview.showRest': '残りの {count} 行を表示',
  'preview.notInReview': 'このレビュー外',
  'preview.removed': '削除',
  'preview.marks': 'diff による変更をプレビューに表示する',
  'preview.marksOn': 'diff で追加された部分(緑)と削除された部分(赤)を表示しています。クリックすると隠します',
  'preview.marksOff': 'diff で追加された部分(緑)と削除された部分(赤)を表示する',

  'asset.tooLarge': '大きすぎるため表示できません({size})',
  'asset.overBudget': 'ページを読み込める大きさに保つため省略しました({size})',
  'asset.outside': 'diff を送ったディレクトリの外にあります',
  'asset.missing': '作業ツリーにありません',
  'asset.unsupported': 'このページでは表示できない画像形式です',
  'asset.notInReview': 'このレビューには含まれていません',
  'asset.unknownSize': 'サイズ不明',

  'notebook.invalid': 'この notebook は正しい JSON ではありません: {error}',
  'notebook.empty': 'この notebook には表示できるセルがありません。',

  'notify.title': '{name}: 新しい diff',
  'notify.body.one': '新しい diff がレビューを待っています。',
  'notify.body.other': '新しい diff が {count} 件、レビューを待っています。',

  'static.noDrop': 'エクスポートしたページでは diff を削除できません',
  'static.noClose': 'エクスポートしたページには閉じるレビューがありません',
  'static.noSubmit': 'エクスポートしたページにはレビューを送信するサーバーがありません',
  'static.noReplyTarget': '返信先のコメントはもうこのページにありません',
  'static.noPreview': 'このページにはそのファイルのプレビューが含まれていません',
  'static.noSource': 'エクスポートしたページにはソースの本文が含まれていません',
  'storage.unreachable': 'このページはブラウザのストレージを使えないため、コメントは保存されません。',
  'storage.unreachableAdvice': ' タブを閉じる前に「プロンプトをコピー」で、書いた内容を手元に残してください。',
  'storage.unreadable': '以前このページで保存したコメントを読み込めなかったため、エクスポートした時点のレビューを表示しています。',
  'storage.unreadableAdvice': ' これから書くコメントは、読み込めなかった内容を上書きして保存されます。',
  'storage.unsaved':
    'このブラウザがコメントの保存を拒否しました(プライベートウィンドウ、サイトデータのブロック、または容量不足)。',
  'storage.unsavedAdvice': ' 今すぐ「プロンプトをコピー」で控えを取ってください。このタブを閉じると、書いた内容は残りません。',

  'app.help': 'ヘルプ: キー操作と画面ツアー',
  'help.tour': '画面ツアーをもう一度見る',
  'sidebar.label': 'このレビューのファイル',
  'sidebar.layoutLabel': 'diff の並べ方',
  'sidebar.readTitle': 'このレビューのファイルのうち、既読の印を付けたものの数',
  'sidebar.roundFiles.one': '{count} ファイル',
  'sidebar.roundFiles.other': '{count} ファイル',
  'sidebar.roundToggle': 'この diff のファイルを表示 / 非表示({date} に送信)',
  'sidebar.openComments': '未解決のコメント',
  'sidebar.additions': '追加された行',
  'sidebar.deletions': '削除された行',
  'diff.viewModeLabel': 'このファイルの表示方法',
  'comment.resolveTitle': '対応済みにします。コメントは薄く表示されたまま残ります',
  'comment.reopenTitle': 'このコメントを未解決に戻します',
  'comment.deleteTitle': 'このコメントを完全に削除します',
  'comment.deleteThreadTitle': 'このコメントを返信ごと完全に削除します',
  'comment.deleteReplyTitle': 'この返信を完全に削除します',
  'comment.replyTitle': 'このコメントに返信します。返信は同じ行のコメントの下に付きます',
  'comment.sendHintTitle': '⌘ + Enter(Mac 以外では Ctrl + Enter)で送信します',
  'preview.openInMo': 'このファイルを mo で新しいタブに開く',
  'preview.rendererLabel': 'プレビューを描画する方法',

  'tour.label': 'レビュー画面のツアー',
  'tour.step': '{at} / {total}',
  'tour.back': '戻る',
  'tour.next': '次へ',
  'tour.done': '完了',
  'tour.skip': 'ツアーをスキップ',
  'tour.files.title': 'レビュー対象のファイル',
  'tour.files.body':
    'このレビューに含まれるファイルが、送られた diff ごとに並びます。/ でパスと行を検索でき、読み終えたファイルには既読の印を付けられます。クリックするとそのファイルへ移動します。',
  'tour.diff.title': 'diff',
  'tour.diff.body':
    'ファイルごとに 1 つのセクションがあります。見出しから、折りたたみや、左右に並べるか 1 列で表示するかの切り替えができ、j と k でファイル間を移動できます。',
  'tour.preview.title': 'プレビュー',
  'tour.preview.body':
    'Markdown、notebook、画像、ソースファイルをここに描画し、diff のスクロールに追従します。Markdown では diff で追加された部分を緑で示し、削除された部分も元の位置に赤で表示します。ツールバーの ± で表示を切り替えられます。',
  'tour.panes.title': '表示幅を切り替える',
  'tour.panes.body':
    '< で diff を、> でプレビューを隠し、もう一方を全幅で表示します。隠した側は同じ場所のボタンで戻せます。間のバーはドラッグで幅を変えられ、ダブルクリックで既定の幅に戻ります。ファイル一覧の横の < も同じように一覧を隠します。',
  'tour.previewComment.title': 'プレビューにコメントする',
  'tour.previewComment.body':
    '描画された Markdown にもコメントできます。文章を選択して、横に出る「コメント」を押してください。コメントは選んだ文章の元になったファイルの行に付き、diff 側にも表示されます。',
  'tour.comment.title': '行にコメントする',
  'tour.comment.body':
    '行番号をクリックするとその行にコメントできます。複数行をドラッグするか Shift を押しながらクリックすると範囲を指定できます。キーボードなら Tab で行番号に移動して Enter を押します。',
  'tour.review.title': 'レビューを終える',
  'tour.review.body':
    '終わったら「レビューを送信」で、判定とともに待っている相手に伝えます。「プロンプトをコピー」は未解決のコメントをエージェント向けにクリップボードへコピーします。',
  'tour.review.bodyStatic':
    '「プロンプトをコピー」でコメントをクリップボードにコピーし、エージェントに貼り付けられます。エクスポートしたページでは、コメントはこのブラウザにだけ保存されます。',
  'tour.help.title': 'ヘルプはいつでもここに',
  'tour.help.body':
    'このボタンか ? キーで、すべてのショートカットの一覧と、このツアーをもう一度開けます。隣の設定ではテーマと言語を変えられます。',
}

export type Language = 'en' | 'ja'

/** LanguageSetting is what the reader picked: a language, or 'auto' to
 * follow the browser. */
export type LanguageSetting = 'auto' | Language

export const LANGUAGES: readonly Language[] = ['en', 'ja']

/** LANGUAGE_NAMES are what each language calls itself, which is the name a
 * reader looking for their own language recognises - so they are the same
 * in every catalog and do not live in one. */
export const LANGUAGE_NAMES: Record<Language, string> = { en: 'English', ja: '日本語' }

/** catalogs is exported for the test that holds the dictionaries to the same
 * keys and the same placeholders. */
export const catalogs: Record<Language, Record<MessageKey, string>> = { en, ja }

const LANGUAGE_KEY = 'sbnn.language'

/** detectLanguage picks the first language in the reader's own list that the
 * page speaks, by its primary subtag: ja-JP is Japanese, en-GB is English.
 * A list that names none of them gets English. */
export function detectLanguage(preferred: readonly string[]): Language {
  for (const tag of preferred) {
    const primary = tag.toLowerCase().split(/[-_]/)[0]
    if ((LANGUAGES as readonly string[]).includes(primary)) return primary as Language
  }
  return 'en'
}

/** browserLanguages is the reader's list, or nothing at all outside a page.
 * It is read off window rather than off the global: Node has a navigator of
 * its own these days, which reports the machine's locale, and following it
 * would make the test suite's English depend on where it runs. A test that
 * stands a window in has no navigator on it. */
function browserLanguages(): readonly string[] {
  if (typeof window === 'undefined') return []
  const nav = (window as { navigator?: Navigator }).navigator
  if (!nav) return []
  if (nav.languages && nav.languages.length > 0) return nav.languages
  return nav.language ? [nav.language] : []
}

function resolve(setting: LanguageSetting): Language {
  return setting === 'auto' ? detectLanguage(browserLanguages()) : setting
}

let setting: LanguageSetting = readEnumSetting<LanguageSetting>(LANGUAGE_KEY, ['auto', 'en', 'ja'], 'auto')
let current: Language = resolve(setting)
const listeners = new Set<() => void>()

/** language is the language the page is speaking now. */
export function language(): Language {
  return current
}

export function languageSetting(): LanguageSetting {
  return setting
}

function changed(): void {
  applyDocumentLanguage()
  for (const fn of listeners) fn()
}

/** setLanguageSetting switches the page and remembers the pick for this
 * browser. Storage may refuse, in which case the switch lasts for this page
 * view, as the theme does. */
export function setLanguageSetting(next: LanguageSetting): void {
  setting = next
  writeSetting(LANGUAGE_KEY, next)
  const resolved = resolve(next)
  if (resolved === current) {
    // The words are the same, but the menu's own pick is not.
    for (const fn of listeners) fn()
    return
  }
  current = resolved
  changed()
}

/** applyDocumentLanguage puts the language on <html lang>, which is what a
 * screen reader picks its voice by and what the browser hyphenates and
 * chooses CJK glyphs by. index.html and the exported page both ship lang="en";
 * this corrects it once the script knows better. */
export function applyDocumentLanguage(): void {
  if (typeof document === 'undefined' || !document.documentElement) return
  document.documentElement.lang = current
}

// A reader on 'auto' who changes the browser's language gets the page in it
// without a reload.
if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
  window.addEventListener('languagechange', () => {
    if (setting !== 'auto') return
    const next = resolve('auto')
    if (next === current) return
    current = next
    changed()
  })
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

/** version changes on every switch, the setting included, so that the hook
 * below re-renders a menu whose pick changed while the words did not. */
let version = 0
listeners.add(() => {
  version++
})

function snapshot(): number {
  return version
}

export type Params = Record<string, string | number>

function fill(template: string, params?: Params): string {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : whole,
  )
}

/** t looks a message up in the current language and fills in its
 * {placeholders}. A key the current catalog somehow lacks falls back to the
 * English, which the test suite is there to make unreachable. */
export function t(key: MessageKey, params?: Params): string {
  return fill(catalogs[current][key] ?? en[key], params)
}

/** dateTime writes a moment the way the page's language does. On 'auto' that
 * is the browser's own locale - an en-GB reader keeps their day-first dates -
 * and a language picked by hand brings its own format along with its words. */
export function dateTime(value: string | number | Date): string {
  return new Date(value).toLocaleString(setting === 'auto' ? undefined : current)
}

/** PluralBase is a key that comes as a `.one` and an `.other` pair. */
export type PluralBase = {
  [K in MessageKey]: K extends `${infer B}.one` ? B : never
}[MessageKey]

/** tn is t for a sentence that depends on a count. English has one form for
 * 1 and one for the rest; Japanese has one, so its pair says the same twice.
 * The count is passed to the message as {count}. */
export function tn(base: PluralBase, count: number, params?: Params): string {
  const key = `${base}.${count === 1 ? 'one' : 'other'}` as MessageKey
  return t(key, { count, ...params })
}

/** rich is t for a sentence with something other than text inside it - a
 * link, a <code> - so that the word order is the catalog's and not the
 * component's. Each {name} is replaced by the node of that name. */
export function rich(key: MessageKey, nodes: Record<string, ReactNode>): ReactNode {
  const parts: ReactNode[] = []
  const template = catalogs[current][key] ?? en[key]
  let last = 0
  for (const m of template.matchAll(/\{(\w+)\}/g)) {
    const at = m.index ?? 0
    if (at > last) parts.push(template.slice(last, at))
    parts.push(Object.prototype.hasOwnProperty.call(nodes, m[1]) ? nodes[m[1]] : m[0])
    last = at + m[0].length
  }
  if (last < template.length) parts.push(template.slice(last))
  // Passed as separate children rather than as an array, so React does not
  // ask for a key on each part.
  return createElement(Fragment, null, ...parts)
}

/**
 * useT makes a component follow the language. It returns t itself, so a
 * component reads `const t = useT()` and calls it like the module function;
 * what the hook adds is the subscription, which re-renders the component when
 * the language changes. That has to happen in every component that says
 * something, not only at the top: the stacks memoise their sections, and a
 * section would otherwise keep the words it rendered with.
 */
export function useT(): typeof t {
  useSyncExternalStore(subscribe, snapshot, snapshot)
  return t
}

/** useLanguage is the language itself, for a component that has to choose by
 * it rather than look a message up - the settings menu, a date. */
export function useLanguage(): { language: Language; setting: LanguageSetting } {
  useSyncExternalStore(subscribe, snapshot, snapshot)
  return { language: current, setting }
}
