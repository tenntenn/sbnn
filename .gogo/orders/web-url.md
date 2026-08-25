slug: web-url

# 指示文 W-6 / web-url — アドレスバーとタブ名

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-url`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 2 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P2**
- 期限: **2026-08-25 中**
- 出す順: #95 → #94（#95 のほうが小さい。先に 1 本出してから #94 にかかる）

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #95 | `t-6bc607` | Every review tab is titled "sbnn", so several open reviews are indistinguishable |
| #94 | `t-5e504d` | The URL never says which file you are looking at, so a review cannot be linked to |

```bash
gogodash task set --id t-6bc607 --status running --progress 30
gogodash task log --id t-6bc607 --message "<1 行>"
gogodash task set --id t-6bc607 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**専有（この波（G5）のほかの 3 レーンとは 1 ファイルも重なっていない）**

- `web/src/main.tsx`
- `web/src/api.ts`
- `web/src/urlState.ts` — **新規に作る**
- `web/src/pageTitle.ts` — **新規に作る**

（`api.ts` は前の波（G4）の web-state も触っている。**G4 が先に終わってから G5 が動く**ので、
毎回 `origin/main` から切り直せば衝突しない。**必ず `git fetch -q origin main` を先にやること。**）

**触ってはいけないファイル（例外なし。ここがこのレーンの一番の制約）**

- **`web/src/App.tsx`** — **web-nav と web-state だけが触れる。あなたは触れない。**
  issue #94 の素直な直し方は `App.tsx` の `activeKey` を URL に出すことだが、**それはできない。**
  §5 の「React の外から DOM で解く」やり方でやる。
- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
- `web/src/components/` 配下すべて — 同じ波の web-perf / web-a11y / web-search の持ち物。
- `web/src/client.ts` / `web/src/storage.ts` / `web/src/markdown.ts` / `web/src/notebook.ts` /
  `web/src/wordDiff.ts` / `web/src/shortcuts.ts` / `web/vite.config.ts`
- `web/src/sectionKey.ts` — **読むだけ。変更しない**（鍵の作り方はここに合わせる）。
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
  **ルータのライブラリを入れない。**
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

### #95 — タブ名がどれも "sbnn"

- `web/src/pageTitle.ts` を新規に作る。中身は「いまのページのタイトル文字列を作って
  `document.title` に入れる」だけの小さな関数。
- タイトルは **`<グループ名> · sbnn`**。グループ名は
  - ライブのページ: `api.ts` の `groupFromLocation()`
  - 書き出したページ: `window.__SBNN_DATA__?.group`
  のどちらか。**`client.ts` を触らずに `window.__SBNN_DATA__` を直接見てよい**
  （型は `web/src/client.ts` の `StaticPayload` にあるので `import type` で借りる。
  `client.ts` は**読むだけ**で、書き換えない）。
- 呼び出しは `web/src/main.tsx` から、React を mount する前に 1 回。
- `document.title` の元の値（`index.html` の `<title>sbnn</title>`）は**変えない。**
  `web/index.html` は触らない。
- グループ名が `default` のときは `sbnn` だけにする（`default · sbnn` は読みにくいため）。

### #94 — URL がどのファイルを見ているか言わない

**`App.tsx` を触れないので、React の状態ではなく DOM を見て解く。** issue 本文が言うとおり、
各セクションには既に安定した DOM の id（`sectionKey` 由来、`d1:f1-abcd1234` の形）が付いている。

`web/src/urlState.ts` を新規に作り、次の 3 つだけをやる。

1. **書く**: スクロールに合わせて `location.hash` を、いま画面の上にあるセクションの id に更新する。
   - セクション要素は `document.querySelectorAll('[id]')` から、`sectionKey` の形に一致するものを拾う。
     **クラス名に依存しない**（`styles.css` を触れないし、G2 / G3 がクラスを触っている）。
   - 更新は `history.replaceState` で行う。**`pushState` を使わない。**
     スクロールのたびに履歴が積もると「戻る」が使い物にならなくなる。
   - スクロールは間引く（`requestAnimationFrame` か 150ms のスロットル）。
