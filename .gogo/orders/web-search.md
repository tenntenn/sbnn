slug: web-search

# 指示文 W-8 / web-search — 差分を探す（検索と構文ハイライト）

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-search`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 2 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P2**
- 期限: **2026-08-25 中**
- 出す順: #99 → #100

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #99 | `t-4997bb` | The filter searches paths only, so you cannot find the change you are looking for |
| #100 | `t-56b56b` | The diff has no syntax highlighting |

```bash
gogodash task set --id t-4997bb --status running --progress 30
gogodash task log --id t-4997bb --message "<1 行>"
gogodash task set --id t-4997bb --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**主担当（あなたが持ち主）**

- `web/src/components/Sidebar.tsx` — **あなたが持ち主。ただし web-a11y が #67 のためだけに
  タブの削除コントロールを触る**（下の「共有」を読むこと）
- `web/src/search.ts` — **新規に作る**（#99 の検索そのもの）
- `web/src/highlight.ts` — **新規に作る**（#100 のトークナイザ）

**共有（同じ波（G5）の web-a11y も触る。下の注意を守ること）**

- `web/src/components/DiffFileSection.tsx` — **#100 のためだけ。**
  持ち主は web-a11y（#68）である。触ってよいのは**行の中身を描いている箇所**だけ。
  **行番号セル（gutter）の `tabIndex` / `onKeyDown` / `role` / `aria-label` には
  一切触らない。** web-a11y がそこを直している。
- `web/src/components/Sidebar.tsx` は**あなたが持ち主**だが、web-a11y が #67 のために
  **タブの削除コントロール**（現在の 210〜230 行付近）だけを触る。
  そこを大きく作り替えるときは、そのボタンの JSX を移動・改名しない。

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
  **#100 は色が要るが、`styles.css` には書けない。**§5 のやり方で解く。
- `web/src/App.tsx` — **web-nav と web-state だけが触れる。あなたは触れない。**
  検索結果から飛ぶ導線は `Sidebar.tsx` の中で完結させる（§5）。
- `web/src/shortcuts.ts` — web-nav の持ち物。`/` の割り当ては既にあるので**変えない。**
- `web/src/components/DiffStack.tsx` / `PreviewStack.tsx` / `PreviewFileSection.tsx` — web-perf の持ち物。
- `web/src/api.ts` / `web/src/main.tsx` — web-url の持ち物。
- `web/src/client.ts` / `web/src/storage.ts` / `web/src/markdown.ts` / `web/src/notebook.ts` /
  `web/src/wordDiff.ts` / `web/src/components/CommentThread.tsx` / `web/src/components/Icon.tsx` /
  `web/vite.config.ts` / `web/src/sectionKey.ts`（読むだけ）
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
  **`highlight.js` / `shiki` / `prismjs` を入れない。**
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

### #99 — フィルタがパスしか見ない

- いま `Sidebar.tsx` にある `matchesPath` を**`web/src/search.ts` へ移す**（名前と挙動はそのまま）。
  そのうえで `search.ts` に「ファイル 1 件に対して、パスの一致と本文の一致を返す」関数を足す。
  - 本文は `Diff.files[].hunks[].lines[].content`。**既にクライアントのメモリにある。**
    サーバに問い合わせない。
  - **今の部分一致（substring）の意味を変えない。** あいまい検索にしない。
    空白区切りの語を**すべて含む**という現在の規則をそのまま本文にも当てる。
  - 大文字小文字は今までどおり無視する。
- サイドバーの表示:
  - 一致したファイルには**どこで当たったか**を出す（`— path` / `— 3 lines`）。
  - **本文だけで当たったファイルも一覧に出す。** いまは消えている。
  - 何も当たらないときの文言を直す。いまの「No path contains that.」は
    本文も見るようになると嘘になる（例: `Nothing matches that.`）。
  - 入力欄のプレースホルダ（現在 `Filter paths ( / )`）も実態に合わせる。
