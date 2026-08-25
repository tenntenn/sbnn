slug: cmd-hook

# 指示文 O-1 — `sbnn hook` の取りこぼしと黙殺を直す

## 前提

- 共通規約は `/home/user/briefs/COMMON.md`。**先に全部読むこと。** 以下はそれを前提に書いてある。
- 名前は上の `slug` からだけ導出する。**自分で別名を付けない。**

```
slug     = cmd-hook                    （このファイルの 1 行目。名前の出典はここだけ）
worktree = /home/user/wt/<slug>        →  /home/user/wt/cmd-hook
branch   = gogo/issue-<N>              →  issue ごとに 1 本。前の issue に積み上げない
```

- **1 issue = 1 PR。** 2 件あるので、ブランチ 2 本・PR 2 本になる。
- 着手はメインが合図してから。G1 の cmd-* 3 レーン（cmd-comment / cmd-reviews / cmd-wait）が
  終わるまでこのレーンは開始しない。合図が来たら上から順にやる。

## 優先度と期限

- issue #47 … 優先度 P2（enhancement）。期限: このサイクル内
- issue #48 … 優先度 P1（bug。ユーザが「登録した」と誤認する）。期限: このサイクル内
- **#47 を先にやる。** 順序の理由は「担当ファイル」の節に書いた。

## 担当 issue とダッシュボードのタスク ID

| issue | タスク ID | 一行 |
|---|---|---|
| #47 | `t-b44856` | `sbnn hook` から 1 個だけ hook を消せない |
| #48 | `t-1267ee` | `sbnn hook --clear --on-review '...'` が登録を黙って捨てる |

節目ごとに `gogodash task set --id <上の ID> --status running --progress <n>` を打つ。
終わったら `--status done --progress 100 --result "<1 行>"`。

## 触ってよいファイル

**この 3 つだけ。ここから 1 バイトも出ない。**

- `cmd/hook.go`
- `cmd/hook_test.go` （**新規作成**。いま存在しない）
- `internal/client/client.go` （**#47 でメソッドを 1 つ「足す」だけ**。既存メソッドの
  シグネチャを変えない。理由は下に書いた）

**触ってはいけないもの（他レーンが使用中、または別 issue の担当）:**

- `cmd/root.go` … **cmd-flags レーンが専有している。** #48 の本文の最後に
  「The same shape exists on the root command」とあるが、**そこは直さない。**
  root コマンド側の同じ形は issue #49 で cmd-flags が直す。
  #48 の PR 本文に「root command side is issue #49, handled separately」と 1 行書いて済ませる。
- `cmd/comment.go`（cmd-comment） / `cmd/reviews.go` `cmd/reviews_test.go`（cmd-reviews） /
  `cmd/wait.go` `cmd/comments.go` `cmd/submit.go` `cmd/util.go`（cmd-wait）
- `internal/server/` 配下すべて。**サーバ側は直す必要がない**（下で確認済み）。
- `README.md` / `docs/` / `skills/`

---

## issue #47 — `sbnn hook --remove <id>` を足す

`mcp__github__issue_read` で #47 を読んでから始めること（owner=tenntenn, repo=sbnn）。

### 確認済みの事実（この指示文を書く前に実際のコードで確かめてある）

サーバ側は**すでに完成している。追加実装は要らない。**

```go
// internal/server/server.go:170-171
mux.HandleFunc("DELETE /_/api/groups/{group}/hooks", s.handleDeleteHooks)
mux.HandleFunc("DELETE /_/api/groups/{group}/hooks/{id}", s.handleDeleteHooks)

// internal/server/server.go:786
removed := s.store.DeleteHooks(name, r.PathValue("id"))

// internal/server/store.go:367 — id == "" なら全部、そうでなければ ID 一致のものだけ
if id == "" || h.ID == id {
```

足りないのは**クライアント側と CLI だけ**である。

### やること

1. `internal/client/client.go` に**新しいメソッドを 1 つ足す。**

```go
// DeleteHook removes one hook by ID.
func (c *Client) DeleteHook(ctx context.Context, group, id string) (int, error)
```

   既存の `DeleteHooks(ctx, group)`（client.go:252）は**そのまま残す。シグネチャを変えない。**
   `--clear` が使い続けるし、他レーンが同じファイルを読んでいる可能性がある。
   **メソッドを足すのは衝突しないが、シグネチャを変えると全レーンに波及する。**
   URL は `c.url("/_/api/groups/%s/hooks/%s", url.PathEscape(group), url.PathEscape(id))`。
   戻り値は既存の `DeleteHooks` と同じく `{"removed": n}` を読む。

