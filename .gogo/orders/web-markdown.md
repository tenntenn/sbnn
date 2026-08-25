slug: web-markdown

# 指示文 W-3 / web-markdown — 本文の描画（Markdown・ノートブック・単語差分）

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-markdown`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 3 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**
- 期限: **2026-08-25 中**
- 出す順: #65 → #150 → #162

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #65 | `t-6ae734` | Comment bodies are documented as Markdown but rendered as plain text in the browser |
| #150 | `t-0ea770` | Word-level diff highlighting can split an emoji in half |
| #162 | `t-2aaa7d` | SVG outputs in a notebook preview are built with a non-standard data URL |

```bash
gogodash task set --id t-6ae734 --status running --progress 30
gogodash task log --id t-6ae734 --message "<1 行>"
gogodash task set --id t-6ae734 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**専有（この指示文だけが触る。他のレーンとは 1 ファイルも重なっていない）**

- `web/src/markdown.ts`
- `web/src/notebook.ts`
- `web/src/wordDiff.ts`
- `web/src/components/CommentThread.tsx`

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
- `web/src/App.tsx` — **web-nav と web-state だけが触れる。あなたは触れない。**
  `#65` でコメント本文の描画を変えるとき、`App.tsx` 側の受け渡しを変えたくなっても
  **`CommentThread.tsx` の内側で完結させる**（§5）。
- `web/src/components/DiffFileSection.tsx` — web-nav の持ち物。
  **#150 の直しを `DiffFileSection.tsx` に入れない。`wordDiff.ts` の中で完結させる。**
- `web/src/api.ts` / `web/src/client.ts` / `web/src/storage.ts` /
  `web/src/components/Sidebar.tsx` / `web/src/components/DiffStack.tsx` /
  `web/src/components/PreviewStack.tsx` / `web/src/components/PreviewFileSection.tsx` /
  `web/src/components/Icon.tsx` / `web/src/main.tsx` / `web/vite.config.ts` /
  `web/src/shortcuts.ts` / `web/src/sectionKey.ts`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
  `marked` と `dompurify` は**既に入っている**ので、それを使う。
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

- **#65**: コメント本文を Markdown として描く。
  - **既存の `web/src/markdown.ts`（`renderMarkdown`）を使う。新しいレンダラを書かない。**
    `marked` + `dompurify` は既に依存に入っている。
  - 描画は `CommentThread.tsx` の中で行い、**必ず `dompurify` を通してから** `dangerouslySetInnerHTML`
    に渡す。素の HTML を通さない。
  - **編集中のテキストエリアは今までどおり生のテキストを持つ。** 描くのは確定表示のときだけ。
  - `suggestion`（```suggestion ブロック）の既存の扱いを壊さない。
    提案ブロックは今の見た目のまま残し、Markdown 化するのはそれ以外の本文である。
  - 見出し・箇条書き・コード・リンクが出れば十分。**目次や脚注のような拡張は入れない。**
  - `styles.css` を触れないので、**新しいクラス名を作らない。** 既存のクラスを使い回す。
- **#150**: 単語差分が絵文字を割らないようにする。
  - `Intl.Segmenter`（`granularity: 'grapheme'`）で書記素に分けてから比較する。
    Node 22 / 現行ブラウザにあるが、**無い環境では既存のコードパスへ落ちる**ようにする
    （`typeof Intl.Segmenter === 'function'` で分岐）。
  - 対象は絵文字だけではない。**サロゲートペア・結合文字・ZWJ 連結（👨‍👩‍👧 など）・
    地域指標（🇯🇵）・肌の色の修飾**を割らないこと。
  - `wordDiff.ts` の公開シグネチャを変えない（呼び出し側の `DiffFileSection.tsx` を触れないため）。
- **#162**: ノートブックの SVG 出力のデータ URL を標準の形にする。
  - `data:image/svg+xml;base64,<base64>` か
    `data:image/svg+xml;charset=utf-8,<encodeURIComponent した中身>` のどちらかにする。
    **`;utf8,` のような非標準の書き方を残さない。**
  - 既定は **base64** にする。ノートブックの SVG 出力は生の文字列で来ることが多く、
    パーセントエンコードの取りこぼしを考えなくて済むため。
  - 多バイト文字を含む SVG で `btoa` が落ちないようにする
    （`TextEncoder` でバイト列にしてから base64 にする）。
  - SVG は `dompurify` を通さずに `<img src=...>` として埋めること。
    **`<img>` の中に入れる限りスクリプトは走らない。** `innerHTML` で SVG を展開しない。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

このレーンは 3 本とも**純粋関数**なので、確認は必ず次の形で行う:

```bash
# 例。/tmp に書く。worktree の中には置かない（コミットしない）
cat > /tmp/check-worddiff.ts <<'EOF'
import { /* wordDiff の公開関数 */ } from '/home/user/wt/web-markdown/web/src/wordDiff.ts'
// 割れてはいけない例を並べ、期待と違ったら process.exit(1)
EOF
node --experimental-strip-types /tmp/check-worddiff.ts; echo "exit=$?"
```

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- **実際のコマンドと出力を PR の `## Verification` に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**3 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-markdown
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-markdown && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|App\.tsx|DiffFileSection\.tsx|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは markdown.ts / notebook.ts / wordDiff.ts / components/CommentThread.tsx のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（`grep` は 1 行以上返れば合格。ただし「何も返らないこと」を条件にした行は明記してある）:

```bash
cd /home/user/wt/web-markdown
# #65 既存のレンダラを使い、必ず sanitize している
grep -n "renderMarkdown" web/src/components/CommentThread.tsx
grep -nE "DOMPurify|sanitize" web/src/markdown.ts
# #65 素の HTML を通していない（sanitize を経ない dangerouslySetInnerHTML が無いこと）
grep -n "dangerouslySetInnerHTML" web/src/components/CommentThread.tsx
#    ↑ 出た各行の直前で sanitize 済みの値を渡していることを目で確かめ、PR に書く
# #150 書記素で分けている & 無い環境へ落ちる分岐がある
grep -n "Intl.Segmenter" web/src/wordDiff.ts
grep -n "typeof Intl.Segmenter" web/src/wordDiff.ts
# #162 非標準の data URL が残っていない（★ 何も出力しないのが合格 ★）
grep -n "image/svg+xml;utf8\|svg+xml;utf-8," web/src/notebook.ts
# #162 標準の形になっている
grep -nE "image/svg\+xml;base64|image/svg\+xml;charset=utf-8," web/src/notebook.ts
# #162 多バイトで btoa が落ちない
grep -n "TextEncoder" web/src/notebook.ts
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-markdown && go run . --foreground
```

- #65: 見出し・箇条書き・コード・リンクを含むコメントを投稿し、描かれることを見る。
  `<img src=x onerror=alert(1)>` を投稿して**何も起きない**ことを見る。
- #150: 絵文字（👨‍👩‍👧 / 🇯🇵 / 👍🏽）を含む行を 1 文字だけ変えた差分を読み込み、絵文字が割れないことを見る。
- #162: SVG 出力を持つ `.ipynb` をプレビューし、画像が出ることと
  DevTools の Console にエラーが 0 件であることを見る。

## 8. 止まらないための決め

- **担当外は触らない。** 直したくなったら直さずに報告の「見送り / 疑義」へ書く。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-markdown | worktree /home/user/wt/web-markdown | issue #65 | PR #<番号> | branch gogo/issue-65 | commit <短縮 SHA> | <1 行要約>
...
```
