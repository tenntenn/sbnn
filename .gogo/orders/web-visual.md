slug: web-visual

# 指示文 O-2 / レーン web-visual — トークン体系の上で見た目の不具合を潰す

優先度: **P2**（P1 のレーン web-tokens が main に入ってから始まる）
期限: 本波内（2026-08-25）
担当 issue（**この順にやる**）: #73 → #74 → #75 → #76 → #78 → #79 → #87 → #117 → #118 → #119
リポジトリ: tenntenn/sbnn / base = `main`

前提として `/home/user/briefs/COMMON.md` を先に読むこと。以下はそれを前提に、
差分だけを書いている。COMMON.md と食い違う箇所は**この指示文が優先**する
（食い違う点は「COMMON.md からの上書き」の節に全部書いてある）。

worktree = `/home/user/wt/web-visual`（`slug` から導出。他の名前を自分で作らない）
branch   = `gogo/issue-<N>`（issue ごと。**1 issue = 1 PR**）

---

## このレーンは前のレーンの上に建つ

`web/src/styles.css` は、**あなたの前にレーン web-tokens が持っていた。**
向こうは `styles.css` にトークン層（センチネル `/* ===== token layer start ===== */` と
`/* ===== token layer end ===== */` で囲まれた区画）を敷き終えて main に入っている。

**あなたはトークンを「使う」側である。新しいトークンを増やさない。**
`token layer start` と `token layer end` の間は**編集しない**。値も名前も変えない。
足りないと思ったら、増やす前に「既存のどれで表せるか」を先に見る。
どうしても表せないと判断したら、**増やさずに**報告の「見送り / 疑義」に
「どの issue で、どのトークンが足りないか」を書く。次の波で web-tokens 側が足す。

### 起動時の前提チェック（実装より先に、これを 1 回だけ走らせる）

```bash
cd /home/user/wt/web-visual
git fetch -q origin main
git show origin/main:web/src/styles.css | grep -c '^/\* ===== token layer end ===== \*/$'
git show origin/main:web/src/styles.css | grep -c -- '--pill-line-height:'
git show origin/main:web/src/styles.css | grep -c -- '--status-added-fg:'
```

3 つとも 1 以上なら前提を満たしている。**そのまま実装へ進む。**

満たしていなければ、まだ web-tokens が main に入っていない。
`git fetch -q origin main` を 2s → 4s → 8s → 16s → 32s の間隔で最大 5 回やり直す。
それでも満たさなければ、**トークンを自分で定義しない。**
実装を始めず、報告に `前提未達: token layer not in origin/main` とだけ書いて**即座に返す**。
勝手にトークンを足すと、次にこのファイルを触る全員と衝突する。

---

## 担当ファイル（ここから出ない）

- `web/src/styles.css` — **これ 1 本だけ**。ただし `token layer start`〜`end` の区画は除く

触ってはいけないもの（他のレーンが同時に持っている）:

- `web/src/App.tsx` … レーン web-nav が使用中
- `web/src/components/**` … G4 / G5 が使用中
- `web/src/*.ts` / `web/src/*.tsx` すべて、`web/dist/`、`go.mod`、`Taskfile.yml`、`.github/`

**担当 10 件はすべて CSS だけで閉じられると判断して割ってある**
（`direction: rtl` のパス折り返し、境界線の太さ、`opacity`、ピルの縦位置、幅の下限——
 いずれも `styles.css` の中にある）。
それでも TSX を直さないと閉じられない issue が出たら、**TSX を触らずに**、
COMMON.md の「issue がおかしいと思ったとき」に従って報告の「見送り / 疑義」に
**コードの引用つきで**書く。その 1 件で止めず、残りの issue は進める。

## ダッシュボードのタスク ID

| issue | task ID | 内容 |
|---|---|---|
| #73  | `t-cc5e7f` | サイドバーのパスが並べ替わって表示される（`.github/...` が `github/....` になる） |
| #74  | `t-ff56f9` | スマホで長いパスがプレビューヘッダのボタンを画面外へ押し出し、横スクロールする |
| #75  | `t-ac3cc8` | コメントスレッドが split 用にインデントされていて、unified で 52px ずれる |
| #76  | `t-9f82d3` | 「追加」の緑がハードコードで、ライトテーマで 3.02:1 とコントラスト不足 |
| #78  | `t-a0c153` | 「エージェントが書いた」印が出ない。太さ 0 の境界線に色を付けている |
| #79  | `t-9bf4bb` | 選択中ファイルの青い帯が何も言っていない（hover と同じ背景、隣の状態ドットと同じ色） |
| #87  | `t-7bec8a` | 解決済みコメントの `opacity: 0.6` が本文と一緒にボタンまで薄くする |
| #117 | `t-da2982` | ラベルピルの文字が縦中央に載らず、同じバッジが置き場所で違う位置に載る |
| #118 | `t-e88619` | コメント件数バッジが桁で幅の変わるいびつな楕円になる |
| #119 | `t-26b61b` | サイドバーの開閉ボックスが 9.6px 幅で、14px のグリフがタイトルへはみ出す |

