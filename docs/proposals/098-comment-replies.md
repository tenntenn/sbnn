# 098 — コメントへの返信

対象 issue: [#98](https://github.com/tenntenn/sbnn/issues/98)
状態: 方針（実装前）

この文書は方針だけを決める。実装は含まない。

#98 は「返信できるようにする」という 1 つの機能に見えて、実際には
**保存形式・API・CLI・prompt の 4 か所で別々の決めごと**が未決のまま
積まれている。ここではその 4 つを決めて、1 PR ずつに割れる形にする。

## 決めること

1. **返信をどう表すか。** `Comment` に `ParentID` を足すのか、
   `POST .../comments/{id}/replies` という別の口を足すのか、
   既存の作成 API の本文に `parentId` を足すのか。3 つのうち 1 つを選ぶ。
2. **保存済みのセッションとの互換をどう扱うか。** 既存の
   `session-<port>.json` を読み込んだとき、また新しい形式のファイルを
   古い sbnn が読んだときに何が起きるか。
3. **「答えの付いた質問は未回答に数えない」をどこで判定するか。**
   `GET .../prompt` を作るときか、保存するときか。
4. **`sbnn comment --reply-to` の値は何か。** コメント ID か、行の指定か。

## 現状（コードを読んで確かめた事実）

**`Comment` の全フィールド**（`internal/model/model.go:147`）:

`ID` `Group` `DiffID` `FileID` `Path` `Author` `Side` `StartLine` `EndLine`
`Body` `Question` `Snippet` `Resolved` `CreatedAt` `UpdatedAt` の 15 個。
これに `MarshalJSON`（`internal/model/model.go:309`）が本文から取り出した
`suggestions` を足して返す。

このうち会話の状態を持つのは `Question` と `Resolved` の 2 つだけで、
**どちらも真偽値であり、誰が何に対して言ったかは持てない。**
`Author` は「誰が」だけを持ち、「何に対して」は持たない。
issue の指摘はこの点で正しい。

**「スレッド」の実体。** `web/src/components/CommentThread.tsx:13`:

```tsx
export function CommentThread({ group, comments, onChanged }: ThreadProps) {
  return (
    <div className="thread">
      {comments.map((c) => (<CommentItem ... />))}
```

並べているだけで、入れ子も順序の意味も無い。
何が同じ「スレッド」に入るかを決めているのは
`web/src/components/DiffFileSection.tsx:82` である:

```tsx
const key = anchorKey(c.side, c.endLine)
```

**`(side, endLine)` が同じものは同じ束になる。** `path` も `diffId` も
鍵に入っていない（そのファイルの節の中でだけ使われるので実害は無い）。
`startLine` は鍵に入らないので、**12-18 行に付けたコメントと
18 行だけに付けたコメントは、同じ束に並ぶ。**

**コメントの API は 5 本**（`internal/server/server.go:165`〜`170`）:

```go
mux.HandleFunc("GET /_/api/groups/{group}/comments", s.handleComments)
mux.HandleFunc("POST /_/api/groups/{group}/comments", s.handleAddComment)
mux.HandleFunc("PATCH /_/api/groups/{group}/comments/{id}", s.handleUpdateComment)
mux.HandleFunc("DELETE /_/api/groups/{group}/comments/{id}", s.handleDeleteComment)
mux.HandleFunc("DELETE /_/api/groups/{group}/comments", s.handleClearComments)
```

作成本文は `AddCommentRequest`（`internal/server/server.go:574`）で、
`DiffID` `FileID` `Author` `Path` `Side` `StartLine` `EndLine` `Body`
`Snippet` `Question` `Suggestion` を持つ。編集は
`UpdateCommentRequest`（`internal/server/server.go:685`）で
`Body` `Resolved` `Question` の 3 つだけを差し替える。

**保存形式**（`internal/server/store.go:63`）:

```go
type persisted struct {
	Version int            `json:"version"`
	Seq     int            `json:"seq"`
	Groups  []*model.Group `json:"groups"`
	Rounds  map[string]int `json:"rounds,omitempty"`
}
```

`Load`（`internal/server/store.go:75`）は `json.Unmarshal` するだけで、
**`p.Version` を読んで分岐していない。** コメントは `model.Group.Comments`
の中に丸ごと入っている。ID は `nextID("c")`（`internal/server/store.go` の
`nextID`）が `c1` `c2` … と採番し、`Seq` に持ち越される。

**削除は参照を知らない。** `Store.DeleteComment`（`internal/server/store.go:615`）は
ID の一致するものをスライスから外すだけで、他のコメントを一切見ない。
`ClearComments(group, resolvedOnly=true)`（`internal/server/store.go:640`）は
resolved なものだけを落とす。**返信を足すと、この 2 つが親を消して
子を残せてしまう。**

**`cmd/comment.go` の CLI。** `runComment`（`cmd/comment.go:110`）は
位置引数を `parseLineSpec`（`cmd/comment.go:408`）に渡し、
`path:12-18` の形から `path` / `start` / `end` を取る。
フラグ（`cmd/comment.go:85`）は `--target` `--port` `--bind` `--message`
`--suggest` `--suggest-file` `--author` `--question` `--side` `--diff`
`--json` `--json-output` で、**`--reply-to` は無い。**
`--json` は stdin からコメントの配列を読む一括経路
（`readBulkComments`、`cmd/comment.go:339`）である。

**質問の数え方。** `internal/server/prompt.go:44`:

```go
questions := 0
for _, c := range comments {
	if c.Question {
		questions++
	}
}
```

数えているのは、`Resolved` で絞ったあとの一覧に対する `Question` の数で、
**答えがあるかどうかは見ていない。** 出力側は
`internal/server/prompt.go:69` の「This one is a question: answer it.」と、
`internal/server/prompt.go:102` の締めの段落である。

## 選択肢

### A. `Comment` に `ParentID` を足し、既存の作成 API に `parentId` を通す

`model.Comment` に 1 フィールド、`AddCommentRequest` に 1 フィールド、
CLI に `--reply-to <comment-id>` を足す。返信は普通のコメントで、
親を指しているものがそう呼ばれるだけになる。

- できるようになること: API が 1 本も増えない。保存形式は
  フィールド 1 つの追加で済む。`GET .../comments` は今までどおり
  平らな一覧を返し、木にするのは読む側（prompt と UI）になる。
  `PATCH` も `DELETE` も返信にそのまま効く。
- 払う代償: 「返信の `Side` / `StartLine` / `EndLine` / `Snippet` は何か」を
  決めないといけない。放っておくと親と食い違った錨を持つコメントができる。

### B. `POST .../comments/{id}/replies` という専用の口を足す

作成 API は触らず、返信だけ別の口にする。

- できるようになること: URL が意図を語る。親の存在は URL の
  `{id}` を引くだけで確かめられる。
- 払う代償: **API が 1 本増える**（現在 `/_/api/` 23 本 +
  `GET /_/events` の 24 本）。返信の中身は結局 `AddCommentRequest` と
  ほぼ同じものになり、2 つの入口が同じ検証を持つことになる。
  そして `ParentID` は結局要る — 保存されたものが親を指せなければ
  木は組み立てられない。**つまり B は A の上に URL を 1 本足す案であって、
  A の代わりにはならない。**

### C. 返信を `Comment` にせず、`Comment` の中に配列で持つ

`Comment.Replies []*Reply` のような入れ子にする。

- できるようになること: 木が構造そのもので表現され、
  親を消せば返信も消える。
- 払う代償: 返信が `ID` を持たないなら編集も解決もできない。
  持つなら `Comment` とほぼ同じ型が 2 つになり、
  `PATCH .../comments/{id}` が返信に効かなくなる。
  `web/src/types.ts` と `internal/export/export.go` の
  `Payload.Comments` も形が変わる。**保存済みのファイルとも非互換になる**
  （古い平らな一覧を読み直す道が要る）。

## 決定

**A を採る。** `model.Comment` に `ParentID string json:"parentId,omitempty"`
を足し、`AddCommentRequest` に同名のフィールドを足す。
専用の URL（B）は足さない。入れ子の型（C）は作らない。

理由:

- **B は A を含むので、B を選ぶ理由は「URL が読みやすい」だけになる。**
  その対価が API 1 本と検証経路 2 つは高い。あとから `replies` の
  URL を足したくなれば足せるが、逆は消せない。
- **C は保存済みのセッションと非互換になる。** A なら互換の心配が要らない
  （下記）。
- **`PATCH` と `DELETE` がそのまま効くのが A だけ。** 返信を解決したり
  直したりできないスレッドは、issue が挙げている 3 つの用途
  （「直した」「反対だ、理由は」「どちらのつもりか」）の 2 つ目と 3 つ目を
  満たさない。

決めること 2〜4 の答え:

- **互換は「何もしなくてよい」。** `Load`（`internal/server/store.go:75`）は
  `Version` を見ずに `json.Unmarshal` するだけなので、`parentId` の無い
  古いファイルは全コメントが `ParentID == ""`（＝根）になる。
  逆に新しいファイルを古い sbnn が読んでも `encoding/json` が
  知らないキーを捨てるだけで、コメントは平らな一覧として読める。
  **`persistVersion` は上げない。** 上げると古い sbnn が拒否する道が
  将来できてしまい、実際には要らない非互換を宣言することになる。
  ただし**親を消したときの後始末は決めないといけない**:
  `DeleteComment` は返信を**親ごと消す**（孤児を残さない）。
  `ClearComments(resolvedOnly)` は**根が resolved でも、返信のどれかが
  未解決ならスレッドごと残す**。どちらも「スレッドは 1 つの単位」という
  同じ規則の言い換えである。
- **「答えの付いた質問」は `Prompt` の中で判定する。保存時ではない。**
  保存時に親の `Question` を落とすと、質問だったという事実が消えて
  もう一度尋ねられなくなる。判定は
  「`Question` が真で、かつ `ParentID` がこのコメントを指す返信が
  1 つも無いもの」を未回答とする。`internal/server/prompt.go:44` の
  数え上げをそう書き換える。
- **`--reply-to` の値はコメント ID（`c12` の形）。** 行の指定
  （`path:42`）は「同じ行の何に対してか」を決められないので採らない。
  `--reply-to` を渡したときは位置引数を**受け取らない**:
  返信の錨は親から継ぐ。すなわち `Path` `Side` `StartLine` `EndLine`
  `DiffID` `FileID` はサーバが親から複写し、`Snippet` は
  **空にする**（同じコードを 2 回貼っても読む人には冗長で、
  `internal/server/prompt.go:75` は空の snippet を黙って飛ばす）。
  ID はいま `sbnn comments --format json` で読める。

**ユーザの決めが要る点:** 返信が何段まで入れ子になれるか。
**この提案の既定は「1 段」**（返信への返信は、同じ根の下の兄弟になる。
すなわち `ParentID` を渡されたコメントがすでに返信なら、
サーバはその根に付け替える）。GitHub がそうしており、
prompt の出力も読みやすい。多段にしたいかどうかは、
sbnn の会話がどれくらい長くなる想定かという持ち主の判断である。

## 後戻りしない第一歩

**返信を含んだときの `prompt` の出力そのものを、この提案の中で固定する。**
`ParentID` を採るか `replies` を採るかに関わらず、
「スレッドをスレッドとして読ませる」が何を意味するかはこれで決まる。

いまの出力（`internal/server/prompt.go:62` から）:

~~~
## 1. internal/server/store.go:615-620

From: alice

This one is a question: answer it.

```
 func (s *Store) DeleteComment(group, id string) bool {
```

Why does this not look at the other comments?
~~~

返信が付いたあとの出力:

~~~
## 1. internal/server/store.go:615-620

From: alice

This one is a question, and it has been answered below.

```
 func (s *Store) DeleteComment(group, id string) bool {
```

Why does this not look at the other comments?

### Reply from claude

Nothing referred to another comment until now. #98 changes that, so this
deletes the replies with the parent.

### Reply from alice

Good. Please say so in the doc comment as well.
~~~

決めていること:

- 返信は**番号を持たない**。番号が付くのは根だけで、
  「3 番のコメントに対応せよ」という指示が返信で崩れない。
- 見出しは `### Reply from <author>`。`Author` が空（ブラウザの
  レビュアー）のときは `### Reply from the reviewer`。
- **返信の snippet は出さない。** 錨は親と同じなので繰り返しになる。
- 質問に返信が付いているとき、冒頭は
  「This one is a question: answer it.」から
  「This one is a question, and it has been answered below.」に変わり、
  `internal/server/prompt.go:51` の数え上げ（`asking`）からも外れる。
- 締めの段落（`internal/server/prompt.go:102`）は、
  **未回答の質問が 1 つも無ければ出さない。**

この形が決まっていれば、保存形式が A でも B でも C でも
`Prompt` の実装は同じものになる。

## やらないこと

- **UI の入れ子表示の細部。** `CommentThread.tsx` が返信を
  どうインデントするか、返信フォームをどこに出すかは実装の判断とする。
  この提案が決めるのは「返信は親の下に、親と同じ行の束の中で描く」までである。
- **`Comment` に「宛先の人」を足すこと。** 返信は親を指すだけで、
  誰かを名指しはしない。
- **通知。** 返信が付いたことを誰かに知らせるのは #105 / #125 の領分である。
- **エクスポートされたページでの返信。** `internal/export/export.go` の
  ページはサーバを持たないので、返信は**表示だけ**できればよい。
  書けるようにはしない。
- **多段の入れ子。** 上で 1 段と決めた。
- **`sbnn comments` の出力形式の変更。** 返信が一覧に平らに出ることは
  当面変えない（`--format json` は `parentId` を持つので、
  読む側が組み立てられる）。

## 次の 1 PR の範囲

**題: コメントが親を指せるようにする（保存と API まで。UI と prompt は次）。**

触るファイル:

- `internal/model/model.go` — `Comment` に `ParentID string` を 1 つ足す。
  `json:"parentId,omitempty"`。
- `internal/server/server.go` — `AddCommentRequest` に `ParentID` を足し、
  `handleAddComment` で次を行う: 親が同じ group に存在しなければ 400。
  親自身が返信なら、その根に付け替える（1 段の規則）。
  `Path` `Side` `StartLine` `EndLine` `DiffID` `FileID` は親から複写し、
  要求の同名フィールドは無視する。`Snippet` は空にする。
- `internal/server/store.go` — `DeleteComment` が返信も消すようにする。
  `ClearComments(resolvedOnly=true)` が、未解決の返信を持つスレッドを残す。
- `web/src/types.ts` — `Comment` に `parentId?: string` を足す（型だけ。
  描画は変えない）。
- `internal/server/server_test.go` / `internal/server/store_test.go` —
  表駆動で足す。

完了条件:

- `POST .../comments` に存在しない `parentId` を渡すと 400 になる。
- 返信を作ると、返ってきたコメントの `path` / `side` / `startLine` /
  `endLine` が親と同じで、`snippet` が空である。
- 返信への返信が、返信ではなく根に付く。
- 親を `DELETE` すると返信も消える（`GET .../comments` に残らない）。
- 根だけ resolved のスレッドが `DELETE .../comments?resolved=true` で残る。
- **`parentId` の無い既存の `session-<port>.json` を読み、
  全コメントが根として出ることをテストする。** これが互換の証拠になる。
- `go build ./... && go vet ./... && go test ./...` が通る。
  `gofmt -l` が何も出さない。

そのあとに来る PR（この 1 本には含めない）:

1. `internal/server/prompt.go` を上の出力形式にする。未回答の質問の
   数え方を変える。`internal/server/prompt_test.go` に出力そのものを固定する。
2. `cmd/comment.go` に `--reply-to` を足す。位置引数と排他にする。
3. `web/src/components/CommentThread.tsx` で返信を親の下に描き、
   返信フォームを出す。
