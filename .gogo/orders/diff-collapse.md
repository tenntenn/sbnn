slug: diff-collapse

# 指示文 O-4 — `--collapse` のパターン照合と `shorten()` を直す

## 前提

- 共通規約は `/home/user/briefs/COMMON.md`。**先に全部読むこと。** 以下はそれを前提に書いてある。
- 名前は上の `slug` からだけ導出する。**自分で別名を付けない。**

```
slug     = diff-collapse               （このファイルの 1 行目。名前の出典はここだけ）
worktree = /home/user/wt/<slug>        →  /home/user/wt/diff-collapse
branch   = gogo/issue-<N>              →  issue ごとに 1 本。前の issue に積み上げない
```

- **1 issue = 1 PR。** 2 件あるので、ブランチ 2 本・PR 2 本になる。
- 着手はメインが合図してから。

## ファイルの所在について（重要。依頼時の記述が事実と違っていた）

**この 2 件は `internal/diff/parse.go` ではない。`internal/server/fold.go` である。**
計画担当が実際のコードを読んで確認した:

```
internal/server/fold.go:97   func matchDoubleStar(pattern, p string) bool   ← issue #18
internal/server/fold.go:120  func shorten(s string, n int) string           ← issue #19
```

`internal/diff/parse.go` は diff-parse レーンが使用中である。**開かないこと。**
`fold.go` は `internal/diff` の `GeneratedMarker` / `VisibleTop` を**呼んでいる**が、
呼ぶだけで、今回は `internal/diff` 側を一切変更しない。

## 優先度と期限

- issue #18 … 優先度 P1（bug。よく使う形のパターンが全く効かない）。期限: このサイクル内
- issue #19 … 優先度 P2（bug。非 ASCII のときだけ文字化けする）。期限: このサイクル内
- **#18 を先にやる。**

## 担当 issue とダッシュボードのタスク ID

| issue | タスク ID | 一行 |
|---|---|---|
| #18 | `t-3e9bcc` | `**` が 2 つあるパターン（`**/testdata/**`）が何にも当たらない |
| #19 | `t-89ff6d` | `shorten()` が UTF-8 を途中で切って fold の理由が壊れる |

節目ごとに `gogodash task set --id <上の ID> --status running --progress <n>` を打つ。
終わったら `--status done --progress 100 --result "<1 行>"`。

## 触ってよいファイル

**この 2 つだけ。ここから 1 バイトも出ない。**

- `internal/server/fold.go`
- `internal/server/fold_test.go`

**触ってはいけないもの（他レーンが使用中、または別 issue の担当）:**

- `internal/diff/parse.go` … **diff-parse レーンが専有している。**
- `internal/server/` の他のファイルすべて。**特に `server.go` `store.go` `preview.go`
  `hook.go` `proxy.go` `prompt.go` `spa.go` と、それらの `_test.go`。**
  srv-core / srv-api / srv-hook / srv-preview レーンが使う。
- `cmd/` 配下すべて（cmd-* の 5 レーンが使用中）
- `README.md` / `docs/` … **`--collapse` のドキュメントは issue #52（doc-collapse レーン）の
  担当である。** パターンの説明を直したくなっても直さない。報告に書く。

## テストファイルの書き分け（2 本の PR が衝突しないための約束）

`internal/server/fold_test.go` を 2 本の PR がどちらも触る。各ブランチは `origin/main` から
切るので、**同じ場所に書き足すとマージで衝突する。** 場所を分けてある:

- **#18 のテストは、既存の `TestCollapsePatterns` の `cases` テーブルに追記する。**
  新しい関数を作らない。
- **#19 のテストは、ファイル末尾に新しい関数 `TestShortenKeepsRunesWhole` を足す。**
  既存の関数には一切触らない。

---

## 1. issue #18（P1）— `**` を何個でも扱えるようにする

`mcp__github__issue_read` で #18 を読んでから始めること（owner=tenntenn, repo=sbnn）。

### 確認済みの事実（実際に読んで追ってある）