完了報告にはこの ID をそのまま書く。ダッシュボードの更新はメインがやる。あなたは書かない。

---

## COMMON.md からの上書き（このレーン限定。理由つき）

1. **ブランチは積む（stacked）。**
   COMMON.md は「毎回 `origin/main` から切る」だが、このレーンは 10 件が**同じ 1 ファイル**を
   順に触る。毎回 main から切ると 10 本すべてが競合する。よってこうする:

   ```bash
   cd /home/user/wt/web-visual
   git fetch -q origin main
   git checkout -q -B gogo/issue-73 origin/main    # 最初の 1 本だけ origin/main から
   git checkout -q -B gogo/issue-74 gogo/issue-73  # 2 本目以降は直前の issue のブランチから
   git checkout -q -B gogo/issue-75 gogo/issue-74
   #  … 以下 #76 #78 #79 #87 #117 #118 #119 も同様に直前から
   ```

   PR の `base` は **`main` のまま**にする。PR 本文の先頭に 1 行:
   `Stacked on #<直前の PR 番号>. Merge in issue order: 73, 74, 75, 76, 78, 79, 87, 117, 118, 119.`
   最初の PR（#73）にはこの行の代わりに
   `First of the web-visual stack. Merge order: 73, 74, 75, 76, 78, 79, 87, 117, 118, 119.` と書く。

2. **push と PR 作成まで行う。マージはしない。**（COMMON.md と同じ。念のため明記）
   `git push -u origin gogo/issue-<N>` して `mcp__github__create_pull_request` で PR を立てるところまでが
   あなたの仕事である。**マージはメインがやる。あなたはしない。**
   push に失敗したら 2s → 4s → 8s → 16s でリトライ。

3. **テストが物理的に書けない件がほとんどである。**
   このレーンは CSS のみで、リポジトリにブラウザも視覚回帰の仕組みも無い
   （`web/package.json` に test スクリプトが無く、Playwright も入っていない。
   それを入れること自体が別 issue #120 #121 #122 で、別レーンの担当である）。
   よって**テストの代わりに、下の「検証」のシェルチェックを実際に走らせ、
   その出力を PR 本文の `## Verification` に貼る。** 「テストを書けない理由」も 1 行書く。

---

## 各 issue でやること

**着手前に必ず `mcp__github__issue_read` で本文を読むこと。** 再現手順と Expected が書いてある。
下の表は「どのトークンを使うか」と「終わったときに何が成り立つか」だけを決めている。

| issue | 使うトークン | 終わったときに成り立つこと（受け入れ条件） |
|---|---|---|
| #73 | （色は変えない） | パス末尾を見せるために `styles.css:453` あたりで使っている `direction: rtl` が、先頭の `.` を末尾へ回している。`direction: rtl` を残すなら**同じ規則に `unicode-bidi: plaintext` を必ず添える**（段落の向きが最初の強方向文字で決まるので `.github/...` が正しい順で出る）。残さないなら rtl をやめて別の方法で末尾を見せる。**どちらでもよいが「rtl だけ」は残さない** → [V4] が何も出さない |
| #74 | `--space-*` | プレビューヘッダのパスに `min-width: 0` と `overflow: hidden` + 省略を効かせ、ボタン列は縮まないようにする（`flex: none`）。ヘッダ自体は横に伸びない → [V5] のヘッダ規則が `min-width: 0` を持つ |
| #75 | `--space-*` | コメントスレッドのインデントを **split view のときだけ**効かせる。unified view では 0 になる。インデント値は `--space-*` から取る |
| #76 | `--status-added-fg` `--status-added-bg`（および removed / modified / renamed） | ハードコードの緑を消し、ファイル状態の色はすべて `--status-*` から取る。ライト・ダーク両方で前景と背景が **4.5:1 以上**（数字を [V7] で出す） |
| #78 | `--border-width-marker` `--accent-border` | エージェント印の境界線に**太さを与える**（`border-left-width: var(--border-width-marker)` と `border-left-style: solid`）。色だけ指定して太さ 0 の状態を残さない → [V6] が何も出さない |
| #79 | `--surface-selected` `--selected-marker` `--surface-hover` | 選択中ファイルの帯を `--selected-marker` にし、hover の背景 `--surface-hover` と選択の背景 `--surface-selected` を**見て区別が付く別の値**にする。帯の色が隣の状態ドット（`--status-*`）のどれとも一致しない |
| #87 | `--fg-muted` | 解決済みコメントの `opacity: 0.6` を**コンテナから外す**。薄くしたいのは本文なので、本文の文字色を `--fg-muted` にする。ボタン・リンクは薄くしない → 解決済みコメントを囲む規則に `opacity` が無い |
| #117 | `--pill-line-height` `--pill-padding-block` `--pill-padding-inline` | ラベルピルに `line-height: var(--pill-line-height)` と `display: inline-flex; align-items: center` を与え、上下 padding を `--pill-padding-block` に統一する。**置き場所によらず同じ位置に載る**（ピルの規則が場所ごとに padding を上書きしていない） |
| #118 | `--pill-min-width` `--pill-numeric` `--radius-pill` | 件数バッジに `min-width: var(--pill-min-width)`、`font-variant-numeric: var(--pill-numeric)`、`justify-content: center` を与える。**1 桁の数字はどれも同じ幅**になり、角丸は `--radius-pill` |
| #119 | `--space-*` | 開閉ボックス（chevron を入れる箱）に、中の 14px グリフが収まる幅と高さを明示する。`.icon` は 16px、`.icon.sm` は 14px なので、箱はそれ以上にする。タイトルへはみ出さない |

