slug: srv-hook

# 指示文 SRV-02 — レビューフックに判定を渡し、配信を固くする（2 issue）

- 優先度: **P1**（#25, #146）
- 期限: 2026-08-26 中
- グループ: G6

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` — 手順・検証・コミット・PR の書式はすべてここに従う。
  この指示文はそれを上書きしない。食い違ったら COMMON.md が優先。
- `/home/user/briefs/TASKIDS.tsv` — issue とタスク ID の対応。

## 名前

```
worktree = /home/user/wt/srv-hook     # slug から機械的に導出する。他の場所に worktree 名は書かない
branch   = gogo/issue-<N>             # COMMON.md のとおり、issue ごとに origin/main から切り直す
```

worktree が無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/srv-hook origin/main
```

## 着手条件

**すぐ着手してよい。誰の完了も待たない。** 担当ファイルは他のどのレーンとも重なっていない。

## 触ってよいファイル（**この 2 本だけ**）

```
internal/server/hook.go
internal/server/hook_test.go   （新規に作ってよい。いま存在しない）
```

**この 2 本から出ない。** 次は他のレーンが使用中なので 1 行も触らない:

| ファイル | 使用中のレーン |
|---|---|
| `internal/server/server.go` `store.go` | G1 store → srv-api（SRV-01）→ srv-core（SRV-04） |
| `internal/server/prompt.go` | export-pkg |
| `internal/server/proxy.go` | mo-proxy |
| `internal/server/preview.go` `spa.go` | srv-preview（SRV-03） |
| `cmd/hook.go` | G8 cmd-hook |
| `cmd/server.go` | cmd-serve（SRV-05） |
| `internal/model/model.go` | model レーン（G1） |

`internal/model` は**読むだけ**なら使ってよい（`model.Verdict` と
`Verdict.Blocking()` はすでに存在する。追加は不要）。

## 1 件目: #25 — フックが判定を知らされない（task `t-b196a1`）

`internal/server/hook.go` の `ReviewEvent` は `Group` `URL` `ReviewedAt` `Note`
`Comments` `Prompt` を持つが、**判定（verdict）を持たない**。判定はコメントを数えなくても
下流が分かるようにするために入れたものなのに、その下流の代表であるフックだけが受け取れていない。

やること:

1. `ReviewEvent` に `Verdict model.Verdict` を足す（JSON タグは `verdict`）。
2. `runHooks` が `g.ReviewVerdict` からその値を詰める。
   `model.Group.ReviewVerdict` はすでに存在する（`internal/model/model.go`）。
3. `runHookCommand` の環境変数に 2 本足す:
   - `SBNN_VERDICT=<verdict の文字列>`（空のときは空文字。既定値をでっち上げない）
   - `SBNN_BLOCKING=1` または `0`（`Verdict.Blocking()` の結果。
     これが無いと全部のフックがルールを書き直すことになる）
4. 既存の 6 本（`SBNN_GROUP` `SBNN_URL` `SBNN_SERVER` `SBNN_PORT` `SBNN_COMMENTS`
   `SBNN_REVIEW_NOTE`）は**綴りも順番も変えない**。

**範囲外（やらない）**: issue が求めている「`sbnn hook --help` に書く」は `cmd/hook.go` で、
そこは G8 の cmd-hook レーンが持っている。**触らない。**
その代わり、PR 本文の `## What changed` に
`--help documentation for SBNN_VERDICT / SBNN_BLOCKING is left to cmd/hook.go (owned by another lane).`
と 1 行書き、最終報告の「見送り」にも書く。
JSON と環境変数の追加はこの PR で完結するので、**PR 本文には `Fixes #25` を書いてよい。**

## 2 件目: #146 — フック URL が検証されない（task `t-0a6091`）

**この issue は担当ファイルの中だけでは全部は直せない。分かっている範囲を先に直す。**

issue が求めているのは 3 つ:

| 求められていること | どこにある | この PR で |
|---|---|---|
| 登録時に URL を検証して 400 | `handleAddHook`（`server.go`）/ `Store.AddHook`（`store.go`） | **やらない**（両方とも他レーンが使用中） |
| 配信の失敗を利用者に見える所へ出す | `GET .../hooks`（`server.go`）+ `model.Hook` | **やらない**（同上） |
| `postHook` が本文を drain して閉じる／読む量を制限する | `hook.go` | **やる** |

