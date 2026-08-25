slug: web-perf

# 指示文 W-5 / web-perf — 大きなレビューの初期描画とプレビューの再取得

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-perf`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 3 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**（#70 は不具合。#103 / #149 は P2 だがレーンとしては P1 で回す）
- 期限: **2026-08-25 中**
- 出す順: #70 → #103 → #149

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #70 | `t-dd50d6` | Every preview refetches on every server event, because the effect depends on the `file` object identity |
| #103 | `t-1ef0c4` | A large review mounts every file at once: 500 files is 130k DOM nodes and 12s to first render |
| #149 | `t-9c4f16` | The mo preview iframe is a fixed 80vh regardless of what is in it |

```bash
gogodash task set --id t-dd50d6 --status running --progress 30
gogodash task log --id t-dd50d6 --message "<1 行>"
gogodash task set --id t-dd50d6 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**専有（この波（G5）のほかの 3 レーンとは 1 ファイルも重なっていない）**

- `web/src/components/DiffStack.tsx`
- `web/src/components/PreviewStack.tsx`
- `web/src/components/PreviewFileSection.tsx`

（`DiffStack.tsx` は前の波（G4）の web-nav も触っている。**G4 が先に終わってから G5 が動く**ので、
毎回 `origin/main` から切り直せば衝突しない。**必ず `git fetch -q origin main` を先にやること。**）

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
  **#103 の issue 本文は `content-visibility: auto` を `.file-section` に足すことを勧めているが、
  `styles.css` は触れない。**インラインスタイルで同じことをする（§5）。
- `web/src/App.tsx` — **web-nav と web-state だけが触れる。あなたは触れない。**
  #103 が触れている `foldOverrides` / `viewModeOverrides` の刈り取りは `App.tsx` にあるので
  **やらない。報告の「見送り / 疑義」へ書く。**
- `web/src/components/DiffFileSection.tsx` — web-a11y の持ち物（同じ波）。
- `web/src/components/Sidebar.tsx` — web-search の持ち物（同じ波）。
- `web/src/api.ts` / `web/src/main.tsx` — web-url の持ち物（同じ波）。
- `web/src/client.ts` / `web/src/storage.ts` / `web/src/markdown.ts` / `web/src/notebook.ts` /
  `web/src/wordDiff.ts` / `web/src/components/CommentThread.tsx` / `web/src/components/Icon.tsx` /
  `web/vite.config.ts` / `web/src/shortcuts.ts` / `web/src/sectionKey.ts`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
  **仮想スクロールのライブラリを入れない。**
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

- **#70**: プレビューの取得 effect が `file` オブジェクトの同一性に依存しているため、
  サーバイベントのたびに作り直されて再取得している。
  - 依存配列を**オブジェクトではなく安定した文字列**にする
    （`groupName`、`diff.id`、`file.id`、必要なら `file` の内容の版を表す文字列）。
  - `sectionKey(diffId, fileId)`（`web/src/sectionKey.ts` は読むだけ）を鍵に使ってよい。
  - **中身が本当に変わったときは取り直す。** 「取らなくする」ではなく「同じものを取り直さない」。
    中身が変わったかは、サーバから来る更新時刻や版があればそれを使い、
    無ければ `file` の hunk 行数など安定した派生値で判断する。ここは自分で決めてよい。
    **決めた根拠を PR 本文に 1 段落で書く。**
- **#103**: 初期描画の重さを下げる。**やることは次の 2 つだけ。仮想スクロールは作らない。**
  1. **プレビューの無いファイルにプレビューのセクションを作らない**（`PreviewStack.tsx`）。
     issue の実測では、これだけでセクション数が半分になる。
  2. **差分のセクションにインラインスタイルで `contentVisibility: 'auto'` と
     `containIntrinsicSize` を付ける**（`DiffStack.tsx`）。
     `containIntrinsicSize` はそのファイルの行数から見積もった高さにする
     （例: 1 行 20px + ヘッダの固定分）。**当てずっぽうの固定値にしない。**
     `content-visibility` を知らないブラウザでは何も起きないだけなので、分岐は要らない。
  - **スクロール自体は今のままでよい。** issue の実測が「スクロールは問題ない」と言っている。
    `IntersectionObserver` の仕組みを作り替えない。
  - **効果を数字で示す。** 直す前と後で、同じレビューを開いたときの
    `document.querySelectorAll('*').length` と、`goto` から落ち着くまでの時間を測り、
    PR の `## Verification` に**表で**貼る。数字が出ていない PR は完了ではない。