**#116 の傘に入っている #76 #79 #87 は、このレーンで閉じる。**
各 PR は担当 issue 1 件だけを `Fixes #<N>` で閉じる。他の issue 番号を `Fixes` に書かない。

---

## 検証（1 issue ごとに全部走らせ、出力を PR 本文に貼る）

```bash
cd /home/user/wt/web-visual/web
pnpm install --frozen-lockfile --offline && pnpm run build   # [V0] 終了コード 0
git checkout -- dist                                          # dist は絶対にコミットしない
```

```bash
cd /home/user/wt/web-visual
S=web/src/styles.css

# [V1] トークン層に手を入れていない → 何も出なければ合格
git diff origin/main -- $S | grep -nE '^[+-].*--(space|text|radius|border-width|motion|pill)-' \
  | grep -vE '^[+-].*var\('
awk '/^\/\* ===== token layer start/,/^\/\* ===== token layer end/' $S | md5sum
git show origin/main:$S | awk '/^\/\* ===== token layer start/,/^\/\* ===== token layer end/' | md5sum
#   → 上の 2 つの md5 が一致すること（トークン層が 1 バイトも変わっていない）

# [V2] 参照しているトークンが全部定義済み → 何も出なければ合格
diff <(grep -oE 'var\(--[a-z0-9-]+' $S | sed 's/var(//' | sort -u) \
     <(grep -oE '^[[:space:]]*--[a-z0-9-]+:' $S | tr -d ' :' | sort -u) | grep '^<'

# [V3] token layer end より下に色・角丸のリテラルを新しく足していない → 何も出なければ合格
sed -n '/^\/\* ===== token layer end ===== \*\/$/,$p' $S | grep -nE '#[0-9a-fA-F]{3,8}\b|rgba?\('
sed -n '/^\/\* ===== token layer end ===== \*\/$/,$p' $S | grep -nE 'border-radius:[^;]*[0-9]+(px|rem|em)'

# [V4] 裸の direction: rtl が残っていない（#73）→ 何も出なければ合格
grep -n 'direction:[[:space:]]*rtl' $S | while IFS=: read -r n _; do
  sed -n "$((n-8)),$((n+8))p" $S | grep -q 'unicode-bidi:[[:space:]]*plaintext' || echo "bare rtl at line $n"
done

# [V5] プレビューヘッダの規則が min-width: 0 を持つ（#74）
grep -nA12 'preview' $S | grep -c 'min-width:[[:space:]]*0'   # 1 以上

# [V6] 色だけ付いて太さ 0 の境界線が無い（#78）→ 何も出なければ合格
grep -nE 'border(-left|-right|-top|-bottom)?-color:' $S | while IFS=: read -r n _; do
  sed -n "$((n-10)),$((n+10))p" $S | grep -qE 'border[a-z-]*(-width|:)[^;]*[0-9]' || echo "colour-only border at line $n"
done
```

**[V7] コントラスト（#76。目視しない。算術で出す）**

`/tmp` に使い捨ての Node スクリプトを書き、`--status-added-fg` / `--status-added-bg` の比を
**ライト・ダーク両方で**計算し、数字を PR 本文の `## Verification` に貼る。**4.5:1 以上**が合格。
`--status-removed-*` `--status-modified-*` `--status-renamed-*` も同じ表に並べる。
**このスクリプトはコミットしない。**

（WCAG の相対輝度: 各チャンネル `c/255` を、`<=0.03928` なら `/12.92`、そうでなければ
`((c+0.055)/1.055)^2.4`、`L = 0.2126R + 0.7152G + 0.0722B`、比 `(L1+0.05)/(L2+0.05)`）

**[V8] 390px と 320px で横スクロールしないこと（このレーンの主要な受け入れ条件）**

