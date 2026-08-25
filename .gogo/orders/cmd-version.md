slug: cmd-version

# 指示文 O-3 — `version.Revision` を build info から読む

## 前提

- 共通規約は `/home/user/briefs/COMMON.md`。**先に全部読むこと。** 以下はそれを前提に書いてある。
- 名前は上の `slug` からだけ導出する。**自分で別名を付けない。**

```
slug     = cmd-version                 （このファイルの 1 行目。名前の出典はここだけ）
worktree = /home/user/wt/<slug>        →  /home/user/wt/cmd-version
branch   = gogo/issue-<N>              →  gogo/issue-72
```

- **1 issue = 1 PR。** 1 件なので、ブランチ 1 本・PR 1 本。
- 着手はメインが合図してから。G1 の cmd-* 3 レーンが終わるまでこのレーンは開始しない。

## 優先度と期限

- issue #72 … 優先度 P2（bug。壊れてはいないが、`Revision` が常に嘘をつく）。期限: このサイクル内

## 担当 issue とダッシュボードのタスク ID

| issue | タスク ID | 一行 |
|---|---|---|
| #72 | `t-006953` | `version.Revision` が誰にも設定されず、`--status` が常に `HEAD` と言う |

節目ごとに `gogodash task set --id t-006953 --status running --progress <n>` を打つ。
終わったら `--status done --progress 100 --result "<1 行>"`。

## 触ってよいファイル

**`version/` の中だけ。ここから 1 バイトも出ない。**

- `version/revision.go` （**新規作成**）
- `version/version_test.go` （**新規作成**）
- `version/version.go` （**触るのは `Revision` の宣言行とそのコメントだけ**。理由は下）

**触ってはいけないもの:**

- `cmd/` 配下すべて。**特に `cmd/server.go` と `cmd/root.go`。**
  cmd-hook / cmd-flags / cmd-comment / cmd-reviews / cmd-wait が使用中である。
- `internal/` 配下すべて
- `Taskfile.yml` / `.github/` / `go.mod` / `go.sum` / `.tagpr` / `README.md`
  （COMMON.md のとおり、明示的に担当していないので触らない）

## 確認済みの事実（この指示文を書く前に実際のコードで確かめてある）

**配線はすでに全部つながっている。直すのは `version/` だけでよい。**

```go
// version/version.go — いまの全部。8 行しかない
var Version = "dev"
var Revision = "HEAD"        // ← ここを書き換える人が誰もいない

// cmd/server.go:43     Revision:    version.Revision,
// internal/server/server.go:288    Revision: s.opts.Revision,
// internal/server/server.go:274    Revision string `json:"revision,omitempty"`
```

つまり `version.Revision` に正しい値が入りさえすれば、`/_/api/status` まで
そのまま届く。**`cmd/` も `internal/` も一切変更しなくてよい。**

## やること

1. **新しいファイル `version/revision.go` を作り、そこに書く。**
   `version/version.go` に書き足さない。理由: `.tagpr` の `versionFile` 機構が
   リリース時に `version/version.go` を機械的に書き換える。同じファイルに
   ロジックを足すと、その書き換えと衝突する余地を作ってしまう。
   **`version/version.go` は `Revision` の宣言行のコメントを直す以外は触らない。**

2. build info から `vcs.revision` を読む。issue が挙げているとおり:

```go
if bi, ok := debug.ReadBuildInfo(); ok {
    for _, s := range bi.Settings {
        if s.Key == "vcs.revision" { Revision = s.Value }
    }
}
```

3. **ただし、この形のままでは試験できない。** `debug.ReadBuildInfo()` は
   テストバイナリでは `vcs.revision` を返さないことが多く、テストから制御できない。
   **純粋な関数に切り出し、`init()` はそれを呼ぶだけにする。**

```go
// revisionFrom picks the commit out of the build info, and returns fallback
// when the build carries no VCS stamp.
func revisionFrom(bi *debug.BuildInfo, ok bool, fallback string) string

func init() {
	bi, ok := debug.ReadBuildInfo()
	Revision = revisionFrom(bi, ok, Revision)
}
```

   パッケージ変数は `init()` より先に初期化されるので、`version.go` の
   `var Revision = "HEAD"` がそのまま fallback として渡る。

4. **見つからなかったときは既存の値を保つ。** `ok` が false、`bi` が nil、
   `Settings` に `vcs.revision` が無い、値が空文字 —— どの場合も
   `Revision` を上書きしない（`"HEAD"` のままにする）。
   **空文字で上書きすると `json:"revision,omitempty"` によってフィールドごと
   消えるので、いまより情報が減る。**

## 決めておいた既定（迷わないこと。issue に書かれていないので、こちらで決めた）

- **`vcs.modified` は見ない。`-dirty` の接尾辞を付けない。** issue が求めているのは
  `vcs.revision` だけである。範囲を広げない。
- **`-ldflags "-X ..."` を足さない。** `Taskfile.yml` も `.github/` も触らない。
  issue の Expected が「with no ldflags to keep in sync」と明記している。