2. `cmd/hook.go` に `--remove` を足す。

   - パッケージ変数 `hookRemove string` を既存の var ブロックに足す
   - `init()` の中で `f.StringVar(&hookRemove, "remove", "", "Remove one hook by ID (sbnn hook lists the IDs)")`
   - `runHook` の `switch` の**先頭**に `case hookRemove != "":` を足す

3. **存在しない ID を渡されたら成功にしない。** `removed == 0` はサーバでは
   エラーにならず `0` が返るだけである（上の store.go の実装を参照）。CLI 側で
   `removed == 0` を見てエラーを返すこと。文言の例:

```
no hook %q on group %q
```

   `removed > 0` のときは既存の `--clear` と揃えた文体で stderr に出す。例:

```
sbnn: removed hook %q from group %q
```

4. `--remove` と `--clear` が同時に来たら**エラーにする。**
   **`init()` の末尾に `MarkFlagsMutuallyExclusive` を足さないこと。**
   そこは #48 が触る。#47 では `runHook` の中で明示的に見て `fmt.Errorf` を返す。
   （2 本の PR がどちらも `init()` の末尾を書き換えると、マージで衝突する。
   #47 は `runHook` の中、#48 は `init()` の末尾、と場所を分けてある。）

5. `hookCmd` の `Long` のコマンド例に 1 行足す。

```
  $ sbnn hook --remove h2           # forget one of them
```

### 決めておいた既定（迷わないこと）

- フラグ名は **`--remove`**。`--clear h2` のように `--clear` へ引数を持たせる形は採らない。
  `--clear` は現に bool であり、bool を string へ変えると既存の `--clear` 単独呼び出しの
  意味が変わるため。
- 短縮形（`-r` 等）は**付けない。**
- `--remove` は 1 個だけ受ける。繰り返し指定（`StringArrayVar`）にはしない。

---

## issue #48 — `--clear` と登録フラグの同時指定を黙って捨てない

`mcp__github__issue_read` で #48 を読んでから始めること。

### 確認済みの事実

`runHook`（cmd/hook.go）は `switch` で `case hookClear:` が先頭にあり、その場で
`return nil` するため、`--clear --on-review '...'` は hook を消したうえで
**新しい登録を捨て、`removed 1 hook(s)` としか言わない。**

### やること

issue は「相互排他にする」か「clear してから register する（replace の意味にする）」の
2 案を挙げている。**相互排他を採る。** 決めた理由:

- replace の意味にすると、`--clear` に「消す」と「消してから入れる」の 2 つの意味が生まれ、
  スクリプトから見て `--clear` の効果が他のフラグ次第で変わる。
- 相互排他はユーザに 1 回エラーを見せるだけで、取り違えようがない。
- **既存の登録済み hook を消す挙動を、新しい条件下で黙って発火させない**ほうが安全側。

実装は `init()` の**末尾**に 1 行:

```go
hookCmd.MarkFlagsMutuallyExclusive("clear", "on-review", "on-review-url")
```

- **`--remove` をこの行に含めないこと。** `--remove` は #47 の PR で入る別フラグで、
  この PR のブランチには存在しない。含めると cobra が起動時に panic する。
  `--remove` と `--clear` の排他は #47 の PR 側で `runHook` の中に入れてある。
- `hookCmd` の `Long` は変えなくてよい。
- **`cmd/root.go` は触らない。** #48 本文の最後の 1 文（root コマンドにも同じ形がある）は
  issue #49 の担当で、cmd-flags レーンが直す。PR 本文にその旨を 1 行書く。

---

## テスト（両方の issue で必須）

`cmd/hook_test.go` を**新規に作る。** 既存の `cmd/reviews_test.go` の書き方（テーブル駆動）に
合わせること。`cmd/reviews_test.go` は**読むだけ。編集しない。**

