slug: test-visual

# test-visual — 見た目の欠陥を機械に見つけさせる土台（issue #120 / #121 / #122）

優先度: #120 = P1、#122 = P1、#121 = P2
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `test-visual`）
  - `branch   = gogo/issue-<N>`（`<N>` は今扱っている issue 番号）
- **1 issue = 1 PR。** issue ごとに毎回 `origin/main` から切り直す。3 本の PR は独立に
  マージされるので、**3 本が同じファイルを触ってはいけない。** 下の表のとおりに分ける。
- **push と PR 作成まで行う。マージはしない。**

## 担当ファイル（これ以外は 1 バイトも触らない）

| issue | このブランチで触ってよいファイル |
|---|---|
| #120 | `test/visual/` 配下（新規ディレクトリ）**のみ** |
| #121 | `web/tokens_test.go`（新規）**のみ** |
| #122 | `web/contrast_test.go`（新規）**のみ** |

**#121 と #122 は同じ Go パッケージ（`package web`）にテストを足す。2 本は別々に
マージされるので、両方が同じ識別子を定義すると、マージ後にコンパイルが壊れる。**
そうならないよう、**トップレベルの識別子には必ず接頭辞を付ける**:

- `web/tokens_test.go` の中で定義するものはすべて `tokens` で始める
  （`tokensReadCSS` / `tokensDefined` / `tokensReferenced` / `TestTokensAllDefined` …）
- `web/contrast_test.go` の中で定義するものはすべて `contrast` で始める
  （`contrastReadCSS` / `contrastLuminance` / `contrastRatio` / `TestContrastTokens` …）
- `parseCSS` `lum` `ratio` `readStyles` のような**接頭辞の無い名前を定義しない。**
  片方が定義したものを、もう片方から使い回そうともしない（別々の PR なので存在しない）。

**3 本とも、次のファイルには絶対に触らない:**

- `web/package.json` / `web/pnpm-lock.yaml`（新しい npm 依存を足さない。
  他の作業者が `pnpm install --frozen-lockfile --offline` で走っており、
  ローカルストアに無いパッケージを lockfile に足すと全員のビルドが壊れる）
- `web/src/` 配下すべて（別レーンが実装中）
- `Taskfile.yml`（別ブランチ #127 と衝突する）
- `.github/workflows/` 配下（別レーン ci / release の担当）
- `web/dist/`（絶対にコミットしない）

この 3 本はどれも **CI への配線を含めない。** `.github/workflows/` は別レーンが同じ波で
作っているので、ここで触ると衝突する。配線は後続の PR に回す、と PR 本文に 1 行書く。

## issue #121 — 未定義のデザイントークンが黙って通る（タスク ID: t-49a286）

issue は stylelint / prettier / eslint / `tsc --noEmit` まで要求しているが、
**この PR ではそのうち「日に日に効く」と issue 自身が書いている 1 つだけを、依存ゼロで入れる。**

作るもの: `web/tokens_test.go`（`package web`。Go のテスト）。

やること:

1. `src/styles.css` を読む（テストは `web/` ディレクトリで走るので相対パスは `src/styles.css`）。
2. `var(--x)` で参照されているカスタムプロパティを全部集める。
3. `--x: ...` で定義されているカスタムプロパティを全部集める。
4. 参照されているのに定義されていないものが、**許可リスト以外に 1 つでもあれば失敗**させる。

許可リストはちょうど 4 つ。それぞれ理由をテストの中にコメントで書く:

| プロパティ | 理由 |
|---|---|
| `--diff-toolbar-h` | `src/components/DiffStack.tsx` が inline style で設定している。CSS 側に定義が無くて正しい |
| `--preview-toolbar-h` | `src/components/PreviewStack.tsx` が inline style で設定している。同上 |
| `--bg-elevated` | issue #81 の既知の欠陥。修正が入るまでの暫定 |
| `--ok-bg` | issue #81 の既知の欠陥。同上 |

