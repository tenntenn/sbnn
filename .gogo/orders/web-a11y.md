slug: web-a11y

# 指示文 W-7 / web-a11y — キーボードと支援技術から届かない操作

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-a11y`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 2 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**
- 期限: **2026-08-25 中**
- 出す順: #67 → #68

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #67 | `t-a83621` | The tab's remove control is a `role="button"` span nested inside a button element |
| #68 | `t-00e180` | A comment cannot be started from the keyboard: the line gutters are pointer-only |

```bash
gogodash task set --id t-a83621 --status running --progress 30
gogodash task log --id t-a83621 --message "<1 行>"
gogodash task set --id t-a83621 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**専有（あなたが持ち主）**

- `web/src/components/DiffFileSection.tsx`

**共有（同じ波（G5）の web-search も触る。下の注意を守ること）**

- `web/src/components/Sidebar.tsx` — **#67 のためだけ。**
  `Sidebar.tsx` の持ち主は web-search（#99）である。
  触ってよいのは、**タブの削除コントロール**（現在の 210〜230 行付近、`role="button"` を持つ
  `<span>` とそれを包む `<button>`）とその周辺だけ。それ以外の行は 1 行も変えない。
  **`matchesPath` とフィルタ入力の周りには一切触らない。**
- `web/src/components/DiffFileSection.tsx` は**あなたが持ち主**だが、
  web-search が #100（構文ハイライト）のために**行の中身を描いている箇所だけ**を触る。
  その箇所を移動・改名しないこと。

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
  フォーカスリングの見た目を CSS で足したくなっても、**インラインスタイルで済ませる。**
- `web/src/App.tsx` — **web-nav と web-state だけが触れる。あなたは触れない。**
  キーボードの割り当てを増やしたくなっても `App.tsx` と `shortcuts.ts` は触らない（§5）。
- `web/src/shortcuts.ts` — web-nav の持ち物。
- `web/src/components/DiffStack.tsx` / `PreviewStack.tsx` / `PreviewFileSection.tsx` — web-perf の持ち物。
- `web/src/api.ts` / `web/src/main.tsx` — web-url の持ち物。
- `web/src/client.ts` / `web/src/storage.ts` / `web/src/markdown.ts` / `web/src/notebook.ts` /
  `web/src/wordDiff.ts` / `web/src/components/CommentThread.tsx` / `web/src/components/Icon.tsx` /
  `web/vite.config.ts` / `web/src/sectionKey.ts`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

### #67 — `<button>` の中に `role="button"` の `<span>` が入っている

- **入れ子をやめる。** ボタンの中にボタンは置けない（HTML として不正で、
  支援技術にも入れ子のボタンとしては伝わらない）。
- 直し方は **2 つのボタンを横に並べる**。タブ本体を選ぶボタンと、削除するボタンを兄弟にする。
  包む要素は `<div>` か `<li>` にして、**role を自分で付けない**。
- 削除ボタンには `aria-label`（例: `Remove this round`）を付ける。
  `title` 属性だけに頼らない。
- **`role="button"` を `<span>` に残さない。** 本物の `<button type="button">` にする。
- Tab キーで「タブ本体 → 削除」の順に届くこと。**`tabIndex` に正の数を使わない**（0 か -1 だけ）。
- **見た目を変えない。** 既存のクラス名をそのまま兄弟の要素に付け替える。
  レイアウトの調整が要るときはインラインスタイルで最小限にする（`styles.css` は触れない）。
  **G2 / G3 が同じ画面のスタイルを直しているので、クラス名を新設しない。**

### #68 — 行番号（gutter）がポインタでしか押せない

- 行番号セルを**キーボードから届く**ようにする。
  - `tabIndex={0}` と `role="button"` を行番号セルに付け、`aria-label` に
    どの行かが分かる文字列（例: `Comment on new line 120`）を入れる。
    **`<td>` を `<button>` に置き換えない**（表の構造が壊れ、差分の桁が崩れるため）。
  - `Enter` と `Space` で `onClick` と同じことが起きるようにする。`Space` は
    ページのスクロールを止める（`preventDefault`）。
  - フォーカスが見えるようにする。`outline` はインラインスタイルか
    `:focus-visible` 相当を JS で（`onFocus`/`onBlur` で state）付ける。**`styles.css` は触らない。**
