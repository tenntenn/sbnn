slug: web-tokens

# 指示文 O-1 / レーン web-tokens — styles.css にトークン体系を敷く

優先度: **P1**（このレーンの成果が web-visual レーンの前提になる。最優先）
期限: 本波内（2026-08-25）
担当 issue（**この順にやる**）: #116 → #80 → #81 → #82 → #83 → #84 → #85 → #86
リポジトリ: tenntenn/sbnn / base = `main`

前提として `/home/user/briefs/COMMON.md` を先に読むこと。以下はそれを前提に、
差分だけを書いている。COMMON.md と食い違う箇所は**この指示文が優先**する
（食い違う点は「COMMON.md からの上書き」の節に全部書いてある）。

worktree = `/home/user/wt/web-tokens`（`slug` から導出。他の名前を自分で作らない）
branch   = `gogo/issue-<N>`（issue ごと。**1 issue = 1 PR**）

---

## 担当ファイル（ここから出ない）

- `web/src/styles.css` — **これ 1 本だけ**

触ってはいけないもの（他のレーンが同時に持っている）:

- `web/src/App.tsx` … レーン web-nav が使用中
- `web/src/components/**` … G4 / G5 が使用中
- `web/src/*.ts` / `web/src/*.tsx` すべて、`web/dist/`、`go.mod`、`Taskfile.yml`、`.github/`

**`web/src/styles.css` は、あなたのあとにレーン web-visual が同じファイルを持つ。**
同時には持たない（直列）。あなたのぶんが main に入ってから web-visual が始まる。
だから**あなたはこのファイルの中で完結させる。**「あとで誰かが直すだろう」を前提にしない。

TSX を直さないと閉じられない issue があったら、**TSX を触らずに**、
COMMON.md の「issue がおかしいと思ったとき」に従って報告の「見送り / 疑義」に書く。
その 1 件で止めず、残りの issue は進める。

## ダッシュボードのタスク ID

| issue | task ID | 内容 |
|---|---|---|
| #116 | `t-3a228f` | トークン体系そのものを作る（このレーンの土台） |
| #80  | `t-7fdbd6` | `--accent` が 11 の意味を兼ねている |
| #81  | `t-d17042` | `--ok-bg` / `--bg-elevated` が参照されているのに未定義 |
| #82  | `t-2ffabb` | ラウンド見出しの `text-transform: uppercase` が送信者のタイトルを壊す |
| #83  | `t-bab8af` | 1 つの `.badge` が 6 種類の情報を兼ねている |
| #84  | `t-c114c7` | 浮いていないものに影、しかもダークテーマでは見えない |
| #85  | `t-4d0cc0` | 角丸が 8 種類、背後にスケールがない |
| #86  | `t-ba49a4` | 変化しないプロパティへの transition、`prefers-reduced-motion` なし |

完了報告にはこの ID をそのまま書く。ダッシュボードの更新はメインがやる。あなたは書かない。

---

## COMMON.md からの上書き（このレーン限定。理由つき）

1. **ブランチは積む（stacked）。**
   COMMON.md は「毎回 `origin/main` から切る」だが、このレーンは 8 件が**同じ 1 ファイル**を
   順に触る。毎回 main から切ると 8 本すべてが競合し、マージ担当が 8 回とも手で解く。
   よってこうする:

   ```bash
   cd /home/user/wt/web-tokens
   git fetch -q origin main
   # 最初の 1 本（#116）だけ origin/main から
   git checkout -q -B gogo/issue-116 origin/main
   # 2 本目以降は「直前の issue のブランチ」から
   git checkout -q -B gogo/issue-80 gogo/issue-116
   git checkout -q -B gogo/issue-81 gogo/issue-80
   #  … 以下 #82 #83 #84 #85 #86 も同様に直前から
   ```

   PR の `base` は **`main` のまま**にする。PR 本文の先頭に 1 行:
   `Stacked on #<直前の PR 番号>. Merge in issue order: 116, 80, 81, 82, 83, 84, 85, 86.`
   最初の PR（#116）にはこの行の代わりに
   `First of the web-tokens stack. Merge order: 116, 80, 81, 82, 83, 84, 85, 86.` と書く。