```go
// internal/server/fold.go:97
func matchDoubleStar(pattern, p string) bool {
	prefix, suffix, _ := strings.Cut(pattern, "**")   // ← 最初の ** で切って終わり
	...
	for {
		if ok, _ := path.Match(suffix, rest); ok {    // ← suffix の中の ** は
			return true                               //    path.Match には
		}                                             //    ただの * 2 個であり、
		_, after, found := strings.Cut(rest, "/")     //    / をまたげない
		...
	}
}
```

`"**/testdata/**"` は prefix `""`、suffix `"testdata/**"` になり、
`path.Match("testdata/**", ...)` はパスの深さが合わないので**どの段でも false** になる。
issue の Reproduce のとおりである。

### やること

`matchDoubleStar` を、**`**` を何個でも、どこにでも書けるように**書き直す。
実装の選び方は任せる（`strings.Split(pattern, "**")` して各断片を順に照合する、
正規表現へ変換する、再帰で照合する —— どれでもよい）。
**満たすべき挙動は下のテーブルで固定する。ここが完了条件である。**

- `matchPath` / `matchAny` / `foldFiles` の**シグネチャは変えない。**
- `matchAny` が空パターンを読み飛ばす挙動（`fold.go:70-73`）は**そのまま維持する。**
- `**` を含まないパターンの経路（`matchPath` の後半）は**触らない。**

### テスト — `TestCollapsePatterns` の `cases` テーブルに**追記**する

既存の 12 ケースは**1 つも消さず、1 つも期待値を変えない。** 全部通り続けること。
そのうえで、少なくとも次を足す:

```go
// issue #18: ** が 2 つ以上あるパターン
{"**/testdata/**", "a/b/testdata/c/d.txt", true},
{"**/node_modules/**", "web/node_modules/x/y.js", true},
{"**/testdata/**", "testdata/a.txt", true},      // 先頭の ** は 0 段にも当たる
{"**/testdata/**", "a/b/testdata", false},       // ディレクトリ自身は「中身」ではない
{"**/testdata/**", "a/testdataX/c.txt", false},  // セグメントの途中一致はしない
{"a/**/b/**/c.txt", "a/x/y/b/z/c.txt", true},    // ** が 2 つ、間に挟まる
{"a/**/b/**/c.txt", "a/x/y/c.txt", false},       // 間の b が無い
{"**", "anything/at/all.txt", true},
{"**/x", "a/b/x", true},
```

**`{"**/testdata/**", "a/b/testdata", false}` と
`{"**/testdata/**", "a/testdataX/c.txt", false}` は必ず入れる。**
「とりあえず全部 true にする」実装がテストを通り抜けるのを防ぐためのケースである。

---

## 2. issue #19（P2）— `shorten()` をルーン単位にする

`mcp__github__issue_read` で #19 を読んでから始めること。

### 確認済みの事実

```go
// internal/server/fold.go:120
func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {          // ← バイト数
		return s
	}
	return s[:n-1] + "…"      // ← バイトで切るので、マルチバイト文字の途中で割れる
}
```

呼び元は `foldFiles` の 1 か所だけ（`fold.go:36`、`shorten(marker, 60)`）で、
結果は `f.FoldReason` に入って JSON で返る。不正な UTF-8 は `encoding/json` が
U+FFFD に置き換えるので、読む人には文字化けとして見える。

### やること — 決めておいた既定

**`n` を「ルーン数」として扱う。** バイト数ではない。

```go
func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
```

- **戻り値のルーン数はちょうど `n`**（`n-1` 個 + 省略記号 1 個）になる。
- **`TrimSpace` は今までどおり残す。**
- **表示幅（east asian width）での切り詰めは実装しない。** issue は
  「Truncating by display width would be better still」と書いているが、
  **それをやると `golang.org/x/text` などの依存が増える。**
  COMMON.md のとおり `go.mod` / `go.sum` は担当外なので触れない。
  **ルーン単位までで止める。** この判断を PR 本文に 1 行書くこと
  （"width-aware truncation would need a new dependency; out of scope here"）。
- `n <= 0` のときは呼ばれないが、`[]rune(s)[:n-1]` が panic しないよう
  **`n < 1` なら `""` を返すガードを 1 行入れておく。** 呼び元は `60` 固定なので
  実害はないが、panic する関数を残さない。

### テスト — ファイル末尾に `TestShortenKeepsRunesWhole` を**新規に足す**

