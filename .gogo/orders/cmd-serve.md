slug: cmd-serve

# 指示文 SRV-05 — 塞がったポートの 10 秒と、理由を言わないエラー（1 issue）

- 優先度: **P1**（#90）
- 期限: 2026-08-26 中
- グループ: G7

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` — 手順・検証・コミット・PR の書式はすべてここに従う。
  この指示文はそれを上書きしない。食い違ったら COMMON.md が優先。
- `/home/user/briefs/TASKIDS.tsv` — issue とタスク ID の対応。

## 名前

```
worktree = /home/user/wt/cmd-serve    # slug から機械的に導出する。他の場所に worktree 名は書かない
branch   = gogo/issue-90              # COMMON.md のとおり、origin/main から切る
```

worktree が無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/cmd-serve origin/main
```

## 着手条件

**すぐ着手してよい。誰の完了も待たない。** 担当ファイルは他のどのレーンとも重なっていない。

## 触ってよいファイル（**この 2 本だけ**）

```
cmd/server.go
cmd/server_test.go    （新規に作ってよい。いま存在しない）
```

**この 2 本から出ない。** 次は他のレーンが使用中なので 1 行も触らない:

| ファイル | 使用中のレーン |
|---|---|
| `cmd/root.go` | cmd-* レーンが使用中。`ensureServer` はここにある。**触らない** |
| `cmd/hook.go` | G8 cmd-hook |
| `cmd/comment.go` `comments.go` `wait.go` `submit.go` | G1 cmd-comment / cmd-wait |
| `cmd/reviews.go` | G1 cmd-reviews |
| `internal/server/` 配下すべて | srv-* レーン（SRV-01〜SRV-04）と G1 store |
| `web/` 配下すべて | G2〜G5 の web レーン |

## やること: #90（task `t-f9b1a3`）

いま何が起きるか。ポートを他の何かが握っていると、`git diff | sbnn` は
**10 秒だまって座り**、それから原因を名指ししないエラーを出す:

```
sbnn: the sbnn server on localhost:6401 did not become ready (pid 18983, see …/state/sbnn/server-6401.log)
```

本当の理由（`bind: address already in use`）は、利用者が自分で開きに行かないと
読めないログファイルの中にしかない。**利用者が知る必要のあるただ 1 つのことが、
唯一表示されないものになっている。**

この issue は 3 つの問題を挙げている。**担当ファイルの中で全部直せるのは 1 と 2 で、
3 は範囲外**である。

| 求められていること | どこにある | この PR で |
|---|---|---|
| 1. 理由を表示する（`spawnServer` が子のログを読んで即座に出す） | `cmd/server.go` | **やる** |
| 2. `serving at …` をリッスン成功のあとへ動かす | `cmd/server.go` | **やる** |
| 3. 空いているポートへのフォールバック | `cmd/root.go` の `ensureServer` とセッション管理 | **やらない** |

### 1. 理由を表示する

`spawnServer`（`cmd/server.go:57` 付近）は子プロセスを detach してポーリングするだけで、
子の `bind: address already in use` は端末に届かない。

- `waitForReady` のポーリングループの中で、**子が書いたログファイルを毎回覗く**。
  `cannot listen on` を含む行が現れたら、**タイムアウトを待たずに即座に戻り**、
  その行を利用者へ提示するエラーにする。
  例: `sbnn: cannot listen on localhost:6401: listen tcp 127.0.0.1:6401: bind: address already in use`
- 子プロセスがすでに終了している場合（`os.Process` が消えている、
  あるいは `cmd.Wait` 相当が取れる場合）も、**10 秒待たずに戻る**。
- ログファイルのパスは既存のエラーメッセージがすでに知っている。
  **新しくパスを組み立てる仕組みを発明しない。** いまその文字列を作っている場所を使う。
- タイムアウトまで何も見つからなかった場合のメッセージは、
  **従来の文言（ログの場所を案内する形）をそのまま残す。** 悪化させない。