- 許可リストは「未定義でも**よい**」という意味にする。「未定義で**なければならない**」にしない。
  #81 が直って定義が入ったときにこのテストが落ちてはいけない。
- `--bg-elevated` / `--ok-bg` の 2 つには、テスト内のコメントに `#81` と書いておく。
- **`web/src/styles.css` は直さない。** 未定義トークンを見つけても自分で定義を足さない。別レーンの担当。

参考（いまの実際の状態。これと違う結果が出たら、まずそちらを疑って報告に書く）:

```console
$ # 参照されているのに定義されていないもの
--bg-elevated
--diff-toolbar-h
--ok-bg
--preview-toolbar-h
```

完了条件:

```bash
test -f web/tokens_test.go
go test ./web/ -run Token -v          # PASS すること
gofmt -l web/tokens_test.go           # 何も返らないのが合格
go vet ./web/
grep -n "81" web/tokens_test.go       # #81 への言及が 1 行以上あるのが合格
git diff --name-only origin/main      # web/tokens_test.go だけが出るのが合格
# わざと壊して落ちることを確かめる（確かめたら元に戻す）
#   styles.css の :root に無いトークンを var() で 1 つ参照させ、go test が FAIL することを確認し、
#   確認できたら git checkout -- web/src/styles.css で必ず戻す
```

**「壊したら落ちること」を実際に確かめて、その出力を報告に貼る。** これを確かめていないテストは、
何も検査していないテストと区別がつかない。確かめたあと `git status` で
`web/src/styles.css` が変更されていないことを必ず確認する。

コミット / PR のフッタは `Refs #121`。issue は open のまま残す。
PR 本文と報告に、次の 3 点を書く:

1. 入れたのは「未定義トークン検査」だけ。stylelint による色リテラル / border-radius / box-shadow /
   `prefers-reduced-motion` の各ルールは、npm 依存を増やす必要があり、いまは lockfile を
   動かせないので入れていない。
2. issue が「`tsc --noEmit` はどこでも走っていない」と書いているのは**事実と違う**。
   `web/package.json` の `build` は `tsc -b && vite build` であり、型検査は既にビルドの一部。
   （`grep -n '"build"' web/package.json` の出力を根拠として貼る）
3. prettier / eslint の設定が無いのは事実。

## issue #122 — コントラストと a11y を自動で見る（タスク ID: t-c015e0）

issue の前半（コントラスト）は「ブラウザすら要らない、トークンの表と計算式だけ」と
issue 自身が書いている。**この PR ではその前半だけを、依存ゼロで入れる。**
後半（axe-core による構造チェック）は #120 のブラウザ土台の上でしか動かないので入れない。

作るもの: `web/contrast_test.go`（`package web`。Go のテスト）。

やること:

1. `src/styles.css` から 2 つのブロックを読み取る。
   - `:root {` … `}`（ライトテーマ）
   - `:root[data-theme='dark'] {` … `}`（ダークテーマ）
   `@media (prefers-color-scheme: dark)` の中身は `:root[data-theme='dark']` と同じ値なので、
   読み取るのは上の 2 つでよい（同じであることをテストで確かめてもよい）。
2. 前景 5 つ × 背景 6 つを総当たりする。
   - 前景: `--fg` `--fg-muted` `--accent` `--warn` `--danger`
   - 背景: `--bg` `--bg-soft` `--bg-inset` `--selected` `--add-bg` `--del-bg`
3. WCAG 2.x の相対輝度とコントラスト比を計算する（sRGB → 線形化 → `0.2126R+0.7152G+0.0722B`、
   比は `(L1+0.05)/(L2+0.05)`）。
4. しきい値は **4.5**（通常サイズの文字の AA）。
5. **いま 4.5 を下回っている組は既知として表に持ち、「今より悪くなったら落ちる」にする。**
   - 表に無い組が 4.5 未満 → 失敗。
   - 表にある組が、表に書いた値より下がった → 失敗。
   - 表にある組が 4.5 以上に改善した → 失敗させない。`t.Logf` で「改善した」と出すだけ。