ブラウザがこの環境に無い（Playwright も Chromium も入っていない）ので、ソース側で見る。

```bash
# 320px より広い固定幅を作っている規則を全部出す
grep -nE '(^|[^-])(min-)?width:[[:space:]]*(3[2-9][0-9]|[4-9][0-9]{2}|[0-9]{4,})px' $S
grep -nE '(^|[^-])(min-)?width:[[:space:]]*([2-9][0-9]|1[0-9]{2})rem' $S
# 横方向にはみ出しうる要素が、縮める余地を持っているか
grep -cE 'min-width:[[:space:]]*0' $S
grep -cE 'overflow-wrap|word-break|text-overflow' $S
```

出た固定幅の行を**1 行ずつ PR 本文に列挙し**、それぞれについて
「`max-width` でビューポートに対して頭打ちになっている」か
「`@media` / `@container` の内側で、狭い画面では効かない」かのどちらかを書く。
どちらでもない行が 1 行でもあれば、**それが 320px で横スクロールする原因なので直す**
（`max-width: min(<元の値>, 100vw - <左右の余白>)` の形にする）。

**[V9] ライトとダークの両方で確認できること**

`#76` `#78` `#79` `#87` で色を触った規則について、
**その色トークンが 3 つのテーマブロックすべてに定義されている**ことを示す。

```bash
for t in --status-added-fg --status-added-bg --surface-selected --surface-hover \
         --selected-marker --accent-border --fg-muted; do
  printf '%s %s\n' "$t" "$(grep -c -- "^[[:space:]]*$t:" $S)"   # 各行 3 なら合格
done
```

さらに、`data-theme` を切り替えても崩れないことを示すため、**あなたが触った規則の中に
テーマ判定（`prefers-color-scheme` や `[data-theme=`）を新しく書いていない**ことを示す:

```bash
git diff origin/main -- $S | grep -nE '^\+.*(prefers-color-scheme|\[data-theme)'   # 何も出なければ合格
```

テーマの分岐はトークン層が持つ。個々の規則が持ってはいけない。

## 完了条件（この 3 つが全部成り立ったとき、このレーンは終わり）

1. #73 #74 #75 #76 #78 #79 #87 #117 #118 #119 の**10 本の PR がすべて open で存在する**。
   各 PR は担当 issue 1 件だけを `Fixes #<N>` で閉じ、`base` は `main`。
2. 最後の PR（#119）のブランチ上で **[V0]〜[V9] がすべて上の合格条件を満たす**。
   とくに **[V1] の md5 が一致**（トークン層に触っていない）と
   **[V8] に未処理の固定幅が残っていない**の 2 つは必須。
3. 変更されたファイルが `web/src/styles.css` 1 本だけである:
   `git diff --name-only origin/main gogo/issue-119` が `web/src/styles.css` の 1 行だけを返す。

## 進め方の決め（迷って止まらないために、先に決めてある）

- **トークンは使うだけ。増やさない。名前も値も変えない。**
  足りなければ増やさずに報告へ書く（上の「このレーンは前のレーンの上に建つ」）。
- **`opacity` で「薄く見せる」をやめる。** #87 がその実例で、
  `opacity` は中の対話要素まで巻き込む。文字を薄くしたいときは `--fg-muted` を使う。
- **セレクタの構造を変えない。** TSX を触れないので、クラス名は増やせない。
  既存のクラスに対する規則の中で解く。既存クラスの組み合わせ（`.a .b`、`.a.b`）は使ってよい。
- 迷ったら**既定を自分で決めて進む。確認を上げて止まらない。** 決めた内容と理由は報告に書く。
- 担当外のファイルに問題を見つけても**直さない。** 報告の「見送り / 疑義」に書く。
- issue へのコメントは書かない（メインが書く）。マージはしない（メインがやる）。

## 報告（最終出力）

COMMON.md の「報告」の形式に加えて、**先頭に次の 4 行**を指示文と同じ綴りで書くこと。

```
slug: web-visual
branch: gogo/issue-73, gogo/issue-74, gogo/issue-75, gogo/issue-76, gogo/issue-78, gogo/issue-79, gogo/issue-87, gogo/issue-117, gogo/issue-118, gogo/issue-119
worktree: /home/user/wt/web-visual
commit: <各ブランチの先頭 commit を issue 番号つきで 10 行>
```

続けて、`## 完了` の各行に**ダッシュボードのタスク ID** を添える
（例: `- issue #73 | t-cc5e7f | PR #<番号> | branch gogo/issue-73 | <1 行要約>`）。
`## 検証` には [V0]〜[V9] の実際の出力を貼る。**「たぶん大丈夫」は書かない。**
前提チェックで止まった場合は、`前提未達: token layer not in origin/main` と、
何回リトライしたかを書く。