2. **読む**: 読み込み時と `hashchange` / `popstate` のときに、hash に対応する要素へスクロールする。
   - セクションはあとから mount されるので、**見つかるまで短く再試行する**
     （100ms ごとに最大 30 回 = 3 秒。それで見つからなければ諦めて何もしない）。
   - スティッキーツールバーに潜り込まないよう、ツールバーの高さぶん手前で止める。
     高さは `getBoundingClientRect().height` で実測する。
     **これは web-nav の #64 と同じ問題だが、`App.tsx` を触らずに `urlState.ts` の中で計算する。**
3. **壊さない**: hash が空・未知・形が違うときは**何もしない。** 例外を投げない。
   `groupFromLocation()` は `pathname` しか見ないので、hash を足してもグループの解決は壊れない
   （**これを実際に確かめて PR に書く**）。

- **行範囲（`:L120-L130`）まではやらない。** issue は "and ideally a line range" と書いているが、
  行の DOM を特定する手当てが `DiffFileSection.tsx`（別レーンの持ち物）に要る。
  **ファイル単位までを今回の範囲とし、行範囲は報告の「見送り / 疑義」へ書く。**
- 呼び出しは `web/src/main.tsx` から 1 回。React の mount の**あと**に始める。
- 書き出したページでも同じに動くこと（`urlState.ts` はサーバに触らないので自然にそうなる）。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- `pageTitle.ts` のタイトル生成と、`urlState.ts` の hash の読み書き（文字列を作る / 解く部分）は
  **DOM に触らない純粋関数として切り出し**、`node --experimental-strip-types` で叩ける
  使い捨てスクリプトを**`/tmp` 配下**に書いて実行し、**コマンドと出力を PR に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**2 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-url
git fetch -q origin main
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-url && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
# ★ App.tsx と components/ と styles.css と index.html が差分に無いこと。何も出力しないのが合格 ★
git diff --name-only origin/main..HEAD | grep -E 'App\.tsx|^web/src/components/|styles\.css|^web/index\.html|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'
git diff --name-only origin/main..HEAD
#   許されるのは web/src/main.tsx / web/src/api.ts / web/src/urlState.ts / web/src/pageTitle.ts のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（`grep` は 1 行以上返れば合格。「何も出力しないのが合格」は明記してある）:

```bash
cd /home/user/wt/web-url
# ---- #95 ----
test -f web/src/pageTitle.ts && echo ok
grep -n "document.title" web/src/pageTitle.ts
grep -n "· sbnn\|pageTitle" web/src/main.tsx
grep -n "__SBNN_DATA__" web/src/pageTitle.ts
# ---- #94 ----
test -f web/src/urlState.ts && echo ok
grep -n "replaceState" web/src/urlState.ts
# pushState を使っていない（★ 何も出力しないのが合格 ★）
grep -n "pushState" web/src/urlState.ts
grep -n "hashchange" web/src/urlState.ts
grep -n "popstate" web/src/urlState.ts
grep -n "getBoundingClientRect" web/src/urlState.ts
grep -n "urlState" web/src/main.tsx
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-url && go run . --foreground
```

- #95: 別のグループ名で 2 つのレビューを開き、**タブの名前が違う**ことを見る。
  グループが `default` のときは `sbnn` だけになることも見る。
- #94:
  1. 下へスクロールしてアドレスバーの `#` が変わることを見る。
  2. その URL をコピーして**新しいタブ**に貼り、同じファイルの位置で開くことを見る。
  3. **戻る / 進む**を押して、履歴がスクロールで埋まっていないことを見る。
  4. 存在しない `#deadbeef` を付けて開き、**何も壊れない**ことを見る。
  5. `sbnn export` で書き出したページでも 1〜4 が同じに動くことを見る。
  6. `groupFromLocation()` がグループを取り違えないこと（`/foo#...` で `foo` になること）を
     DevTools の Console で確かめ、出力を PR に貼る。

## 8. 止まらないための決め

- **担当外は触らない。特に `App.tsx` は 1 バイトも触らない。**
  触らないと直せないと判断したら、**直さずに報告の「見送り / 疑義」へ、
  何がどうしても `App.tsx` を要求するのかをコードの引用付きで書く。**
- 行範囲（`:L120-L130`）は §5 のとおり**今回の範囲外**。報告へ書く。
- **issue の前提が事実と違う / 再現しない**ときも、無理に直さず報告へ書き、**次の issue へ進む。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-url | worktree /home/user/wt/web-url | issue #95 | PR #<番号> | branch gogo/issue-95 | commit <短縮 SHA> | <1 行要約>
...
```