参考（ライトテーマの実測値。小数第 2 位まで。これと違う値が出たら計算式を疑う）:

```
--fg        : bg 15.80  bg-soft 14.84  bg-inset 13.93  selected 14.66  add-bg 14.95  del-bg 13.78
--fg-muted  : bg  5.33  bg-soft  5.01  bg-inset  4.70  selected  4.95  add-bg  5.05  del-bg  4.65
--accent    : bg  5.19  bg-soft  4.88  bg-inset  4.58  selected  4.82  add-bg  4.92  del-bg  4.53
--warn      : bg  4.87  bg-soft  4.57  bg-inset  4.29  selected  4.52  add-bg  4.61  del-bg  4.24
--danger    : bg  5.36  bg-soft  5.03  bg-inset  4.72  selected  4.97  add-bg  5.07  del-bg  4.67
```

ライトで 4.5 を下回るのは `--warn` × `--bg-inset`（4.29）と `--warn` × `--del-bg`（4.24）の 2 つ。
ダークの値は自分で計算して、下回るものがあれば同じ扱いで表に入れる。
既知の表の各行には、対応する issue 番号（`#116` = トークンを文字用と塗り用に分ける提案）を
コメントで添える。

**`web/src/styles.css` は直さない。** 比が低いトークンを見つけても自分で色を変えない。別レーンの担当。

完了条件:

```bash
test -f web/contrast_test.go
go test ./web/ -run Contrast -v       # PASS すること
gofmt -l web/contrast_test.go         # 何も返らないのが合格
go vet ./web/
git diff --name-only origin/main      # web/contrast_test.go だけが出るのが合格
```

**ここでも「壊したら落ちること」を確かめる。** `styles.css` の `--fg-muted` を一時的に
背景に近い色（例: `#f0f0f0`）にして `go test ./web/ -run Contrast` が FAIL することを確認し、
出力を報告に貼り、`git checkout -- web/src/styles.css` で必ず戻す。戻したことを `git status` で確認する。

コミット / PR のフッタは `Refs #122`。issue は open のまま残す。
PR 本文と報告に、次の 2 点を書く:

1. axe-core による構造チェック（nested-interactive / button-name / aria-hidden-focus）は
   #120 のブラウザ土台が要るので入れていない。
2. issue が挙げている `#2da44e`（3.02、AA 未達）は**トークンではなくハードコードされた色**なので、
   このトークン総当たりの網には最初から掛からない。`grep -n "2da44e" web/src/styles.css` の
   出力を根拠として貼り、これは #76 の担当だと書く。

## issue #120 — 描画テストの土台がない（タスク ID: t-0ed64b）

作るもの: `test/visual/` に**それ自体で完結した Node プロジェクト**を 1 つ。
`web/package.json` には**触らない**（依存を web の lockfile に混ぜない。
この土台は開発用の道具であって、利用者に配られるバンドルの一部ではない）。

置くもの:

- `test/visual/package.json`（private。依存は `@playwright/test` だけ。スクリプト `test`）
- `test/visual/pnpm-lock.yaml`（`pnpm install` で生成されたもの）
- `test/visual/playwright.config.ts`
- `test/visual/fixtures/visual.diff` — issue が「リポジトリに置くべき」と書いている固定 diff。
  次のケースを全部含める: ドットファイルのパス（`.github/workflows/ci.yml` のようなもの）、
  括弧を含むパス、非常に長いパス、リネーム、新規追加ファイル、削除ファイル、
  バイナリ、相対画像を含む Markdown ファイル。