- **ただし ldflags で入った値を壊さないこと。** 別レーン（release、issue #101）が
  `.goreleaser.yml` に `-X github.com/tenntenn/sbnn/version.Revision={{.FullCommit}}` を
  入れる予定である。ldflags はリンク時にパッケージ変数へ値を入れるので、
  その後に走る `init()` が**無条件に上書きすると、リリースビルドの正しい commit を
  build info の値で潰す**ことになる。上の 4. の規則（見つからなければ既存の値を保つ）を
  守っていれば、
  - build info に `vcs.revision` があれば同じ commit で上書きされる（実害なし）
  - 無ければ ldflags の値がそのまま残る
  のどちらかになり、どちらでも正しい。**「見つからなければ既存の値を保つ」を必ず守ること。**
  この相互作用を PR 本文に 1 行書く。
- **`Version` には一切触らない。** `.tagpr` が管理している。
- **短縮（7 桁に切る等）はしない。** 生の値をそのまま入れる。切る判断は表示側の仕事で、
  ここではない。

## テスト（`version/version_test.go` を新規作成）

`revisionFrom` をテーブル駆動で回す。`debug.BuildInfo` は自分で組み立てられる。

| 入力 | 期待 |
|---|---|
| `Settings` に `{Key:"vcs.revision", Value:"abc123"}` があり `ok=true` | `"abc123"` |
| `Settings` に `vcs.revision` が無く `ok=true` | fallback（`"HEAD"`） |
| `Settings` に `{Key:"vcs.revision", Value:""}` があり `ok=true` | fallback（**必ず入れる**） |
| `ok=false` | fallback |
| `bi=nil, ok=true` | fallback（**panic しないこと。必ず入れる**） |
| `Settings` に `vcs.time` や `vcs.modified` が混ざっている | `vcs.revision` の値だけを拾う |

`init()` が走った後の `Revision` そのものを断定するテストは**書かない**
（`go test` の実行環境によって値が変わるため、落ちるテストになる）。
代わりに「`Revision` が空文字ではないこと」だけを 1 つ確かめてよい。

## 完了条件（**あなた自身が実行して真偽を判定できるもの**。実行結果を PR 本文と報告に貼る）

```bash
cd /home/user/wt/cmd-version

# 1. ビルド・vet・テスト・整形。gofmt は「何も出ない」のが合格
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')

# 2. 1 行以上返ること
grep -n 'func revisionFrom(' version/revision.go
grep -n 'vcs.revision' version/revision.go
grep -n 'debug.ReadBuildInfo' version/revision.go

# 3. 担当外に出ていないこと
#    → version/ の下のファイルしか出さないのが合格
git diff --name-only origin/main

# 4. cmd/ と internal/ に手を入れていないこと（**何も返らない**のが合格）
git diff --name-only origin/main | grep -E '^(cmd|internal)/'

# 5. Taskfile.yml / .github/ に ldflags を足していないこと（**何も返らない**のが合格）
git diff --name-only origin/main | grep -E '^(Taskfile.yml|\.github/|\.tagpr)'
```

さらに**実際に動かして確かめる**（自己申告ではなく出力を報告に貼る）。
`go run` と `go build` は VCS スタンプの付き方が違うので、**両方**見ること:

```bash
# go build で建てたバイナリは vcs.revision を持つ
go build -o /tmp/sbnn-rev . && go version -m /tmp/sbnn-rev | grep vcs.revision

# 実際に status へ出ること
/tmp/sbnn-rev --foreground &        # 終わったら kill する
curl -s localhost:6280/_/api/status | grep -o '"revision":"[^"]*"'
```

`"revision":"HEAD"` のままなら**直っていない。** そのときは報告に
「何をどう試して、どこで HEAD のままだったか」を書くこと。**通ったことにしない。**

（補足: リポジトリに未コミットの変更があると Go が VCS スタンプを省くことがある。
その場合は一度コミットしてから `go build` し直して確認すること。
`go build` でどうしてもスタンプが付かない環境なら、**その事実と、代わりに何で
確かめたかを報告に書く。** 確かめずに「直った」と書かない。）

## push と PR

- **push と PR 作成まで行う。マージはしない。**
- `git push -u origin gogo/issue-72` → `mcp__github__create_pull_request`。
  base は `main`、head は `gogo/issue-72`。本文の書き方は COMMON.md の「PR」の節に従う。
- `## Verification` には、上の `go version -m` と `/_/api/status` の**実際の出力**を貼る。
- **`web/dist/` はコミットしない。**

## 進め方の約束

- **担当外は触らない。** 直したくなる箇所を見つけても直さず、報告に書く。
  そのとき**コードを引用して**書く。issue へのコメントは書かない（メインの担当）。
- **判断に迷って止まらない。** ここに書いていないことが出てきたら、自分で既定を決めて進み、
  **決めた内容と理由を報告に書く。** 確認を上げてよいのは、それが無いと物理的に前へ進めない
  ものだけ。
- issue の前提が事実と違う / 再現しない / 仕様の決めが要る、のどれかなら
  **無理に直さず**、COMMON.md の「issue がおかしいと思ったとき」に従って報告へ回す。
- 完了報告には `slug` / branch / worktree / commit の 4 つを、**この指示文と同じ綴りで**書く。
