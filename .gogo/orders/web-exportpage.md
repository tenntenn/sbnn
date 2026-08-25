slug: web-exportpage

# 指示文 W-4 / web-exportpage — 書き出したページ（`sbnn export`）のブラウザ側

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-exportpage`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 3 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**
- 期限: **2026-08-25 中**
- 出す順: #55 → #60 → #59

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #55 | `t-7b77d0` | Exported pages render icons as overlapping literal text — the header is illegible on the artifact path |
| #60 | `t-ee337a` | Comments left on an exported page can vanish without a word (localStorage quota, private window) |
| #59 | `t-c02378` | The export payload still uses the pre-rename `saVersion` key |

```bash
gogodash task set --id t-7b77d0 --status running --progress 30
gogodash task log --id t-7b77d0 --message "<1 行>"
gogodash task set --id t-7b77d0 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**専有（この指示文だけが触る。他のレーンとは 1 ファイルも重なっていない）**

- `web/src/client.ts`
- `web/src/components/Icon.tsx`
- `web/vite.config.ts`

**触ってはいけないファイル（例外なし。ここが他のレーンと違うので特に注意）**

- **`internal/export/export.go` と `internal/export/` 配下すべて。**
  #55 と #59 の issue 本文はどちらも Go 側（`export.readAssets` / `SaVersion`）を
  「Expected」に書いているが、**`internal/export/` は別レーン（export-pkg、issue #56 / #57 / #58）が
  同じサイクルで持っている。** あなたは Go を 1 バイトも触らない。
  代わりに §5 の方針でブラウザ側とビルド設定側から解く。**Go 側に残る分は報告へ書く。**
- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
  `@font-face` はこの中にあるが、**編集しない。**
- `web/src/App.tsx` — **web-nav と web-state だけが触れる。あなたは触れない。**
  #60 の告知はページ側から出す（§5）。
- `web/src/storage.ts` — web-state の持ち物。
  **`writeSetting` の握り潰しを直さない。**`client.ts` の中に静的ページ専用の書き込み経路を作る。
- `web/src/api.ts` / `web/src/markdown.ts` / `web/src/notebook.ts` / `web/src/wordDiff.ts` /
  `web/src/components/*.tsx`（`Icon.tsx` を除く）/ `web/src/main.tsx` / `web/src/shortcuts.ts`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
  **#55 はビルド出力を変える直しなので、ここを間違えやすい。**
  ビルドで確認したら必ず `git checkout -- web/dist` してからコミットする。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

### #55 — アイコンのフォントがページに入っていない

いま `web/src/styles.css` の `@font-face` が
`url('./assets/material-symbols-outlined-subset.woff2')` を指し、Vite が別ファイルとして
出力するため、ビルド後の CSS は `url(/assets/material-symbols-outlined-subset-*.woff2)` になる。
`file://` や埋め込み先ではこれが 404 になり、ligature 名がそのまま文字として出る。

**採る手は 2 つ、両方やる。**

1. **`web/vite.config.ts` でフォントを CSS の中へ base64 で埋め込む。**
   `build.assetsInlineLimit` を、この woff2（`web/src/assets/material-symbols-outlined-subset.woff2`、
   約 260 KiB）が確実に入る値にする。
   **全部の資産を無差別に大きく埋め込まない。** `assetsInlineLimit` は関数を取れるので、
   `.woff2` のときだけ `true` を返し、それ以外は既定（4096 バイト）に任せる形にする。
   これで `styles.css` を触らずに、ビルド後の CSS が `url(data:font/woff2;base64,...)` になる。
2. **`web/src/components/Icon.tsx` に、フォントが無いときの逃げ道を作る。**
   `document.fonts` が使えるときに `'sbnn Icons'` が読み込めたかを見て、
   読み込めていなければ**グリフ名の文字を出さない**（幅は固定のまま空にする）。
   `document.fonts` が無い環境では今までどおりに描く。
   **`aria-hidden="true"` は外さない。**

`internal/export/` の `readAssets` は触らない。**Go 側は export-pkg（#58）の担当である。**
PR 本文にこの 1 行を入れる:
`The Go side (export.readAssets) is owned by another lane (#58) and is untouched here; inlining is done at build time instead.`

### #60 — 書き出したページのコメントが黙って消える

`createStaticClient` は `readSetting` / `writeSetting` 経由で `localStorage` に置いているが、
`writeSetting` は失敗を握り潰す。ペイン幅なら正しいが、レビュー本文では正しくない。

- **`client.ts` の中に静的クライアント専用の読み書きを作る。** `storage.ts` は触らない。
  - 書き込みは `window.localStorage.setItem` を自前の `try/catch` で呼び、
    **失敗を捕まえたら握り潰さない。**
  - 読み込みの `JSON.parse` が失敗したときも、黙って `data.comments` へ戻さない。
    **「保存されていた内容が読めなかった」ことを同じ経路で知らせる。**