- **#149**: mo のプレビュー iframe が中身に関係なく 80vh に固定されている。
  - iframe が**同一オリジン**なら、読み込み後に
    `contentDocument.documentElement.scrollHeight` を測って高さに反映する。
    `ResizeObserver` を中の `documentElement` に付けて追随させる。
  - **別オリジンや `contentDocument` が読めないときは、今までどおりの高さに落ちる。**
    例外を投げてページを壊さないこと（`try/catch` で囲む）。
  - 高さは**下限と上限で挟む**。下限 240px、上限は `min(90vh, 中身の高さ)`。
    中身が短いプレビューで巨大な空白が出ないこと、長いプレビューで
    iframe の中が二重スクロールにならないことの両方を満たす。
  - 高さはインラインスタイルで指定する（`styles.css` を触れないため）。
  - iframe を取り外すときに `ResizeObserver` を必ず切る。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- このレーンは**測ることが検証**である。§5 と §7 の測定を実際に行い、
  **コマンドと出力を PR の `## Verification` に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**3 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-perf
git fetch -q origin main
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-perf && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|App\.tsx|DiffFileSection\.tsx|Sidebar\.tsx|api\.ts|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは components/DiffStack.tsx / components/PreviewStack.tsx / components/PreviewFileSection.tsx のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（`grep` は 1 行以上返れば合格）:

```bash
cd /home/user/wt/web-perf
# #70 依存配列が安定した文字列になっている（file オブジェクトそのものを依存にしていない）
grep -nE "useEffect|useMemo" -A 3 web/src/components/PreviewFileSection.tsx | grep -n "\], \[" 
grep -n "sectionKey\|diff.id\|file.id" web/src/components/PreviewFileSection.tsx web/src/components/PreviewStack.tsx
# #103 プレビューの無いファイルを mount していない
grep -nE "previews\?\.|hasPreview|canPreview" web/src/components/PreviewStack.tsx
# #103 インラインで content-visibility を付けている
grep -n "contentVisibility" web/src/components/DiffStack.tsx
grep -n "containIntrinsicSize" web/src/components/DiffStack.tsx
# #149 中身の高さを測り、読めないときに落ちる分岐がある
grep -n "scrollHeight" web/src/components/PreviewFileSection.tsx
grep -n "ResizeObserver" web/src/components/PreviewFileSection.tsx
grep -n "catch" web/src/components/PreviewFileSection.tsx
```

**測ること（PR の `## Verification` に表で貼る。これが無い PR は未完成）**

```bash
cd /home/user/wt/web-perf && go run . --foreground
# ブラウザの DevTools Console で、直す前 / 直した後の両方で:
#   document.querySelectorAll('*').length
#   document.querySelectorAll('.file-section').length
#   performance.getEntriesByType('navigation')[0].duration
```

| レビュー | DOM ノード | セクション | 落ち着くまで |
|---|---|---|---|
| 直す前 |  |  |  |
| 直した後 |  |  |  |

- #70: DevTools の Network を開いた状態でコメントを 1 件足し、
  **プレビューの再取得が発生しない**ことを見る（リクエスト数を前後で書く）。
- #149: 短いプレビューと長いプレビューの両方で、余白が出ないこと・二重スクロールにならないことを見る。

## 8. 止まらないための決め

- **担当外は触らない。** `App.tsx` の `foldOverrides` / `viewModeOverrides` の刈り取りは
  **やらずに報告へ書く。**
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-perf | worktree /home/user/wt/web-perf | issue #70 | PR #<番号> | branch gogo/issue-70 | commit <短縮 SHA> | <1 行要約>
...
```
