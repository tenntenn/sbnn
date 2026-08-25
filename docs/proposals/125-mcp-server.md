# 125 — MCP サーバを出す案

対象 issue: [#125](https://github.com/tenntenn/sbnn/issues/125)
状態: 方針（実装前）

この文書は方針だけを決める。実装は含まない。

順序（4 つの入口のどれを先に出すか）は #147 の担当なので、ここでは決めない。
ここで決めるのは **MCP をやるかどうかと、やるならどういう形か**である。

## 決めること

1. **やるか、やらないか。** issue は反対の論拠も自分で並べている
   （設計思想と逆行する / 説明面が 3 つに増えて #109〜#112 の食い違いが 3 倍になる）。
   両論併記で終わらせない。
2. やるなら **`sbnn mcp` は常駐サーバを起動するのか。**
3. **スキルの説明と MCP のツール説明を 1 つの出典から生成するか。**
   生成しないなら、食い違いをどう防ぐか。
4. **`wait_for_review` のタイムアウトの既定値と、超えたときの返り方。**

## 現状（コードを読んで確かめた事実）

**`internal/client` のメソッドは 16 個**（`internal/client/client.go`）:

`Status`（:45）`AddDiff`（:57）`Group`（:66）`AddComment`（:75）
`Comments`（:84）`Prompt`（:94）`ClearComments`（:127）`DeleteGroup`（:142）
`DeleteAllGroups`（:147）`Reviews`（:158）`Shutdown`（:181）
`SubmitReview`（:224）`Hooks`（:234）`AddHook`（:243）`DeleteHooks`（:252）
`WaitForReview`（:272）。加えて `BaseURL`（:37）。

**issue の「MCP サーバは `internal/client` の上の薄い層」は、7 つのうち
5 つについては正しい。残り 2 つには穴がある。** 1 つずつ突き合わせた結果:

| issue のツール | 対応するルート | 実際 |
|---|---|---|
| `send_diff` | `POST .../diffs` | **`baseDir` が余る**（下記） |
| `get_comments` | `GET .../comments` | **`include_resolved` に対応する引数が無い**（下記） |
| `get_prompt` | `GET .../prompt` | そのまま。`?resolved=` `?instruction=`（`internal/server/server.go:755`） |
| `add_comment` | `POST .../comments` | そのまま。**`snippet` は送らなくてよい**（下記） |
| `submit_review` | `POST .../review` | そのまま（`SubmitReviewRequest`、`internal/server/server.go:763`） |
| `wait_for_review` | `GET /_/events` | そのまま。`client.WaitForReview`（`internal/client/client.go:272`）が既にある |
| `status` | `GET /_/api/status` | そのまま（`Status`、`internal/server/server.go:296`） |

**穴 1: `baseDir`。** `AddDiffRequest`（`internal/server/server.go:416`）は
`Title` `BaseDir` `Content` `Labels` `Collapse` を持つ。
CLI は `workingDir()`（`cmd/root.go:464`）を入れる。
`BaseDir` は `source.AbsPath` が作業ツリーのファイルを探す起点であり、
空だと `AbsPath` が `""` を返して `diff.Reconstruct` に落ちる。
**シェルの無いエージェントには作業ディレクトリが無いので、
MCP 経由で送られた diff は、Markdown プレビューも画像も
「diff から再構成したもの」だけになる。** これは MCP 経路の
本質的な制約であり、隠さずに書くべきものである。

**穴 2: `include_resolved`。** `handleComments`（`internal/server/server.go` の
`handleComments`）は本体が 6 行で、**クエリを 1 つも読まない**:

```go
comments, found := s.store.Comments(name)
if !found {
	comments = []*model.Comment{}
}
writeJSON(w, http.StatusOK, comments)
```

`client.Comments`（`internal/client/client.go:84`）にも引数は無い。
では `sbnn comments --include-resolved` は何をしているかというと、
**`cmd/comments.go:111` で手元で捨てている**:

```go
if !commentsResolved {
```

**つまり `include_resolved` は API の機能ではなく CLI の機能である。**
MCP でそれを提供するなら、同じ絞り込みを MCP 側にもう 1 回書くことになる。
**これが「説明面が 3 つに増える」の、文章ではなく*コード*での現れ方である。**
issue が心配しているのは説明の食い違いだが、実際にはロジックも重複する。

**`snippet` はサーバが作る。** `handleAddComment`（`internal/server/server.go:596`）は
`path` が与えられて `snippet` が空のとき:

```go
req.Snippet = diff.Snippet(f, req.Side, req.StartLine, req.EndLine)
```

を実行し、それでも空なら
「`%s has no line %s in this diff`」で 400 を返す。
**MCP のクライアントは snippet を作らなくてよく、
存在しない行を指したらその場で叱られる。** これは MCP に向いた性質である。

**`sbnn mcp` が常駐サーバを起動すべきか — issue の前提は事実と違う。**
issue は「`sbnn mcp` が、他のどのサブコマンドもそうしているように
常駐サーバを起動するか（一貫性のためにはそうすべき）」と書いている。
**他のサブコマンドはそうしていない。** サーバを起動するのは
`cmd/root.go:237` の `ensureServer` だけで、`comment` `comments` `hook`
`submit` `wait` はどれも:

```go
return fmt.Errorf("no sbnn server found on %s", c.Addr)
```

で断る（`cmd/comment.go:118` / `cmd/comments.go:80` / `cmd/hook.go:80` /
`cmd/submit.go:89` / `cmd/wait.go:89`）。
**サーバを立てるのは「diff を受け取る入口」だけ、という区別である。**

**依存はまだ 2 つしかない。** `go.mod` の直接依存は
`github.com/spf13/cobra` と `github.com/pkg/browser` だけである。

**スキルは 1 ファイル・425 行。** `skills/sbnn/SKILL.md` が
`go:embed`（`skills/skills.go`）で埋め込まれ、`cmd/skill.go:62` の
`runSkill` が標準出力に出すか `--install` で書き出す。
**いまは CLI の使い方だけを、人間が書いた散文で説明している。**

## 選択肢

### A. やらない

CLI とスキルで足りるという立場を貫く。

- できるようになること: 説明面が 2 つのままで済む。
  #109〜#112 の食い違いを直すことに集中できる。
- 払う代償: **シェルの無いホストからは sbnn が存在しないままになる。**
  issue の言うとおり、ここだけは便利さの話ではなく可否の話である。

### B. やる。MCP の SDK を依存に足して `sbnn mcp` を書く

- できるようになること: プロトコルの細部を自分で書かなくてよい。
- 払う代償: **直接依存が 2 つから 3 つになる。** MCP の SDK は
  プロトコルの改訂に追従して動くので、sbnn のリリースが
  他人の都合で動くようになる。sbnn は `go.mod` を
  意図的に小さく保っている。

### C. やる。ただし依存を足さず、stdio の JSON-RPC を自分で書く

MCP の stdio 転送は **1 行 1 メッセージの JSON-RPC 2.0** であり、
`encoding/json` と `bufio` だけで書ける。sbnn が答える必要があるのは
`initialize` / `tools/list` / `tools/call` の 3 つ、
それに `notifications/initialized` を黙って捨てることである。

- できるようになること: 依存が増えない。プロトコルの表面が
  sbnn のリポジトリの中に見える形で残るので、
  「何に対応しているか」がコードを読めば分かる。
  ツールの定義は JSON スキーマの構造体になるので、
  スキルの表と機械的に突き合わせられる（決めること 3 に効く）。
- 払う代償: プロトコルが変わったとき自分で直す。
  対応する機能を絞る必要がある（リソース、プロンプト、
  サンプリングには対応しない）。

## 決定

**C を採る。MCP をやる。ただし新しい依存は足さず、
`encoding/json` だけで stdio の JSON-RPC を書く。**

理由:

- **A を採らない理由は、issue 自身が挙げているとおり「ここだけは
  可否の話」だから。** 他の 3 つ（アーティファクト・デスクトップ・拡張）は
  既に使える人をもっと楽にする話だが、これは使えない人を使えるようにする話である。
- **B を採らない理由は `go.mod` である。** 直接依存が 2 つしかない
  プロジェクトで、3 つ目が「他人のプロトコル実装」になるのは重い。
  そして上で見たとおり、必要なのは 3 つのメソッドに答えることだけで、
  SDK が提供するものの大半を使わない。
- **「設計思想と逆行する」という反対論について。** これは
  「シェルに繋ぎ役をやらせる」という立場のことだが、
  **その立場はシェルがある人に向けた立場である。** シェルの無いホストに
  対して「シェルを使え」は立場ではなく、単に届いていないだけである。
  MCP は CLI を**置き換えない**ので、立場は変わらない。

決めること 2〜4 の答え:

- **`sbnn mcp` は常駐サーバを起動する。** ただし理由は
  「他のサブコマンドと一貫させるため」ではない。上で確かめたとおり、
  他のサブコマンドは起動しない。**起動するのは `cmd/root.go` の
  diff を受け取る経路だけであり、`sbnn mcp` はまさにそれと同じ立場に立つ**
  ——シェルの無いエージェントにとって、これが唯一の入口だからである。
  `sbnn mcp` の中で `ensureServer` を呼ぶのではなく、
  **`send_diff` が最初に呼ばれたときに `ensureServer` を通す。**
  `sbnn mcp` の起動そのものではサーバを立てない
  （MCP のクライアントは起動時に全サーバを起こすので、
  sbnn を一覧に入れただけの人のマシンで勝手にプロセスが増えるのは行儀が悪い）。
- **スキルと MCP のツール説明は 1 つの出典から生成する。** ただし
  「SKILL.md を生成する」ではない。**逆に、ツールの定義を出典にする。**
  MCP のツール表（名前・引数・1 行説明）を Go の値として持ち、
  `sbnn skill --list` の隣に**その表を出す口を足して、
  SKILL.md の対応表と一致するかをテストで突き合わせる。**
  散文まで生成しようとすると SKILL.md が読めなくなる。
  **止めるべきなのは「引数の名前と数の食い違い」であって、文体ではない。**
  これは #114 が CLI とスキルの間でやろうとしていることと同じ仕組みである。
- **`wait_for_review` の既定のタイムアウトは 300 秒（5 分）、上限 600 秒。**
  超えたときは**エラーではなく、正常な返り**として
  `{"reviewed": false, "waited": 300}` を返す。理由: MCP の呼び出しは
  クライアント側にも独自のタイムアウトがあり、そこに引っかかると
  エージェントは「壊れた」と解釈する。**「まだです」は結果であって失敗ではない**
  ——これは `sbnn wait --timeout` が終了コード 2 を
  「まだレビューされていない」に割り当てているのと同じ判断である
  （`cmd/wait.go` の `exitNotReviewed`）。
  返り値には**必ずフックの案内を添える**: 5 分を超える待ちは
  `--on-review` の仕事である、と。

**ユーザの決めが要る点:** **`submit_review` をツールに入れるかどうか。**
`POST .../review` は「人間がレビューを終えたと言う」瞬間であり、
`Group.ReviewedAt` のコメント（`internal/model/model.go:328`）は
「a review is over when the reviewer says so」と書いている。
エージェントがそれを自分で叩けることは、CLI では既にそうなっている
（`sbnn submit`）ので新しい穴ではないが、MCP でツールとして
**目立つ場所に置く**ことには別の意味がある。**この提案の既定は
「入れる。ただし説明文に『これは人間の代わりに押すボタンではない』と書く」**。
入れないと決めるなら、それは持ち主の判断である。

## 後戻りしない第一歩

**7 つのツールのスキーマと、既存の HTTP API との対応表を固定する。**
実装するかどうかに関わらず、**API が CLI とは別に、外から使える形で
文書化されている**ことは #147 の言うとおり 3 つの案すべてが必要としている。

| ツール | 引数 | 返り | ルート |
|---|---|---|---|
| `send_diff` | `diff`(必須) `target` `title` `labels` `collapse` `base_dir` | `{group, url, files, additions, deletions}` | `POST /_/api/groups/{group}/diffs` |
| `get_comments` | `target` `include_resolved`(既定 false) | `[Comment]` | `GET /_/api/groups/{group}/comments` ＋ 手元で絞る |
| `get_prompt` | `target` `include_resolved` `instruction`(既定 true) | テキスト | `GET /_/api/groups/{group}/prompt?resolved=&instruction=` |
| `add_comment` | `target` `path`(必須) `line`(必須) `end_line` `body` `side`(既定 new) `question` `suggestion` | `Comment` | `POST /_/api/groups/{group}/comments` |
| `submit_review` | `target` `verdict` `note` | `Group` | `POST /_/api/groups/{group}/review` |
| `wait_for_review` | `target` `timeout`(既定 300、上限 600) | `{reviewed, waited, comments, verdict}` | `GET /_/events` |
| `status` | — | `Status` | `GET /_/api/status` |

この表で決まっていること:

- **`base_dir` は任意で、既定は空。** 空のときプレビューは
  diff から再構成されたものになる、と**ツールの説明文に書く**。
  同じマシンで動くクライアント（Claude Desktop）は渡せる。
- **`snippet` は引数に無い。** サーバが `diff.Snippet` で作る。
- **`line` / `end_line` は 1 始まり。** `end_line` を省くと 1 行。
  サーバが `startLine < 1` を 400 で断る（`internal/server/server.go:596` の検査）。
- **`include_resolved` は API ではなく手元の絞り込みである。**
  絞り込みは `cmd/comments.go:111` と**同じ 1 つの関数**に寄せる
  （どこに置くかは実装の判断だが、2 か所に書かないことは決める）。
- **`target` の既定は `"default"`**（`server.DefaultGroup`）。
  環境変数 `SBNN_TARGET` は**読まない**: MCP のクライアントに
  シェルの環境は無い。

## やらないこと

- **MCP のリソース / プロンプト / サンプリングへの対応。** ツールだけ。
- **`sbnn mcp` を HTTP や SSE の転送で出すこと。** stdio のみ。
  リモートは `--dangerously-allow-remote-access` の話であり、混ぜない。
- **`clear` / `delete` 系のツール。** `DELETE .../groups` は
  レビューを消す。エージェントに渡す道具ではない。
- **`hooks` のツール。** フックはシェルのコマンドを走らせる。
  シェルの無いホストのために作る入口から、シェルのコマンドを
  登録できるようにするのは筋が通らない。
- **SKILL.md の散文の生成。** 上で決めたとおり、突き合わせるのは
  引数の名前と数だけである。
- **順序の決定**（MCP を他の 3 つより先に出すかどうか）。#147 の担当。

## 次の 1 PR の範囲

**題: MCP のツール表を、Go の値として 1 か所に置く（プロトコルはまだ書かない）。**

依存を足さず、`sbnn mcp` も作らない。上の表を機械が読める形にするだけである。
プロトコルを書くのはその次で、この 1 本は**やらないと決めても捨てずに済む。**

触るファイル:

- `internal/` に `mcpschema/` を新設し、`schema.go`（新規）を置く。
  7 つのツールを `[]Tool{{Name, Description, Params []Param{Name, Type, Required, Description}}}`
  のような値として持ち、JSON スキーマに変換する関数を 1 つ持つ。
  **HTTP も stdio も触らない。純粋なデータと変換だけ。**
- `internal/mcpschema/` に `schema_test.go`（新規）。

完了条件:

- 7 つのツールがすべて定義されており、名前が上の表と一致する。
- **各ツールが、対応する HTTP のルートを文字列として持つ**
  （`POST /_/api/groups/{group}/diffs` など）。
- テストが、その 7 本のルートが `internal/server` の
  `mux.HandleFunc` に実在することを確かめる。
  **`internal/server/server.go` を `go/parser` で読んで
  登録済みのルート文字列を集め、突き合わせる。**
  外部コマンドを呼ばない。新しい依存を足さない。
  これで「ツール表が API から静かにずれる」ことが起きなくなる。
- JSON スキーマへの変換の出力を、テストで文字列として固定する。
- `go build ./... && go vet ./... && go test ./...` が通り、
  `gofmt -l` が何も出さない。

そのあとに来る PR（この 1 本には含めない）:

1. `sbnn skill` に、上の表を出す口を足し、`skills/sbnn/SKILL.md` の
   対応表と引数の名前・数が一致することをテストする（#114 と同じ仕組み）。
2. `cmd/` に `mcp.go` — stdio の JSON-RPC で
   `initialize` / `tools/list` / `tools/call` に答える。
   `tools/call` は `internal/client` を呼ぶ。
3. `README.md` と `skills/sbnn/SKILL.md` に `sbnn mcp` を載せる。