- **一致した行へ飛ぶ導線**: 一致件数をクリックしたら、その最初の一致行のあるファイルへ飛ぶ。
  飛ぶ先は既存のセクションの DOM id（`sectionKey` 由来）へ `scrollIntoView` する。
  **`App.tsx` を触れないので、`Sidebar.tsx` の中から DOM で飛ぶ。**
  行そのものへ飛ぶところまではやらなくてよい（ファイルまでで可）。
- **速さ**: 500 ファイルのレビューで打鍵ごとに全文を舐めると重い。
  - 検索は**入力から 120ms のデバウンス**をかける。
  - 1 回の検索で走査する行数に上限を置き（例: 20 万行）、超えたら
    「本文の検索は打ち切った」ことを画面に出す。**黙って部分的な結果を返さない。**
  - `useMemo` で、同じクエリと同じ diffs なら再計算しない。

### #100 — 差分に構文ハイライトが無い

- **`web/src/highlight.ts` を新規に作り、依存を足さずに自前で書く。**
  - 対応する言語は**拡張子で選ぶ**。最初は `.go` `.ts` `.tsx` `.js` `.jsx` `.json`
    `.py` `.sh` `.css` `.md` `.yaml` `.yml` の 12 種類に絞る。
    **知らない拡張子は色を付けない**（今までどおりの素の文字）。
  - トークンの種類は **5 つだけ**: コメント / 文字列 / 数値 / キーワード / それ以外。
    正規表現ベースの単純なトークナイザでよい。**完璧な構文解析を目指さない。**
  - 出力は「文字列とトークン種別の配列」。**HTML 文字列を返さない。**
    描くのは `DiffFileSection.tsx` で `<span>` を並べる形にする（`innerHTML` を使わない）。
  - **単語差分（`wordDiff`）の強調と喧嘩させない。** 単語差分が出ている行は
    今までどおり単語差分の見た目を優先し、構文ハイライトを重ねない。
    （`wordDiff.ts` は別レーンの持ち物なので**読むだけ**。）
- **色の付け方**（ここが一番間違えやすい）:
  - `styles.css` を触れないので、`highlight.ts` から `<style>` 要素を 1 つだけ
    `document.head` に足して、そこにトークンのクラスの色を書く。
  - **色は必ず既存のカスタムプロパティに fallback を付けて乗せる。**
    いま `styles.css` に定義があるのは次の 15 個:
    `--accent --add-bg --add-strong --bg --bg-inset --bg-soft --border --danger --del-bg
     --del-strong --fg --fg-muted --mono --selected --warn`
    例: `color: var(--fg-muted, #6b7280)`。
    **生の色を単独で書かない**（G2 / G3 がトークンを整理しているので、そこに乗る形にする）。
  - 割り当ての既定: コメント → `--fg-muted`、文字列 → `--add-strong`、
    数値 → `--warn`、キーワード → `--accent`、それ以外 → 継承（色を指定しない）。
  - **ライトとダークの両方で読めることを目で確かめる**（§7）。
- **速さ**: `#103` で初期描画が問題になっているので、
  ハイライトは**描くときに 1 行ずつ**行い、全ファイルを事前に処理しない。
  結果は行単位でメモ化してよい。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- `search.ts` と `highlight.ts` は**どちらも純粋関数**である。
  `node --experimental-strip-types` で叩ける使い捨てスクリプトを**`/tmp` 配下**に書いて実行し、
  **コマンドと出力を PR の `## Verification` に貼る。** 最低限、次を確かめる:
  - `search.ts`: 現行の `matchesPath` と同じ入力で同じ結果になること（挙動を変えていない証拠）。
  - `search.ts`: パスにだけ当たる例 / 本文にだけ当たる例 / どちらにも当たらない例。
  - `highlight.ts`: 12 種の各言語で 1 例ずつ、トークンの並びが期待どおりであること。
  - `highlight.ts`: 知らない拡張子で**トークンが 1 つ（それ以外）だけ**返ること。
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**2 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-search
git fetch -q origin main
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-search && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|App\.tsx|shortcuts\.ts|DiffStack\.tsx|Preview.*\.tsx|api\.ts|main\.tsx|wordDiff\.ts|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは components/Sidebar.tsx / components/DiffFileSection.tsx /
#   web/src/search.ts / web/src/highlight.ts のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（「何も出力しないのが合格」は明記してある。それ以外は 1 行以上で合格）:

```bash
cd /home/user/wt/web-search
# ---- #99 ----
test -f web/src/search.ts && echo ok
grep -n "matchesPath" web/src/search.ts
grep -n "hunks" web/src/search.ts          # 本文を見ている
grep -n "search" web/src/components/Sidebar.tsx
# 古い文言が消えている（★ 何も出力しないのが合格 ★）
grep -n "No path contains that" web/src/components/Sidebar.tsx
grep -n "Filter paths" web/src/components/Sidebar.tsx
# デバウンスと上限が入っている
grep -nE "120|setTimeout|debounce" web/src/components/Sidebar.tsx web/src/search.ts
grep -nE "limit|cap|truncat" web/src/search.ts
# ---- #100 ----
test -f web/src/highlight.ts && echo ok
# HTML 文字列を組み立てていない（★ 何も出力しないのが合格 ★）
grep -n "innerHTML\|dangerouslySetInnerHTML" web/src/highlight.ts web/src/components/DiffFileSection.tsx
# 色が既存のカスタムプロパティに乗っている
grep -nE "var\(--(fg-muted|add-strong|warn|accent)" web/src/highlight.ts
# 生の 16 進色を単独で書いていない（出た行はすべて var(...) の fallback の中であること。目で確認して PR に書く）
grep -nE "#[0-9a-fA-F]{3,8}" web/src/highlight.ts
# 12 種の拡張子を持っている
grep -nE "'go'|'tsx'|'yaml'" web/src/highlight.ts
# gutter のキーボード対応（web-a11y の担当）に触っていない（★ 何も出力しないのが合格 ★）
git diff origin/main..HEAD -- web/src/components/DiffFileSection.tsx | grep -E "^\+.*(tabIndex|onKeyDown|aria-label|role=)"
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-search && go run . --foreground
```

- #99: 差分の本文にしか出てこない識別子（例: `handleGroups`）で絞り込み、
  ファイルが一覧に出て `— 3 lines` のような内訳が出ることを見る。
  そのファイルへ飛べることを見る。何も当たらない語で新しい文言が出ることを見る。
- #99: 大きめのレビューで打鍵しても入力が引っかからないことを見る
  （DevTools の Performance で 1 打鍵ぶんの時間を測り、数字を PR に書く）。
- #100: `.go` `.ts` `.py` `.json` の差分を開き、色が付くことを見る。
  **ライトとダークの両方**で読めることを見る（スクリーンショットの説明を PR に書く）。
  拡張子の無いファイルで色が付かないことを見る。
  単語差分が出ている行で、単語差分の見た目が壊れていないことを見る。
- 両方: DevTools の Console にエラーが **0 件**であることを見る。

## 8. 止まらないための決め

- **担当外は触らない。** 特に `App.tsx` / `styles.css` / `wordDiff.ts` / `shortcuts.ts` は
  1 バイトも触らない。直したくなったら直さずに報告の「見送り / 疑義」へ書く。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- #100 は範囲が広がりやすい。**§5 に書いた 12 拡張子・5 トークンより広げない。**
  広げたくなったら、その理由を報告へ書いて次の回に回す。
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-search | worktree /home/user/wt/web-search | issue #99 | PR #<番号> | branch gogo/issue-99 | commit <短縮 SHA> | <1 行要約>
...
```