- **行数が多いと Tab の回数が増える問題**は承知の上で、今回は「届くこと」を優先する。
  一括で飛ばすキー（`j`/`k` など）は `shortcuts.ts` / `App.tsx` の担当で、あなたは触れない。
  **この制約を PR 本文に 1 行で書く**:
  `Bulk keyboard navigation lives in App.tsx/shortcuts.ts (web-nav lane) and is not changed here.`
- web-nav が #91 で「コードセルのクリックで下書きを開かない、gutter には残す」方向に直している。
  **その方針と矛盾しないこと**（gutter が下書きの入口である、という前提は同じ）。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- このレーンは**ブラウザでしか確かめられない**種類の直しである。
  §7 の「手で確かめること」を実際にやり、**操作と結果を PR の `## Verification` に書く。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: this is keyboard/AOM behaviour that only a browser shows, and web/ has no JS test runner (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**2 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-a11y
git fetch -q origin main
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-a11y && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|App\.tsx|shortcuts\.ts|DiffStack\.tsx|Preview.*\.tsx|api\.ts|main\.tsx|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは components/DiffFileSection.tsx / components/Sidebar.tsx のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（「何も出力しないのが合格」は明記してある。それ以外は 1 行以上で合格）:

```bash
cd /home/user/wt/web-a11y
# ---- #67 ----
# role="button" の span が消えている（★ 何も出力しないのが合格 ★）
grep -n 'role="button"' web/src/components/Sidebar.tsx
# 本物のボタンになっている
grep -n '<button type="button"' web/src/components/Sidebar.tsx
grep -n 'aria-label' web/src/components/Sidebar.tsx
# 正の tabIndex を使っていない（★ 何も出力しないのが合格 ★）
grep -nE 'tabIndex=\{[1-9]' web/src/components/Sidebar.tsx
# Sidebar への変更が小さく収まっている（30 行を大きく超えていたら見直す）
git diff --stat origin/main..HEAD -- web/src/components/Sidebar.tsx
# ---- #68 ----
grep -n "tabIndex={0}" web/src/components/DiffFileSection.tsx
grep -n "onKeyDown" web/src/components/DiffFileSection.tsx
grep -nE "'Enter'|\"Enter\"" web/src/components/DiffFileSection.tsx
grep -nE "' '|'Space'|\"Space\"" web/src/components/DiffFileSection.tsx
grep -n "preventDefault" web/src/components/DiffFileSection.tsx
grep -n "aria-label" web/src/components/DiffFileSection.tsx
# 正の tabIndex を使っていない（★ 何も出力しないのが合格 ★）
grep -nE 'tabIndex=\{[1-9]' web/src/components/DiffFileSection.tsx
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-a11y && go run . --foreground
```

- #67: **マウスを使わずに** Tab だけでタブ一覧に入り、「タブ本体 → 削除」の順に止まり、
  Enter で削除が動くことを見る。DevTools の Elements で
  **`<button>` の中に `<button>` や `role="button"` が無い**ことを見て、その断片を PR に貼る。
- #67: DevTools の Console に警告（`validateDOMNesting` など）が **0 件**であることを見る。
- #68: **マウスを使わずに** 行番号へ Tab で届き、Enter と Space の両方でコメントの下書きが開き、
  Space でページがスクロールしないことを見る。フォーカスの枠が見えることも見る。
- 両方: DevTools の Accessibility ペインで、それぞれのコントロールの
  **name と role** が期待どおりであることを見て、値を PR に書く。

## 8. 止まらないための決め

- **担当外は触らない。** 特に `App.tsx` / `shortcuts.ts` / `styles.css` は 1 バイトも触らない。
  直したくなったら直さずに報告の「見送り / 疑義」へ書く。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-a11y | worktree /home/user/wt/web-a11y | issue #67 | PR #<番号> | branch gogo/issue-67 | commit <短縮 SHA> | <1 行要約>
...
```