2. **push と PR 作成まで行う。マージはしない。**（COMMON.md と同じ。念のため明記）
   `git push -u origin gogo/issue-<N>` して `mcp__github__create_pull_request` で PR を立てるところまでが
   あなたの仕事である。**マージはメインがやる。あなたはしない。**
   push に失敗したら 2s → 4s → 8s → 16s でリトライ。

3. **テストが物理的に書けない件がほとんどである。**
   このレーンは CSS のみで、リポジトリにブラウザも視覚回帰の仕組みも無い
   （`web/package.json` に test スクリプトが無く、Playwright も入っていない）。
   よって**テストの代わりに、下の「検証」のシェルチェックを実際に走らせ、
   その出力を PR 本文の `## Verification` に貼る。** 「テストを書けない理由」も 1 行書く。

---

## #116 でやること — トークン層の確定（このレーンの土台。最初にやる）

issue #116 の本文（インベントリの表）を必ず読んでから始めること。以下はそれを実装に落とした形。

### 1. センチネルで層を囲む

`styles.css` の先頭に、下の 2 行を**この綴りのまま**入れる。以後の検証はこの 2 行を目印にする。

```css
/* ===== token layer start ===== */
/* ===== token layer end ===== */
```

`start` と `end` の間に**トークン定義だけ**を置く。セレクタは次の 4 つだけ:
`:root` / `@media (prefers-color-scheme: dark) { :root:not([data-theme='light']) }` /
`:root[data-theme='dark']` / `@media (prefers-reduced-motion: reduce) { :root }`。
`@font-face` と `.icon` は `end` より下へ動かす。

### 2. 色トークン（**3 つのテーマブロックすべてに同じ名前を定義する**）

| 種別 | トークン |
|---|---|
| 面 | `--bg` `--bg-soft` `--bg-inset` `--bg-elevated` |
| 文字 | `--fg` `--fg-muted` |
| 罫 | `--border` `--border-strong` |
| 対話色（#80） | `--accent-fg` `--accent-fill` `--accent-on-fill` `--accent-subtle` `--accent-border` |
| 差分行 | `--add-bg` `--add-strong` `--del-bg` `--del-strong` |
| ファイル状態（#76 がこれを使う） | `--status-added-fg` `--status-added-bg` `--status-removed-fg` `--status-removed-bg` `--status-modified-fg` `--status-modified-bg` `--status-renamed-fg` `--status-renamed-bg` |
| 選択と hover（#79 がこれを使う） | `--surface-hover` `--surface-selected` `--selected-marker` |
| 状態色 | `--ok-fg` `--ok-bg` `--warn-fg` `--warn-bg` `--danger-fg` `--danger-bg` |
| 影（#84） | `--shadow-overlay` |

決めごと:

- **`--accent` という名前は廃止する。** 用途ごとに上の 5 つへ割る。
  `--accent-fg` はテキスト／アイコン用、`--accent-fill` は塗り用。
  #116 が測っているとおり、ダークテーマの accent は**文字としては 6.11:1、
  白を載せた塗りとしては 3.1:1** なので、1 つのトークンでは兼ねられない。
  `--accent-on-fill` と `--accent-fill` のコントラストは**ライト・ダーク両方で 4.5:1 以上**にする。
- `--status-*` は「ファイルの状態チップ」用で、差分の行背景 `--add-bg` / `--del-bg` とは**別物**。
  同じ値にしない。`--status-added-fg` は**その隣で使う背景に対して 4.5:1 以上**にする
  （#76 が挙げている、ライトテーマ 3.02:1 のハードコード緑がこれで消える）。
