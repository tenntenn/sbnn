slug: cmd-tests

# cmd-tests — CLI のヘルパ関数にテーブルテストを足す（issue #145）

優先度: P1
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `cmd-tests`）
  - `branch   = gogo/issue-145`
- **1 issue = 1 PR。** `origin/main` から切る。
- **push と PR 作成まで行う。マージはしない。**

## 担当ファイル（これ以外は 1 バイトも触らない）

- `cmd/helpers_table_test.go`（新規）**これ 1 ファイルだけ**

触ってはいけないもの:

- **`cmd/` 配下の実装ファイル（`*.go` で `_test.go` でないもの）は 1 行も変えない。**
  `cmd/comment.go` `cmd/root.go` `cmd/util.go` `cmd/reviews.go` `cmd/hook.go` `cmd/wait.go`
  `cmd/submit.go` などは全部、別レーンが同じ波で実装を直している。
- **`cmd/reviews_test.go`（既存）も触らない。** cmd-reviews レーンの担当。
- `internal/` `web/` `go.mod` `go.sum` `Taskfile.yml` `.github/` も触らない。
- `web/dist/` は絶対にコミットしない。

**テストの中で実装を「直したくなる」場面が必ず来る。直さない。** 見つけた不具合は
テストにせず、報告に「関数名・現在の挙動・期待される挙動・根拠のコード引用」を書く。

## issue #145 — cmd がほぼ無テスト（タスク ID: t-c490f7）

やること: `cmd` パッケージの純粋なヘルパ関数に、テーブル駆動のテストを足す。

### どの関数を選ぶか（ここが一番大事）

issue が列挙している 0% の関数のうち、**いま別の issue で挙動そのものが直されている最中のものは
選んではいけない。** 選ぶと、いまの（壊れた）挙動をテストに焼き付けることになり、
修正 PR とぶつかって両方が落ちる。

**選んではいけない関数（確定。理由の issue 番号つき）:**

| 関数 | 直っている最中の issue |
|---|---|
| `parseLines` | #141（オーバーフローが MaxInt64 になる） |
| `parseLineSpec` | #141 と同じ経路 |
| `parseLabels` | #142（trim されない・重複キーが黙って消える） |
| `historyFile` | #143（`false` / `0` / `-` がファイル名になる） |
| `flexLines.UnmarshalJSON` | #144（`12.0` が「数値でない」と弾かれる） |
| `normalizeSide` | #53（`--side OLD` が弾かれる） |
| `splitPatterns` | #52（パターン中のカンマで分割される） |
| `printCommentStream` / `lineRef` / `firstLine` | 既に `cmd/reviews_test.go` にテストがある |

**候補（ここから選ぶ）:** `groupName`（util.go）、`chosenVerdict`（submit.go）、
`shouldOpen`（root.go）、`lineRangeOf`（comment.go）、`suggestionText`（comment.go）、
`singleComment`（comment.go）、`readBulkComments`（comment.go）、`readStdin`（root.go）、
`describeHook`（hook.go）、`shortDuration`（reviews.go）、`indent`（reviews.go）、
`waited`（reviews.go）、`labelPairs`（reviews.go）、`suggestionCount`（reviews.go）、
`summarize`（root.go）、`jsonEncoder` / `lineEncoder`（util.go）。

**候補から 1 つ選ぶたびに、必ずこの機械的な確認をしてから書く:**

```
mcp__github__search_issues で
  query: "repo:tenntenn/sbnn is:issue is:open <関数名> in:body"
```

1 件でもヒットしたら**その関数は選ばない**（別レーンが直している最中の可能性が高い）。
ヒットした関数名と issue 番号を、選ばなかった理由として報告に書く。

**目標は 6 関数以上。** 6 に届かない場合は、届いた数で出す。**そこで止まらない。**

### テストの書き方

- ファイルは `cmd/helpers_table_test.go` の 1 本にまとめる。`package cmd`。
- `internal/` の既存のテーブル駆動テスト（`internal/diff/parse_test.go`、
  `internal/model/model_test.go`）の書き方に合わせる。名前付きケースの `[]struct` と
  `t.Run(tt.name, ...)`。
- **トップレベルの識別子はすべて `helpers` で始める**（`helpersTempHome` など）。
  `cmd/reviews_test.go` や、別レーンが後で足すテストファイルと名前がぶつからないようにするため。
  テスト関数名も `TestHelpers<関数名>` の形にする。
- グローバルなフラグ変数に依存する関数（`shouldOpen`、`groupName`、`chosenVerdict` など）は、
  テストの中で値を保存 → 設定 → `t.Cleanup` で復元する。**復元を忘れると他のテストが壊れる。**
- 環境変数やカレントディレクトリに依存するものは `t.Setenv` と `t.TempDir` を使う。
  `os.Chdir` は使わない（並列テストを壊す）。
- 標準入力を読む関数（`readStdin`）は、パッケージ変数を差し替えられないなら**選ばない。**
  実装を変えてまでテストしやすくしない。

### いまの挙動が間違っていると思ったとき

**間違った挙動をテストに焼き付けない。実装も直さない。** その関数を候補から外し、
報告の「見送り / 疑義」に、関数名・現在の挙動・そう判断した根拠（コードの引用）を書く。

## 完了条件（自分で実行して真偽を決められること）

```bash
test -f cmd/helpers_table_test.go
go build ./... && go vet ./...
go test ./... 
gofmt -l $(git diff --name-only origin/main -- '*.go')      # 何も返らないのが合格

# 実装ファイルを 1 行も触っていないことの証明（何も返らないのが合格）
git diff --name-only origin/main | grep -v '^cmd/helpers_table_test.go$'

# テストした関数の数（6 以上が目標）
grep -c '^func TestHelpers' cmd/helpers_table_test.go

# カバレッジが上がったこと。before と after の両方の数字を報告に貼る
git stash -u && go test ./cmd/ -cover ; git stash pop && go test ./cmd/ -cover
```

`go test ./cmd/ -cover` の **before（origin/main）と after（このブランチ）の数字を両方**
報告に貼る。after が before より大きいことが合格。issue の記録では before は 14.1% である。
違う数字が出たらそのまま書く。

さらに、**書いたテストが実際に何かを検査していること**を 1 つ以上示す。
選んだ関数のうち 1 つについて、返り値を意図的に変えたらテストが落ちることを
（実装を一時的に書き換えて）確認し、出力を報告に貼り、**`git checkout -- cmd/` で必ず戻す。**
戻したことを `git status --porcelain` で確認する。

## コミット / PR

フッタは `Refs #145` にする（`Fixes #145` にしない）。
理由: issue の Expected が真っ先に挙げている `parseLines` / `parseLabels` / `normalizeSide` /
`splitPatterns` / `historyFile` / `flexLines` は、いずれも別の issue で挙動そのものが
修正中のため、意図的に対象から外している。issue は open のまま残す。
この理由を PR 本文に 1 段落で書く。

`internal/client` / `internal/mo` / `internal/paths` にテストが 1 つも無いことも
issue は指摘しているが、**この PR では触らない**（担当ファイル外）。報告に 1 行で書く。

## 全体を通しての決まり

- 担当外のファイルは触らない。見つけた問題は自分で直さず報告に書く。
- 判断に迷って止まらない。既定を自分で決めて進み、決めた内容と理由を報告に書く。
- **issue へのコメントは書かない。** 判断と根拠を報告に書くだけ。メインが書く。
- 報告には `slug` / branch / worktree / commit の 4 つを、この指示文と同じ綴りで書く。