- `test/visual/geometry.spec.ts` — 幾何と computed style のアサーション。
  issue の表にある項目のうち、**まず次の 4 つ**を書く（少なくて構わない。動くことのほうが大事）:
  - 描かれた文字列が元のパス文字列と一致する（#73）
  - 子要素の幅がその親の幅以下（#119）
  - 各ビューポートで `document.scrollingElement.scrollWidth === clientWidth`（#74）
  - hover 状態と selected 状態の computed background が異なる（#79）
- `test/visual/README.md` — 走らせ方と、Chromium が要ることを書く。

土台の作り:

- サーバは `go run . --foreground` で起動する。ポートは固定せず、`--port` に空きポートを渡すか、
  起動時の標準出力から URL を取る。**どちらにするかは自分で読んで決める**
  （`cmd/root.go` と `cmd/server.go` を読めば決まる）。決めた方法を README に書く。
- 固定 diff は標準入力から流す（`git diff | sbnn` と同じ経路）。
- ビューポートは 1440×900 と 390×844 の 2 つ、カラースキームは light / dark の 2 つ。
  4 通りは Playwright の projects で回す。
- **`--dangerously-allow-remote-access` は使わない。** loopback のまま。

**Chromium がこの環境に無い場合の扱い（重要。ここで止まらない）:**

`npx playwright install chromium` は大きなダウンロードで、失敗しうる。失敗しても止まらない。
この PR の合格条件は「ブラウザが実際に走ったこと」ではなく「土台が組み上がっていること」にする。

```bash
cd test/visual
pnpm install                                    # ネットワークが要る。--offline は付けない
pnpm exec playwright test --list                # ブラウザ無しでも動く。spec が列挙されれば合格
```

`--list` が spec を列挙すれば、設定・TypeScript・spec の構文はすべて正しい。
そのうえで `pnpm exec playwright install chromium` を 1 回試し、
入ったら `pnpm test` まで走らせて結果を報告に貼る。入らなければ「入らなかった」と書く。
**走らせていないアサーションを「通った」と書かない。**

完了条件:

```bash
test -f test/visual/package.json
test -f test/visual/playwright.config.ts
test -f test/visual/geometry.spec.ts
test -f test/visual/fixtures/visual.diff
test -f test/visual/README.md
cd test/visual && pnpm exec playwright test --list     # spec が 1 つ以上列挙されるのが合格
# 固定 diff が本当に diff として読めること（sbnn 自身のパーサに通す）
cd /home/user/wt/test-visual && go run . --help >/dev/null
git diff --name-only origin/main | grep -v '^test/visual/'   # 何も返らないのが合格
git status --porcelain web/dist | head                       # 何も返らないのが合格
```

`test/visual/node_modules/` は**コミットしない**。`test/visual/.gitignore` に `node_modules/` を書く
（このファイルは `test/visual/` 配下なので担当範囲の中）。

コミット / PR のフッタは `Refs #120`。issue は open のまま残す。
PR 本文と報告に、次の 3 点を書く:

1. 依存を `web/package.json` ではなく `test/visual/` に閉じた理由（配られるバンドルに開発用の
   依存を混ぜないため、および他の作業者の `--frozen-lockfile --offline` を壊さないため）。
2. Chromium が実際に走ったかどうか。走っていないならそう書く。
3. CI への配線（`.github/workflows/` と `Taskfile.yml`）は、同じ波で別レーンが同じファイルを
   触っているため入れていない。

## 全体を通しての決まり

- 担当外のファイルは触らない。見つけた問題は自分で直さず報告に書く。
- 判断に迷って止まらない。既定を自分で決めて進み、決めた内容と理由を報告に書く。
- **issue へのコメントは書かない。** 判断と根拠（コードやコマンド出力の引用付き）を報告に書くだけ。メインが書く。
- Go を触った 2 本（#121 / #122）は COMMON.md の検証一式を必ず走らせる:
  `go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')`
- 報告には `slug` / branch / worktree / commit の 4 つを、この指示文と同じ綴りで書く。
  branch と commit は issue ごとに 3 組ある。