- `--surface-selected` と `--surface-hover` は**見て区別が付く別の値**にする。
  `--selected-marker` は `--status-*` のどれとも違う色にする（#79）。
- `--shadow-overlay` は**3 つのテーマブロックすべてに定義**し、
  ダークテーマでは影だけでは見えないので `0 0 0 1px` のヘアラインを重ねた値にする（#84）。
- `--selected` `--warn` `--danger` `--accent` `--mono` の旧名は残さない。全参照を新名に置き換える。

### 3. 寸法トークン（`:root` に**1 回だけ**。テーマごとに再定義しない）

```
書体   --font-sans  --font-mono
字送り --text-2xs:11px --text-xs:12px --text-sm:13px --text-md:14px --text-lg:16px --text-xl:20px
余白   --space-2xs:2px --space-xs:4px --space-sm:6px --space-md:8px --space-lg:12px --space-xl:16px --space-2xl:24px
角丸   --radius-sm:4px --radius-md:6px --radius-lg:10px --radius-pill:999px
線幅   --border-width-hairline:1px --border-width-marker:2px --border-width-block:3px
動き   --motion-duration:120ms --motion-ease:cubic-bezier(0.2, 0, 0, 1)
ピル   --pill-line-height:1 --pill-padding-block --pill-padding-inline --pill-min-width --pill-numeric:tabular-nums
```

- **単位は px に寄せる。** `rem` / `em` は本文が読み手の設定に追従すべき箇所
  （Markdown プレビュー本文と、`min-width: 20rem` のダイアログ幅）だけに残し、
  それ以外の `0.8rem 0.85rem 1rem 0.92em 1.4em 1.8em` は上の `--text-*` に寄せる。
- `--pill-*` は #117 #118 が使う。`--pill-line-height: 1` と `--pill-numeric: tabular-nums` は
  「どこに置いても同じ高さで中央に載り、1 桁の数字が同じ幅になる」ためのもので、
  **web-visual レーンがこの名前を前提にしている。名前を変えない。**
- `prefers-reduced-motion` は**トークン層の中で**次の 1 ブロックだけで効かせる（#86）:

  ```css
  @media (prefers-reduced-motion: reduce) {
    :root { --motion-duration: 0ms; }
  }
  ```

  個々の規則に `prefers-reduced-motion` を書き足さない。

### 4. 既存の値をトークンへ移す

`token layer end` より下の規則から、**色・角丸・transition のリテラルを消す**。
`border-radius: 50%`（真円）だけは例外としてリテラルのままでよい。

---

## #80〜#86 でやること（#116 のあと。1 件 1 PR）

| issue | やること | 終わったときに成り立つこと |
|---|---|---|
| #80 | `--accent` の 11 用途を `--accent-fg` / `--accent-fill` / `--accent-on-fill` / `--accent-subtle` / `--accent-border` に割り直す。ファイル状態・選択・リンクが同じ青で被らないようにする | `styles.css` に `--accent:` も `var(--accent)` も 1 つも無い |
| #81 | `--ok-bg` と `--bg-elevated` を 3 テーマブロックすべてに定義する。あわせて「参照されているのに未定義のトークン」が 0 であることを検証で示す | 下の **[V2]** が何も出さない |
| #82 | ラウンド見出しの `text-transform: uppercase` を消す。送信者が付けたタイトルをそのまま出す。大きさの差は `--text-*` と `font-weight` で付ける | `grep -n 'text-transform: *uppercase' web/src/styles.css` が何も返さない |
| #83 | 1 つの `.badge` を種類ごとのクラスに割る。6 種類（少なくとも: ファイル状態 / コメント件数 / ラベル / ラウンド / 解決済み / エージェント印）にそれぞれ意味の分かるクラス名を与え、`--pill-*` を共通の土台にする。**TSX は触らないので、既存の `.badge` クラス名は残したうえで修飾クラスを足す形にする** | `.badge` を使う規則が種類ごとに分かれており、色が `--status-*` / `--ok-*` / `--accent-*` から来ている |
| #84 | 影は `--shadow-overlay` の 1 段階だけにする。**浮いているもの（ポップオーバー / メニュー / モーダル / トースト）にだけ**付け、浮いていない 3 箇所からは外す。ダークテーマでも見える値にする | `grep -cE 'box-shadow' web/src/styles.css` の各行が `var(--shadow-overlay)` か `none` だけ |
| #85 | 角丸 8 種を `--radius-sm/md/lg/pill` の 4 つ（+ 真円の `50%`）に畳む。`2px 7px 8px` は近い段へ寄せる | 下の **[V3]** が何も出さない |
| #86 | `transition` を「実際に変化するプロパティ」だけに絞り、`var(--motion-duration) var(--motion-ease)` で統一する。変化しないプロパティへの transition を消す | `grep -nE 'transition' web/src/styles.css` の全行が `var(--motion-duration)` と `var(--motion-ease)` を含む |

