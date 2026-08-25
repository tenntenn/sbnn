slug: cmd-flags

# 指示文 O-2 — `cmd/root.go` のフラグの取りこぼしと危険な既定を直す

## 前提

- 共通規約は `/home/user/briefs/COMMON.md`。**先に全部読むこと。** 以下はそれを前提に書いてある。
- 名前は上の `slug` からだけ導出する。**自分で別名を付けない。**

```
slug     = cmd-flags                   （このファイルの 1 行目。名前の出典はここだけ）
worktree = /home/user/wt/<slug>        →  /home/user/wt/cmd-flags
branch   = gogo/issue-<N>              →  issue ごとに 1 本。前の issue に積み上げない
```

- **1 issue = 1 PR。** 4 件あるので、ブランチ 4 本・PR 4 本になる。
- 着手はメインが合図してから。G1 の cmd-* 3 レーン（cmd-comment / cmd-reviews / cmd-wait）が
  終わるまでこのレーンは開始しない。

## このレーンは issue が 4 件ある（1 件は計画担当が移した）

**issue #49 はもともと cmd-hook レーンに割り当てられていたが、このレーンへ移した。**
理由を書いておく（勝手に戻さないこと）:

- #49 の直し場所は `cmd/root.go` の `run()` である。`--all` を見ているのはそこだけで、
  他に置ける場所がない（実際のコードで確認済み）。
- `cmd/root.go` は #142 / #160 / #161 でこのレーンが専有している。
  **2 人の作業者が同じファイルを別々の worktree で直すと、相互待ちとマージ衝突になる。**