既存の関数には触らない。少なくとも次を見ること:

- issue の再現文字列をそのまま使う:

```go
s := "// 自動生成されたファイルです。編集しないでください。これは非常に長いマーカー行です"
got := shorten(s, 60)
```

  - `utf8.ValidString(got)` が **true** であること（**これが本題**）
  - `utf8.RuneCountInString(got)` が **60** であること
  - `strings.HasSuffix(got, "…")` が true であること
- ASCII の長い文字列（`strings.Repeat("a", 100)`, n=10）→ `"aaaaaaaaa…"`、ルーン数 10
- 短い文字列はそのまま返る（切られない・省略記号が付かない）
- ちょうど `n` ルーンの文字列はそのまま返る（**境界。必ず入れる**）
- 前後の空白が落ちること
- `shorten("あいうえお", 0)` が panic しないこと

---

## 完了条件（**あなた自身が実行して真偽を判定できるもの**。実行結果を PR 本文と報告に貼る）

各ブランチで、そのブランチに入っている変更に対応する行を全部満たすこと。

```bash
cd /home/user/wt/diff-collapse

# 1. 共通（両ブランチ）。gofmt は「何も出ない」のが合格
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')

# 2. 共通: 担当外に出ていないこと
#    → internal/server/fold.go と internal/server/fold_test.go 以外を
#      1 つも出さないのが合格
git diff --name-only origin/main

# 3. 共通: internal/diff に手を入れていないこと（**何も返らない**のが合格）
git diff --name-only origin/main | grep '^internal/diff/'

# 4. #18: 新しいケースが入っていること（1 行以上返るのが合格）
grep -n '\*\*/testdata/\*\*' internal/server/fold_test.go
grep -n '\*\*/node_modules/\*\*' internal/server/fold_test.go

# 5. #18: 該当パッケージのテストが通ること
go test ./internal/server/ -run TestCollapsePatterns -v

# 6. #19: 新しいテスト関数が入っていること（1 行返るのが合格）
grep -n 'func TestShortenKeepsRunesWhole' internal/server/fold_test.go

# 7. #19: ルーン単位になっていること（1 行以上返るのが合格）
grep -n 'utf8.RuneCountInString' internal/server/fold.go
#    #19: バイト長で判定する行が消えたこと（**何も返らない**のが合格）
grep -n 'if len(s) <= n' internal/server/fold.go

# 8. #19: テストが通ること
go test ./internal/server/ -run TestShorten -v
```

**#18 のブランチでは、修正前に落ちることを先に確かめること。**
テストだけ先に足して `go test ./internal/server/ -run TestCollapsePatterns` を走らせ、
**落ちる**のを見てから実装を直す。落ちなかったら、その issue の前提が
事実と違うということなので、**直さずに報告へ書く**（COMMON.md の
「issue がおかしいと思ったとき」）。同じことを #19 でもやること。
「修正前に落ちて修正後に通る」を**実際の出力で**報告に貼る。

## push と PR

- **push と PR 作成まで行う。マージはしない。**
- `git push -u origin gogo/issue-<N>` → `mcp__github__create_pull_request`。
  base は `main`、head は `gogo/issue-<N>`。本文の書き方は COMMON.md の「PR」の節に従う。
- **`web/dist/` はコミットしない**（このレーンでは web を触らないので、そもそも差分に出ない。
  出たら何かがおかしいので報告に書く）。

## 進め方の約束

- **担当外は触らない。** 直したくなる箇所を見つけても直さず、報告に書く。
  そのとき**コードを引用して**書く。issue へのコメントは書かない（メインの担当）。
- **`--collapse` の README / docs の記述は直さない。** issue #52（doc-collapse）の担当である。
  #18 で挙動が変わるので説明を直したくなるが、**やらない。** 報告に 1 行書けばよい。
- **判断に迷って止まらない。** ここに書いていないことが出てきたら、自分で既定を決めて進み、
  **決めた内容と理由を報告に書く。** 確認を上げてよいのは、それが無いと物理的に前へ進めない
  ものだけ。
- 完了報告には `slug` / branch / worktree / commit の 4 つを、**この指示文と同じ綴りで**書く。
  2 本ぶん並べること。