- **#47**: `httptest.NewServer` でサーバを立て、`client.New(...)` を向けて
  `DeleteHook` を叩く。少なくとも次の 3 つ:
  1. リクエストパスが `/_/api/groups/<group>/hooks/<id>` になっていること
     （ハンドラ側で `r.URL.Path` を記録して照合する）
  2. `{"removed":1}` を返したとき `1, nil` が返ること
  3. `{"removed":0}` を返したとき `0, nil` が返ること（**エラーにするのは CLI 側の責務**）
  グループ名や ID に `/` や空白が入っても壊れないこと（`url.PathEscape` の確認）を
  1 ケース足す。
- **#48**: `hookCmd.Flags()` に対して cobra の排他が効いていることを確かめる。
  `hookCmd` はパッケージ変数なのでテストから触れる。
  `--clear` と `--on-review` を同時にセットして `hookCmd.ValidateFlagGroups()` が
  エラーを返すこと、片方だけならエラーを返さないことの 2 ケース。
  グローバル変数を触るので、テストの最後に `hookCmd.Flags().Visit` などで
  立てたフラグを戻すか、`t.Cleanup` で元に戻すこと。

---

## 完了条件（**あなた自身が実行して真偽を判定できるもの**。実行結果を PR 本文と報告に貼る）

各ブランチで、そのブランチに入っている変更に対応する行を全部満たすこと。

```bash
cd /home/user/wt/cmd-hook

# 1. 共通（両ブランチ）: ビルド・vet・テスト・整形。gofmt は「何も出ない」のが合格
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')

# 2. issue #47 のブランチでだけ 1 行以上返ること
grep -n 'func (c \*Client) DeleteHook(' internal/client/client.go
grep -n '"remove"' cmd/hook.go
grep -n 'hookRemove' cmd/hook.go

# 3. issue #47: 既存メソッドのシグネチャを変えていないこと（1 行返るのが合格）
grep -n 'func (c \*Client) DeleteHooks(ctx context.Context, group string) (int, error)' internal/client/client.go

# 4. issue #48 のブランチでだけ 1 行返ること
grep -n 'MarkFlagsMutuallyExclusive("clear", "on-review", "on-review-url")' cmd/hook.go

# 5. issue #48: --remove を排他リストに含めていないこと（**何も返らない**のが合格）
grep -n 'MarkFlagsMutuallyExclusive.*"remove"' cmd/hook.go

# 6. 両ブランチ: 担当外に出ていないこと
#    → 下のコマンドが cmd/hook.go / cmd/hook_test.go / internal/client/client.go
#      以外を 1 つも出さないのが合格
git diff --name-only origin/main
```

さらに**実際に叩いて確かめる**（自己申告ではなく出力を報告に貼る）:

```bash
go run . --foreground &      # サーバを前面で起動（終わったら kill する）
go run . hook --on-review 'echo one'
go run . hook --on-review-url http://localhost:9000/x
go run . hook                          # ID が 2 つ出ること
go run . hook --remove <上で出た 2 つ目の ID>
go run . hook                          # 1 つだけ残っていること
go run . hook --remove nosuchid        # エラーで終わり、終了コードが 0 でないこと
go run . hook --clear --on-review 'x'  # #48 のブランチではエラーになること
```

## push と PR

- **push と PR 作成まで行う。マージはしない。**
- `git push -u origin gogo/issue-<N>` → `mcp__github__create_pull_request`。
  base は `main`、head は `gogo/issue-<N>`。本文の書き方は COMMON.md の「PR」の節に従う。
- **`web/dist/` はコミットしない**（このレーンでは web を触らないので、そもそも差分に出ないはず。
  出たら何かがおかしいので報告に書く）。

## 進め方の約束

- **担当外は触らない。** 直したくなる箇所を見つけても直さず、報告に書く。
  そのとき**コードを引用して**書く。issue へのコメントは書かない（メインの担当）。
- **判断に迷って止まらない。** ここに書いていないことが出てきたら、自分で既定を決めて進み、
  **決めた内容と理由を報告に書く。** 確認を上げてよいのは、それが無いと物理的に前へ進めない
  ものだけ。
- issue の前提が事実と違う / 再現しない / 仕様の決めが要る、のどれかなら
  **無理に直さず**、COMMON.md の「issue がおかしいと思ったとき」に従って報告へ回す。
- 完了報告には `slug` / branch / worktree / commit の 4 つを、**この指示文と同じ綴りで**書く。