- そのうえ **#160 の Expected は #49 を名指しで含んでいる**（"Reject `--all` without
  `--clear` (#49)"）。同じ数行を 2 レーンが別々に直すことになる。

ダッシュボードのタスク ID は issue に紐づくので変わらない（#49 = `t-1e39d9`）。
`/home/user/briefs/TASKIDS.tsv` の slug 列だけが `cmd-hook` のままなので、
**そこが食い違っている事実を報告に 1 行書くこと。** 直すのはあなたの仕事ではない。

## 優先度と期限

| 順番 | issue | 優先度 | 触る関数 | 期限 |
|---|---|---|---|---|
| 1 | #161 | **P0** | `readStdin()` | このサイクル内 |
| 2 | #142 | P1 | `parseLabels()` | このサイクル内 |
| 3 | #49 | P1 | `run()` + `--all` のヘルプ 1 行 | このサイクル内 |
| 4 | #160 | **P0** | `runClear()` + `init()` に `--yes` を追加 | このサイクル内 |

**この順番でやること。** 4 本とも `cmd/root.go` を触るが、上の「触る関数」が
**1 つも重ならないように割ってある。** 各ブランチは `origin/main` から切るので、
自分の担当関数の外を書き換えると 4 本の PR が互いに衝突する。**担当関数の外に出ないこと。**

## 担当 issue とダッシュボードのタスク ID

| issue | タスク ID | 一行 |
|---|---|---|
| #161 | `t-9f13a0` | `readStdin` が stat 失敗を「diff なし」にして、送ったつもりの diff が消える |
| #142 | `t-b50caa` | `--label` が前後の空白を残し、キー重複を黙って上書きする |
| #49 | `t-1e39d9` | `sbnn --all` が `--clear` 無しでも通り、何もしない |
| #160 | `t-cc4f20` | `sbnn --clear --all` が確認なしで全レビューを消す |

節目ごとに `gogodash task set --id <上の ID> --status running --progress <n>` を打つ。
終わったら `--status done --progress 100 --result "<1 行>"`。

## 触ってよいファイル

**この 2 つだけ。ここから 1 バイトも出ない。**

- `cmd/root.go`
- `cmd/root_test.go` （**新規作成**。いま存在しない）

**触ってはいけないもの（他レーンが使用中）:**

- `cmd/hook.go`（cmd-hook） / `cmd/comment.go`（cmd-comment） /
  `cmd/reviews.go` `cmd/reviews_test.go`（cmd-reviews） /
  `cmd/wait.go` `cmd/comments.go` `cmd/submit.go` `cmd/util.go`（cmd-wait）
- **`cmd/util.go` は特に注意。** `isTerminal()` はここにあるが、#160 では**読んで呼ぶだけ**で、
  中身を変えない。シグネチャも変えない。
- `internal/` 配下すべて / `version/` / `README.md` / `docs/`

---

## 1. issue #161（P0）— `readStdin` が stat の失敗を握りつぶす

`mcp__github__issue_read` で #161 を読んでから始めること（owner=tenntenn, repo=sbnn）。

### 確認済みの事実

`cmd/root.go:404`:

```go
func readStdin() (string, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", nil          // ← エラーを捨てて「入力なし」と同じ答えにしている
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	...
}
```

`run()` は `content == ""` なら `AddDiff` を呼ばずに URL を印字して 0 で終わる
（`cmd/root.go:247`）。つまり **diff が送られていないのに成功に見える。**

### やること

1. stat の失敗を**返す。** キャラクタデバイス判定（「何もパイプされていない」の正当な合図）は
   **そのまま残す。** 文言の例:

```go
return "", fmt.Errorf("cannot inspect stdin: %w", err)
```

2. テストできる形にする。いまの `readStdin` は `os.Stdin` を直に読むので試験できない。
   **引数を取る関数へ切り出し、`readStdin` はそれを `os.Stdin` で呼ぶだけにする。**

```go
func readStdin() (string, error) { return readDiff(os.Stdin) }

func readDiff(f *os.File) (string, error) { /* いまの中身。os.Stdin を f に置き換える */ }
```

   `maxDiffSize` の上限判定と 32MB のエラー文言は**そのまま維持する。**

3. **`run()` は触らない。** issue の最後の段落（「diff が空だったのか、そもそも
   パイプされていないのかを `run` が区別できない」）は**このPRの範囲外**である。
   理由: `run()` は #49 が触る。ここで触ると 2 本の PR が衝突する。
   **PR 本文に 1 行書いて済ませる**: the broader "empty diff vs no diff" change in `run` is
   out of scope here; it touches the same function as #49.

### テスト（`cmd/root_test.go`）

- 閉じた `*os.File` を渡すと**エラーが返る**こと（`os.Stdin.Stat()` の失敗を再現できる）。

```go
f, err := os.Open(os.DevNull)   // エラー処理は省かない
f.Close()
if _, err := readDiff(f); err == nil { t.Fatal("...") }
```

- `os.Pipe()` に "diff text" を書いて閉じ、読み側を渡すと中身がそのまま返ること。
- `maxDiffSize` を超える入力でエラーになること（`maxDiffSize+1` バイトを流す。
  32MB を実際に確保するのが重いなら、`readDiff` に上限を引数で渡す形にはせず、
  **このケースは省いてよい。省いた理由を PR 本文に 1 行書く**）。
- キャラクタデバイスのケースはテストで作れないので**書かなくてよい**（理由を PR 本文に 1 行）。

---

## 2. issue #142（P1）— `--label` の空白とキー重複

`mcp__github__issue_read` で #142 を読んでから始めること。

### 確認済みの事実

`cmd/root.go:425` の `parseLabels` は `strings.Cut` の結果をそのまま入れており、
trim もキー重複の検査もしていない。

### やること — 決めておいた既定（issue が両論併記なので、こちらで決めた）

`parseLabels` を次の規則にする。**この 4 つ以外は変えない。**

1. **キーと値の両方を `strings.TrimSpace` する。** trim は `strings.Cut` の**後**に行う
   （`"a=b=c"` が `a` → `b=c` に割れる既存の挙動を保つため）。
2. **trim した後にキーが空ならエラー。**（`"=1"`、`" = 1"`、`"a"` はすべてエラー）
3. **キーが重複したらエラー。キー名を文言に含める。** 上書きも「最後が勝つ」も採らない。
   理由: label は review と PR / リビジョンを結びつける唯一の手段で、issue が書いている
   とおり**黙って片方を捨てるのは、拒否するより悪い。**
4. **空の値は今までどおり受け入れる**（`"a="` → `map[a:]`）。issue の Measured 表でも
   これはエラー扱いされていない。**ここを勝手に厳しくしない。**

文言の例:

```go
return nil, fmt.Errorf("--label %q was given more than once", key)
```

5. `init()` の `--label` のヘルプ文（`cmd/root.go:168-169`）を、決めた規則が読み取れる
   ものに更新する。issue が「flag help currently says only `key=value kept with the diff,
   repeatable`, which does not settle it either way」と指摘している箇所である。
   **`--label` の行だけ**書き換える。同じ `init()` の中の他のフラグ行には触らない
   （`--all` の行は #49、`--yes` の追加は #160 が触る）。

### テスト（`cmd/root_test.go`）

issue の Measured 表をそのままテーブルにする。期待値は上の規則に合わせて更新する。

| 入力 | 期待 |
|---|---|
| `["a=1"]` | `map[a:1]` |
| `["a="]` | `map[a:]` |
| `[" a = 1 "]` | `map[a:1]` |
| `["a=b=c"]` | `map[a:b=c]` |
| `["a=1","a=2"]` | エラー。文言に `a` が含まれること |
| `["a=1"," a =2"]` | エラー（**trim してから重複を見ている**ことの確認。必ず入れる） |
| `["=1"]` | エラー |
| `[" = 1"]` | エラー |
| `["a"]` | エラー |
| `[]` | `nil, nil`（既存の挙動。維持する） |

---

## 3. issue #49（P1）— `--all` を `--clear` 無しで受け付けない

`mcp__github__issue_read` で #49 を読んでから始めること。

### 確認済みの事実

`clearAll` を読んでいるのは `runClear()` の中だけ（`cmd/root.go:376`）なので、
`sbnn --all` は `run()` の `switch`（`cmd/root.go:213-224`）を素通りして
「stdin を読んで diff を足して URL を出す」経路へ落ち、フラグは無視される。

### やること

1. **`run()` の中で手で検査する。** `switch` に入る**前**に置く:

```go
if clearAll && !doClear {
	return errors.New("--all only works with --clear (did you mean --clear --all?)")
}
```

   `errors` は既に import 済み（`cmd/root.go:6`）。

2. **`init()` に `MarkFlagsRequiredTogether("clear", "all")` を使わないこと。**
   これは**間違った直し方**である。`MarkFlagsRequiredTogether` は「片方が来たら両方必須」を
   意味するので、**`sbnn --clear` 単独（いちばん普通の使い方）がエラーになる。**
   issue はこの API を候補として挙げているが、採らない。上の手検査を採る。

3. `--all` のヘルプ文（`cmd/root.go:181`）を、必須の関係が読み取れるものにする。
   **この 1 行だけ触る。** `init()` の他の行には触らない。

### テスト（`cmd/root_test.go`）

`run()` は cobra とサーバに依存していて素で呼べないので、**検査そのものを小さな関数へ
切り出してテストする。**

```go
// validateClearFlags reports the error for --all without --clear.
func validateClearFlags(doClear, clearAll bool) error
```

`run()` の先頭からこれを呼ぶ。テストは 4 通り（`--clear --all` / `--clear` のみ /
`--all` のみ / どちらもなし）を表で回し、`--all` のみのときだけエラーになることを見る。
**`--clear` 単独がエラーにならないことを必ずケースに入れる**（上の 2. の罠の回帰テスト）。

---

## 4. issue #160（P0）— `sbnn --clear --all` に確認を足す

`mcp__github__issue_read` で #160 を読んでから始めること。**これは破壊的操作を扱う。**

### 確認済みの事実

`runClear()`（`cmd/root.go:371`）は `c.Status(ctx)` の**戻り値を捨てて**エラーだけを見ている:

```go
if _, err := c.Status(ctx); err != nil {
```

その `Status` には**必要な情報がすでに全部入っている**（`runStatus` が使っている）:

```go
// internal/server/server.go の GroupSummary
g.Name, g.URL, g.Diffs, g.Files, g.Comments, g.Unresolved
```

**`internal/client` にも `internal/server` にも、新しいメソッドを足す必要はない。**
`Status` の戻り値を受け取って使うだけでよい。

`isTerminal(f *os.File) bool` は `cmd/util.go` にある。**呼ぶだけ。中身を変えない。**

### やること — 方針は「足す方向」で（破壊的操作なので既存の破壊力を強めない）

1. `init()` に `--yes` を足す。

```go
f.BoolVar(&assumeYes, "yes", false, "Skip the confirmation of --clear")
```

   - パッケージ変数 `assumeYes bool` を既存の var ブロックに足す。
   - **短縮形（`-y`）は付けない。** 破壊的操作の迂回は明示的に打たせる。
   - **`init()` の中で触るのはこの 1 行の追加だけ。** `--all` の行（#49）と
     `--label` の行（#142）には触らない。

2. `runClear()` を書き換える。`c.Status(ctx)` の戻り値を受け取る。

   **`--clear --all` のとき:**
   - 消える対象を**先に名指しで出す。** グループ名と、開いているコメント数を並べる。
     `st.Groups` の `Name` / `Diffs` / `Comments` / `Unresolved` を使う。
   - そのうえで聞く。例:

```
sbnn: this will close every review on the server:
  default   3 diff(s), 2 comment(s), 1 open
  api       1 diff(s), 0 comment(s), 0 open
Close all 2 review(s)? [y/N]:
```

   - **`st.Groups` が空なら、聞かずにそのまま進む**（失うものが無いため）。

   **プレーンな `sbnn --clear -t <group>` のとき:**
   - `st.Groups` から該当グループを探し、**`Unresolved > 0` のときだけ**聞く。
     ブラウザ側の文言に合わせる（issue が引用している）:

```
Close the review of "api"? 1 comment(s) are still open and will go with it. [y/N]:
```

   - 該当グループが `st.Groups` に無い（すでに空）なら、聞かずに今までどおり進む。

   **聞かずに進んでよい条件（どちらも今までの挙動を壊さないため）:**
   - `assumeYes` が true
   - `!isTerminal(os.Stdin)` … パイプやジョブの中には答える人がいない。
     issue の Expected が明示している条件そのもの（"unless stdin is not a terminal
     (`isTerminal` already exists ...) or `--yes` is given"）。

3. **確認の入出力を、テストできる関数に切り出す。**

```go
// confirm asks the question and reports whether the answer was yes.
func confirm(in io.Reader, out io.Writer, question string) (bool, error)
```

   - `io` は既に import 済み（`cmd/root.go:8`）。
   - 質問文を `out` に書き、`in` から 1 行読む。`bufio.NewReader(in).ReadString('\n')` でよい。
   - **`strings.TrimSpace` して `strings.ToLower` し、`"y"` か `"yes"` のときだけ true。**
     それ以外（空行・EOF・`n`・何か別の文字列）は**すべて false。** 既定は「消さない」。
   - EOF は false を返してエラーにしない。
   - 実運用では `confirm(os.Stdin, os.Stderr, question)` として呼ぶ。
     **質問は stderr へ出す**（stdout は URL や JSON の出力に使われているため）。

4. **断られたときは何も消さず、終了コード 0 で終わる。**
   stderr に `sbnn: cancelled\n` と出して `return nil`。
   理由: ユーザが自分の意思で止めたのは失敗ではない。エラー終了にすると、
   確認付きにしただけでスクリプトが落ちるようになる。

5. 消したあとのメッセージは今までのものを維持する
   （`sbnn: closed %d review(s)` / `sbnn: closed the review of %q`）。

### テスト（`cmd/root_test.go`）

- `confirm` を表で回す。`in` は `strings.NewReader`。
  `"y\n"` `"Y\n"` `"yes\n"` `" y \n"` → true。
  `"n\n"` `"\n"` `""`（EOF） `"yolo\n"` → false。
  質問文が `out` に書かれていることも 1 ケースで見る。
- 「消える対象の一覧」を組み立てる部分も、`[]server.GroupSummary` を受けて
  文字列を返す純粋な関数に切り出してテストする。グループ 0 個 / 1 個 / 複数、
  `Unresolved` が 0 と 1 以上の両方を入れる。
- **`runClear` そのものを実サーバ相手にテストしなくてよい**（`httptest` で組むなら
  やってよいが必須ではない）。必須は上の 2 つ。

---

## 完了条件（**あなた自身が実行して真偽を判定できるもの**。実行結果を PR 本文と報告に貼る）

各ブランチで、そのブランチに入っている変更に対応する行を全部満たすこと。

```bash
cd /home/user/wt/cmd-flags

# 1. 共通（4 本すべて）。gofmt は「何も出ない」のが合格
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')

# 2. 共通: 担当外に出ていないこと
#    → cmd/root.go と cmd/root_test.go 以外を 1 つも出さないのが合格
git diff --name-only origin/main

# 3. #161: 1 行以上返ること
grep -n 'func readDiff(' cmd/root.go
grep -n 'cannot inspect stdin' cmd/root.go
#    #161: stat のエラーを捨てる行が消えたこと（**何も返らない**のが合格）
grep -n 'fi, err := os.Stdin.Stat()' cmd/root.go

# 4. #142: 1 行以上返ること
grep -n 'was given more than once' cmd/root.go
grep -n 'TrimSpace' cmd/root.go

# 5. #49: 1 行以上返ること
grep -n 'func validateClearFlags(' cmd/root.go
#    #49: 間違った API を使っていないこと（**何も返らない**のが合格）
grep -n 'MarkFlagsRequiredTogether' cmd/root.go

# 6. #160: 1 行以上返ること
grep -n 'func confirm(' cmd/root.go
grep -n '"yes"' cmd/root.go
grep -n 'isTerminal(os.Stdin)' cmd/root.go
#    #160: -y の短縮形を付けていないこと（**何も返らない**のが合格）
grep -n 'BoolVarP(&assumeYes' cmd/root.go
```

さらに**実際に叩いて確かめる**（#49 と #160 は特に。出力を報告に貼る）:

```bash
go run . --foreground &          # サーバを起動（終わったら kill する）
echo "diff --git a/x b/x" | go run . --target demo

# #49: --clear 無しの --all はエラーで、終了コードが 0 でないこと
go run . --all ; echo "exit=$?"
# #49: --clear 単独は今までどおり通ること（ここが壊れていないのが肝心）
go run . --clear --target demo ; echo "exit=$?"

# #160: 端末でないので聞かずに進むこと
echo "" | go run . --clear --all ; echo "exit=$?"
# #160: --yes で聞かずに進むこと
go run . --clear --all --yes ; echo "exit=$?"
```

## push と PR

- **push と PR 作成まで行う。マージはしない。**
- `git push -u origin gogo/issue-<N>` → `mcp__github__create_pull_request`。
  base は `main`、head は `gogo/issue-<N>`。本文の書き方は COMMON.md の「PR」の節に従う。
- **#160 の PR 本文には、確認を出す条件（`--yes` と 非 tty のとき聞かない）を必ず明記する。**
  破壊的操作の挙動を変える PR なので、読む人がその 2 つの抜け道を知らないまま
  マージすることがないようにする。
- **`web/dist/` はコミットしない。**

## 進め方の約束

- **担当外は触らない。** 直したくなる箇所を見つけても直さず、報告に書く。
  そのとき**コードを引用して**書く。issue へのコメントは書かない（メインの担当）。
- **上の 4 つの「触る関数」の外に出ない。** 出た瞬間に 4 本の PR が互いに衝突する。
- **判断に迷って止まらない。** ここに書いていないことが出てきたら、自分で既定を決めて進み、
  **決めた内容と理由を報告に書く。** 確認を上げてよいのは、それが無いと物理的に前へ進めない
  ものだけ。
- issue の前提が事実と違う / 再現しない / 仕様の決めが要る、のどれかなら
  **無理に直さず**、COMMON.md の「issue がおかしいと思ったとき」に従って報告へ回す。
- 完了報告には `slug` / branch / worktree / commit の 4 つを、**この指示文と同じ綴りで**書く。
  4 本ぶん並べること。
- 報告の最後に「#49 を cmd-hook から cmd-flags へ移した。TASKIDS.tsv の slug 列は
  `cmd-hook` のままで食い違っている」と 1 行書くこと。