- **知らせ方**: `client.ts` から `document.body` に固定表示のバナー要素を 1 つ足す。
  - `App.tsx` を触れないので、React の外から `document.createElement` で作る。
  - スタイルはインラインで書く（`styles.css` を触れないため）。色は
    `var(--danger, #b00020)` / `var(--bg, #fff)` / `var(--fg, #111)` のように
    **既存のカスタムプロパティに fallback 付きで乗せる**（定義済みのものは
    `--accent --add-bg --add-strong --bg --bg-inset --bg-soft --border --danger --del-bg
    --del-strong --fg --fg-muted --mono --selected --warn`）。
  - 文言は英語で、次の 2 つを言う: 保存できていないこと / いま「Copy prompt」で控えを取ること。
  - バナーは 1 度だけ出す（同じ失敗で積み上げない）。閉じるボタンを付ける。
  - `console.error` も 1 回出す。
- ライブ（サーバあり）のページでは**この経路を通さない**。`isStatic` のときだけ。

### #59 — `saVersion` という古い鍵

Go 側の書き出しは `internal/export/export.go` にあり、**あなたは触れない。**
ブラウザ側だけでできることをやる。

- `web/src/client.ts` の `StaticPayload` に `sbnnVersion?: string` を足し、
  **`saVersion?: string` は残したまま**「読むときは `sbnnVersion` を優先し、無ければ `saVersion`」に
  する小さな関数を置く（issue の Expected が明示的に許している
  "keep accepting `saVersion` on read for one release" がこれに当たる）。
- `PayloadVersion` の bump と Go 側の改名は**やらない。** PR 本文にこの 1 行を入れる:
  `Reader side only: the writer (internal/export/export.go) is owned by another lane (#56-#58), so the Go rename and the PayloadVersion bump are not in this PR.`

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- #59 の「`sbnnVersion` を優先し無ければ `saVersion`」は純粋関数なので、
  `node --experimental-strip-types` で叩ける使い捨てスクリプトを**`/tmp` 配下**に書いて実行し、
  **コマンドと出力を PR の `## Verification` に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**3 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-exportpage
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-exportpage && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|App\.tsx|storage\.ts|^internal/|^cmd/|^web/(package\.json|pnpm-lock\.yaml)'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは web/src/client.ts / web/src/components/Icon.tsx / web/vite.config.ts のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認:**

```bash
cd /home/user/wt/web-exportpage/web
# ---- #55 ----
pnpm run build
# ビルド後の CSS にフォントが data URL で入っている（1 行以上返れば合格）
grep -o "url(data:font/woff2;base64," dist/assets/*.css | head -1
# 外部の woff2 参照が残っていない（★ 何も出力しないのが合格 ★）
grep -o "url(/assets/[^)]*woff2[^)]*)" dist/assets/*.css
# 別ファイルとしての woff2 が出力されていない（★ 何も出力しないのが合格 ★）
ls dist/assets/*.woff2 2>/dev/null
# woff2 以外を巻き込んで埋め込んでいない（js/css 以外の資産が dist に残っていることを確認）
ls dist/assets/
# 逃げ道が入っている
grep -n "document.fonts" ../web/src/components/Icon.tsx
# 確認が済んだら必ず捨てる
cd /home/user/wt/web-exportpage && git checkout -- web/dist && git status --porcelain -- web/dist

# ---- #60 ----
cd /home/user/wt/web-exportpage
grep -n "isStatic" web/src/client.ts
# 静的クライアントが自前の書き込み経路を持ち、失敗を握り潰していない
grep -n "console.error" web/src/client.ts
grep -n "document.createElement" web/src/client.ts
# storage.ts を触っていない（★ 何も出力しないのが合格 ★）
git diff --name-only origin/main..HEAD -- web/src/storage.ts

# ---- #59 ----
grep -n "sbnnVersion" web/src/client.ts
grep -n "saVersion" web/src/client.ts        # 互換のため残っているのが合格
git diff --name-only origin/main..HEAD -- internal/   # ★ 何も出力しないのが合格 ★
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-exportpage
go run . export --fragment -o /tmp/exported.html   # 実際のフラグは sbnn export --help で確かめる
```

- #55: `/tmp/exported.html` を `file://` で開き、ヘッダのアイコンがグリフとして出ることと、
  DevTools の Network に **404 が 0 件**であることを見る。390×844 の表示でも確かめる。
- #60: プライベートウィンドウ（またはサイトデータを止めたプロファイル）で書き出したページを開き、
  コメントを 1 件書いて**バナーが出る**ことを見る。通常のウィンドウでは**出ない**ことも見る。
- #59: 書き出したページが今までどおり開くこと（読み側の互換が壊れていないこと）を見る。

## 8. 止まらないための決め

- **担当外は触らない。** 特に `internal/export/` は**触らない。** 直したくなったら
  直さずに報告の「見送り / 疑義」へ書く（Go 側に残る作業はそこに列挙する）。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。
**`## 見送り / 疑義` には、`internal/export/` に残した Go 側の作業を issue 番号ごとに 1 行で書く。**

```
## 完了
- slug web-exportpage | worktree /home/user/wt/web-exportpage | issue #55 | PR #<番号> | branch gogo/issue-55 | commit <短縮 SHA> | <1 行要約>
...
```