やること（すべて `internal/server/hook.go` の中）:

1. `postHook` に、**送る前の URL 検証**を入れる。`net/url.Parse` して、
   スキームが `http` / `https` のどちらでもない、あるいは `Host` が空なら、
   **リクエストを組み立てずに warn ログを 1 本出して戻る。**
   ログには URL とフック ID を含める。`postHook` はこの 2 スキームしか送れないので、
   それ以外を握って送ろうとするのは無意味な待ちを作るだけである。
2. その検証を `validateHookURL(raw string) error` のような**独立した関数**として
   `hook.go` に置く。登録時の 400 を実装するレーンが後からそのまま呼べるようにするため。
   （呼び出し側の `handleAddHook` を書き換えるのは**あなたの仕事ではない**。）
3. `postHook` はレスポンス本文を **drain してから Close する**
   （`io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))` のように**上限付きで**）。
   いまは閉じているが drain していないので接続が再利用されない。
   無制限に読まないこと（フック先が巨大な本文を返しても sbnn が詰まらないように）。

**PR 本文には `Fixes #146` を書かない。`Refs #146` を書く。**
そのうえで `## What changed` の末尾に、上の表の「やらない」2 行を英語でそのまま書く
（どのファイルが誰のものだから残したのか、が読めば分かる形で）。
最終報告の「見送り / 疑義」にも同じことを書く。

## テスト（必須）

`internal/server/hook_test.go` を新規に作る。`internal/server` の既存のテーブル駆動テスト
（`prompt_test.go` `proxy_test.go`）の書き方に合わせる。

- #25: `runHooks` が組んだ `ReviewEvent` の JSON に `"verdict"` が出ること。
  `runHookCommand` が組む環境に `SBNN_VERDICT` と `SBNN_BLOCKING` が
  期待どおりの値で入ること（`approved` → `SBNN_BLOCKING=0`、
  `changes-requested` → `SBNN_BLOCKING=1`）。
- #146: `validateHookURL` のテーブル駆動テスト。
  最低でも `http://x/y`（可）、`https://x/y`（可）、`not a url`（不可）、
  `file:///etc/passwd`（不可）、`http://`（Host が空なので不可）を含める。
  `postHook` については `httptest.NewServer` を立てて、本文が読み切られること
  （サーバ側で `r.Body` を最後まで書いても handler が返ってくること）を確かめる。

## 完了条件（**実行すれば真偽が決まるもの。自己申告しない**）

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')
```

`gofmt -l` が**何も出力しない**のが合格。加えて:

```bash
# 担当外のファイルを触っていないこと（何も出力しなければ合格）
git diff --name-only origin/main | grep -v -e '^internal/server/hook\.go$' -e '^internal/server/hook_test\.go$'

# #25: 3 つとも 1 行以上返れば合格
grep -n 'Verdict .*model\.Verdict' internal/server/hook.go
grep -n 'SBNN_VERDICT=' internal/server/hook.go
grep -n 'SBNN_BLOCKING=' internal/server/hook.go

# #146: 2 つとも 1 行以上返れば合格
grep -n 'func validateHookURL' internal/server/hook.go
grep -n 'io.Discard' internal/server/hook.go

# 既存の環境変数を消していないこと（6 行返れば合格）
grep -c -e 'SBNN_GROUP=' -e 'SBNN_URL=' -e 'SBNN_SERVER=' -e 'SBNN_PORT=' -e 'SBNN_COMMENTS=' -e 'SBNN_REVIEW_NOTE=' internal/server/hook.go
```

PR は 2 本（#25 は `Fixes`、#146 は `Refs`）。

## やること・やらないこと

- **push と PR 作成まで行う。マージはしない。**
- 担当外は触らない。見つけた問題は自分で直さず、最終報告に書く。
- 判断に迷って止まらない。既定を決めて進み、決めた内容と理由を報告に書く。
- **`server.go` / `store.go` が空くのを待たない。** 待つくらいなら `Refs` で出す。

## 完了報告

COMMON.md の報告書式に加えて、先頭に次の 4 行をこの綴りのまま書く:

```
slug: srv-hook
branch: gogo/issue-25 / gogo/issue-146
worktree: /home/user/wt/srv-hook
commit: <PR ごとの commit を両方とも列挙する>
```