**#116 は #76 #79 #87 の傘でもあるが、それらは別レーン（web-visual）の担当である。
あなたの PR で `Fixes #76` / `Fixes #79` / `Fixes #87` と書かない。** 各 PR は担当 issue 1 件だけを閉じる。

---

## 検証（1 issue ごとに全部走らせ、出力を PR 本文に貼る）

```bash
cd /home/user/wt/web-tokens/web
pnpm install --frozen-lockfile --offline && pnpm run build   # [V0] 終了コード 0
git checkout -- dist                                          # dist は絶対にコミットしない
```

```bash
cd /home/user/wt/web-tokens
S=web/src/styles.css

# [V1] センチネルが 1 行ずつある（#116 以降すべての PR で 1 と 1）
grep -c '^/\* ===== token layer start ===== \*/$' $S
grep -c '^/\* ===== token layer end ===== \*/$'   $S

# [V2] 参照されているのに定義されていないトークンが無い → 何も出なければ合格（#81）
diff <(grep -oE 'var\(--[a-z0-9-]+' $S | sed 's/var(//' | sort -u) \
     <(grep -oE '^[[:space:]]*--[a-z0-9-]+:' $S | tr -d ' :' | sort -u) | grep '^<'

# [V3] token layer end より下に色と角丸のリテラルが残っていない → 何も出なければ合格
sed -n '/^\/\* ===== token layer end ===== \*\/$/,$p' $S | grep -nE '#[0-9a-fA-F]{3,8}\b|rgba?\('
sed -n '/^\/\* ===== token layer end ===== \*\/$/,$p' $S | grep -nE 'border-radius:[^;]*[0-9]+(px|rem|em)'

# [V4] 3 つのテーマブロックすべてに同じ色トークンがある
#      下の各行が 3 を返せば合格
for t in --bg --bg-elevated --fg --border --accent-fg --accent-fill --accent-on-fill \
         --status-added-fg --surface-selected --ok-bg --shadow-overlay; do
  printf '%s %s\n' "$t" "$(grep -c -- "^[[:space:]]*$t:" $S)"
done

# [V5] 定義したのに一度も使っていないトークンが無い → 何も出なければ合格
for t in $(grep -oE '^[[:space:]]*--[a-z0-9-]+:' $S | tr -d ' :' | sort -u); do
  grep -q -- "var($t" $S || echo "unused: $t"
done

# [V6] reduced-motion のガードがトークン層の中に 1 つだけある（#86）
grep -c 'prefers-reduced-motion' $S    # 1

# [V7] 旧トークン名が残っていない → 何も出なければ合格
grep -nE -- '--accent:|var\(--accent\)|--selected:|var\(--selected\)|--mono:|var\(--mono\)' $S
```

**[V8] コントラスト（#116 が「全部が算術である」と書いている部分。手で目視しない）**

`/tmp` に使い捨ての Node スクリプトを書き、次の 3 組を**ライト・ダーク両方で**計算し、
比の数字を PR 本文の `## Verification` に表で貼る。**このスクリプトはコミットしない。**