### 2. `serving at …` を後ろへ動かす

`runServer`（`cmd/server.go:22`、印字は `cmd/server.go:52` 付近）は
`srv.Run` がリッスンを試みる**前に** `sbnn: serving at …` を出している。
そのせいで、これから失敗するサーバのログの 1 行目が成功を主張している。
**ログ自身が最初に嘘をつくので、読んだ人が原因を取り違える。**

- `srv.Run` がリッスンに成功したあとに印字する形へ変える。
- `internal/server` 側にコールバックや新しいメソッドが必要になる場合、
  **`internal/server/server.go` は他レーンが使用中なので触れない。**
  その場合は、`cmd/server.go` の中でリッスンの成否が分かる形
  （たとえば `srv.Run` を起動したあとに `srv.BaseURL()` へ短い間隔で
  1 回だけ疎通確認してから印字する）に落とす。
  **`internal/server` を触らないと実現できない案は採らない。**
  どちらを採ったかと理由を PR 本文と報告に書く。

### 3. フォールバック（やらない）

自動で次の空きポートを選ぶ案は `cmd/root.go` の `ensureServer` とセッション管理に
またがるので、この PR では**やらない**。
PR 本文の `## What changed` の末尾に
`Automatic fallback to a free port needs cmd/root.go (owned by another lane) and is left out.`
と英語で 1 行書き、最終報告の「見送り」にも書く。

**PR 本文には `Fixes #90` を書かない。`Refs #90` を書く。**

## テスト（必須）

`cmd/server_test.go` を新規に作る。`cmd/reviews_test.go` の書き方に合わせる。

- 塞がったポートの再現: テストの中で `net.Listen("tcp", "127.0.0.1:0")` して
  取れたポートを塞ぎ、そのポートで起動を試みる。
  **10 秒かからずに**エラーが返り、そのエラー文字列に
  `address already in use` が**含まれる**ことを確かめる
  （`t.Deadline` ではなく、経過時間を測って 5 秒未満であることを assert する）。
- ログ行の解析部分は、**ログファイルの中身を組み立てて関数へ渡せる形**に切り出し、
  テーブル駆動でテストする（`cannot listen on` を含む行／含まない行／空ファイル）。
- `serving at` の印字順は、**印字を担う関数の呼び出し順**が検証できる形にして確かめる。
  実プロセスを起動しないと検証できない形にはしない。

## 完了条件（**実行すれば真偽が決まるもの。自己申告しない**）

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')
```

`gofmt -l` が**何も出力しない**のが合格。加えて:

```bash
# 担当外のファイルを触っていないこと（何も出力しなければ合格）
git diff --name-only origin/main | grep -v -e '^cmd/server\.go$' -e '^cmd/server_test\.go$'

# 1 行以上返れば合格
grep -n 'cannot listen on' cmd/server.go

# 新規テストが実際に走って通っていること（1 行以上返れば合格）
go test ./cmd/ -run 'TestSpawnServer|TestWaitForReady' -v | grep '^--- PASS'

# ポートが塞がっているときに 10 秒かからないこと（テスト全体が 30 秒以内で終われば合格）
go test ./cmd/ -run 'TestSpawnServer|TestWaitForReady' -timeout 30s
```

PR は 1 本。

## やること・やらないこと

- **push と PR 作成まで行う。マージはしない。**
- 担当外は触らない。特に **`cmd/root.go` と `internal/server/` は 1 バイトも触らない**。
- 見つけた問題は自分で直さず、最終報告に書く。
- 判断に迷って止まらない。既定を決めて進み、決めた内容と理由を報告に書く。
- **`cmd/root.go` が空くのを待たない。** 待つくらいなら `Refs` で出す。

## 完了報告

COMMON.md の報告書式に加えて、先頭に次の 4 行をこの綴りのまま書く:

```
slug: cmd-serve
branch: gogo/issue-90
worktree: /home/user/wt/cmd-serve
commit: <この PR の commit>
```