| 前景 | 背景 | 合格ライン |
|---|---|---|
| `--accent-on-fill` | `--accent-fill` | 4.5:1 以上 |
| `--accent-fg` | `--bg` | 4.5:1 以上 |
| `--status-added-fg` | `--status-added-bg` | 4.5:1 以上 |
| `--fg-muted` | `--bg-soft` | 4.5:1 以上 |

（WCAG の相対輝度: 各チャンネル `c/255` を、`<=0.03928` なら `/12.92`、そうでなければ
`((c+0.055)/1.055)^2.4`、`L = 0.2126R + 0.7152G + 0.0722B`、比 `(L1+0.05)/(L2+0.05)`）

**[V9] 320px / 390px で横スクロールしないこと（このレーンでは「壊さない」ことの確認）**

ブラウザがこの環境に無いので、ソース側で見る。

```bash
# 320px より広い固定幅を作っている規則を全部出す
grep -nE '(^|[^-])(min-)?width:[[:space:]]*(3[2-9][0-9]|[4-9][0-9]{2}|[0-9]{4,})px' $S
grep -nE '(^|[^-])(min-)?width:[[:space:]]*([2-9][0-9]|1[0-9]{2})rem' $S
```

出た行を**1 行ずつ PR 本文に列挙し**、それぞれについて
「`max-width` でビューポートに対して頭打ちになっている」か
「`@media` / `@container` の内側で、狭い画面では効かない」かのどちらかを書く。
どちらでもない行を**新しく増やさない**（既存の行の扱いを変える必要はない。増やさなければよい）。

## 完了条件（この 3 つが全部成り立ったとき、このレーンは終わり）

1. #116 #80 #81 #82 #83 #84 #85 #86 の**8 本の PR がすべて open で存在する**。
   各 PR は担当 issue 1 件だけを `Fixes #<N>` で閉じ、`base` は `main`。
2. 最後の PR（#86）のブランチ上で **[V0]〜[V7] がすべて上の合格条件を満たす**。
3. 変更されたファイルが `web/src/styles.css` 1 本だけである:
   `git diff --name-only gogo/issue-116~1 gogo/issue-86` が `web/src/styles.css` の 1 行だけを返す。

## 進め方の決め（迷って止まらないために、先に決めてある）

- **色の具体値はあなたが決めてよい。** 既存のライト/ダークのパレットを土台にし、
  [V8] のコントラストを満たす範囲で決める。決めた値と理由を PR 本文に 1〜2 行書く。
- **トークンの「名前」は上の表から変えない。** web-visual レーンがこの綴りを前提にしている。
  名前を変えると次のレーンが全部ずれる。値は自由、名前は固定。
- 迷ったら**既定を自分で決めて進む。確認を上げて止まらない。** 決めた内容と理由は報告に書く。
- 担当外のファイルに問題を見つけても**直さない。** 報告の「見送り / 疑義」に書く。
- issue へのコメントは書かない（メインが書く）。マージはしない（メインがやる）。

## 報告（最終出力）

COMMON.md の「報告」の形式に加えて、**先頭に次の 4 行**を指示文と同じ綴りで書くこと。

```
slug: web-tokens
branch: gogo/issue-116, gogo/issue-80, gogo/issue-81, gogo/issue-82, gogo/issue-83, gogo/issue-84, gogo/issue-85, gogo/issue-86
worktree: /home/user/wt/web-tokens
commit: <各ブランチの先頭 commit を issue 番号つきで 8 行>
```

続けて、`## 完了` の各行に**ダッシュボードのタスク ID** を添える
（例: `- issue #116 | t-3a228f | PR #<番号> | branch gogo/issue-116 | <1 行要約>`）。
`## 検証` には [V0]〜[V9] の実際の出力を貼る。**「たぶん大丈夫」は書かない。**
