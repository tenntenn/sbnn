# 097 — ハンクの前後を開く

対象 issue: [#97](https://github.com/tenntenn/sbnn/issues/97)
状態: 方針（実装前）

この文書は方針だけを決める。実装は含まない。#97 は「やるかどうか」ではなく
「どの形でやるか」が決まっていないために大きく見えている issue であり、
ここで形を決めれば残りは 1 PR ずつに割れる。

## 決めること

1. **展開した行はサーバが返すのか、ブラウザが持つのか。** 言い換えると、
   展開のための API を 1 本足すのか、ファイル全文を先にブラウザへ渡して
   ブラウザが切り出すのか。
2. **ディスク上のファイルが diff の文脈行と一致しないとき、何を比べて、
   一致しなければ何を出すのか。** issue はこれを #40 の誤ラベル問題と
   同じ検査だと言っている。比較の対象（どの行を、どちら側の何と）を決める。
3. **展開した行にコメントを付けられるのか。** issue は「付けられてはいけない」
   と言っている。その主張がコード上で成り立つかを確かめ、決める。
4. **ファイルがディスクに無いとき（`source.FromDiff`）に何を見せるのか。**
   展開の入口を消すのか、出すが押せなくするのか、押せるが gap だけを埋めるのか。

## 現状（コードを読んで確かめた事実）

**全文はすでにサーバが読んでいる。** `internal/source/source.go:40` の `NewSide` が
作業ツリーのファイルを読み、読めたときは `Complete: true` を返す:

```go
func NewSide(baseDir string, f *model.File) Result {
	if path := AbsPath(baseDir, f.Path()); path != "" {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			if b, err := os.ReadFile(path); err == nil {
				return Result{Content: string(b), Kind: FromWorktree, Path: path, Complete: true}
```

読む前に `AbsPath`（`internal/source/source.go:61`）が封じ込め検査をしている。
diff のパスは sbnn が書いたものではないので、送信元ディレクトリの外に出るパスは
`""` になり、`Reconstruct` に落ちる。**展開機能は新しい読み取り経路を必要としない。
この封じ込め検査の内側にすでに全文がある。**

**それを外に出す HTTP の口もすでにある。** `internal/server/server.go:163`:

```go
mux.HandleFunc("GET /_/api/groups/{group}/diffs/{diff}/files/{file}/content", s.handleFileContent)
```

`handleFileContent` は `previewer.content`（`internal/server/preview.go:63`）を呼び、
`source.NewSide` の結果をそのまま `{path, source, complete, content}` として返す。

**足りないのはここである。** `previewer.content` の 1 行目が
`previewableText(f)` を呼んでおり、それが `internal/server/preview.go:116` で
Markdown でもノートブックでもないファイルを拒否する:

```go
case !f.IsMarkdown && !f.IsNotebook:
	return fmt.Errorf("%w: %s has no preview", errNotPreviewable, f.Path())
```

つまり `.go` のファイル — #97 が問題にしているまさにその場合 — は、
全文がディスクにあってサーバもそれを読めるのに、この API では 400 が返る。
**「diff ペインだけが全文を使うことを拒んでいる」の正体はこの 1 つの分岐である。**

**`Snippet` は展開には使えない。** `internal/diff/reconstruct.go:51` の
`Snippet(f, side, start, end)` は `f.Hunks` の中しか歩かない:

```go
for _, h := range f.Hunks {
	for _, l := range h.Lines {
		num := l.NewNumber
```

diff に載っていない行は `Hunks` に無いので、`Snippet` は範囲を広げても
何も増やさない。`Snippet` が既にあるという事実は「行範囲を切り出して
diff マーカー付きのテキストにする書き方はもう決まっている」という意味であって、
「展開の半分ができている」という意味ではない。展開が要るのは
**ディスク側の行範囲**であり、それを返す関数は今どこにも無い。

**gap の大きさは既に計算できる。** `internal/model/model.go:56` の `Hunk` は
`NewStart` と `NewLines` を持つので、隣り合うハンクの間の行数は
`next.NewStart - (prev.NewStart + prev.NewLines)` で出る。
`Reconstruct`（`internal/diff/reconstruct.go:20`）は同じ量を
`h.NewStart - next` として計算し、`GapMarker` に埋めている。

**ハンク見出しの描画位置。** `web/src/components/DiffFileSection.tsx:324` に
unified 側、`:432` に split 側があり、どちらも同じ形をしている:

```tsx
<tr className="hunk">
  <td className="num" />
  <td className="num" />
  <td className="code" colSpan={2}>{hunk.header}</td>
</tr>
```

展開の操作子はこの行に入る。**2 か所ある**ので、どちらにも同じものを置くか、
この行を 1 つのコンポーネントに括り出すかを実装時に決めることになる。

**コメントの行番号がどこから来るか。** `internal/model/model.go:147` の `Comment` は
`Path` / `Side` / `StartLine` / `EndLine` を持ち、`Snippet` フィールドに
「レビューされたコード」を焼き付けている。サーバ側 `handleAddComment`
（`internal/server/server.go:596` 付近）は `AddCommentRequest` を受け取り、
その `StartLine` / `EndLine` を検査せずに保存する。**したがって「展開行に
コメントが付かない」はサーバが保証していない。** 保証しているのは UI だけである。

## 選択肢

### A. ブラウザが全文を先に受け取り、切り出しもブラウザがやる

diff を開いたときに（あるいはハンク見出しを押したときに 1 回だけ）
ファイル全文を取り、展開はすべてブラウザ内の配列操作にする。

- できるようになること: 展開のたびに往復しない。2 回目以降は即時。
  サーバ側の追加は「既存の content API から Markdown 限定の分岐を外す」だけ。
- 払う代償: 大きなファイルを、読まれないかもしれないのに丸ごと送る。
  ブラウザ側に「ファイル全文」という、いま存在しない状態が増える。
  一致検査（決めること 2）をブラウザで書くことになり、
  同じ検査を使うはずの #40（サーバ側のラベル）と実装が分かれる。

### B. 展開範囲を指定する API を 1 本足し、サーバが行だけを返す

`GET .../files/{file}/lines?side=new&start=&end=` のような口を足す。
サーバは `source.NewSide` の結果を行に割り、要求された範囲だけを返す。

- できるようになること: 送る量が要求した分で収まる。一致検査がサーバ側の
  1 か所に載り、#40 と同じ関数を共有できる。ブラウザは「返ってきた行を
  ハンクの間に差し込む」だけで、全文という状態を持たない。
- 払う代償: 展開のたびに往復する。API が 1 本増える（現在 `/_/api/` 23 本 +
  `GET /_/events` 1 本 = 24 本）。

### C. diff を返すときに前後 N 行をあらかじめ載せておく

`POST .../diffs` の時点でハンクの前後を広げて `Hunks` に混ぜてしまう。

- できるようになること: UI もサーバも新しい口が要らない。
- 払う代償: **diff が言ったことと、sbnn がディスクから足したことが
  同じ `Hunks` に混ざる。** コメントの行番号、`Snippet`、エクスポート、
  `Reconstruct` のすべてが、diff の保証しない行を diff の行として扱う。
  issue が「#23 が裏側から再発する」と言っているのはこの経路である。
  さらに N を後から変えられず、展開「全部」ができない。

## 決定

**B を採る。展開範囲を指定する API を 1 本足し、返ってきた行は
`Hunks` に混ぜずに、UI 側の別のものとして描く。**

理由は 3 つある。

1. **一致検査をサーバの 1 か所に置けるのが B だけだから。** 決めること 2 は
   #40 と同じ検査であり、#40 はサーバ側の話である。A だとブラウザに、
   C だとパーサに同じ判断が生まれる。
2. **C は `model.File` を汚す。** `Hunks` は「diff が言ったこと」であり、
   ディスクから足した行をそこへ入れると、`Snippet` が diff の保証しない
   行をコメントに焼き付けるようになる。これは #128 が別の角度から
   問題にしている「diff が言ったことと sbnn が決めたことの混在」そのもので、
   同じ間違いを 1 つ増やすことになる。
3. **A の利点（往復しない）は B でも後から足せる。** ブラウザが返ってきた
   範囲を覚えておけば 2 回目は往復しない。逆に A から B へは戻れない。

決めること 2〜4 の答え:

- **一致検査は「diff の文脈行 (`model.LineContext`) が、ディスク上の同じ
  新側行番号の行と、文字列として等しいか」で行う。** 追加行・削除行は
  比べない（追加行はディスクにあるはずだが、比較の対象を最小にするほど
  誤検知が減る）。1 行でも食い違えば、そのファイルの展開は**できない**とし、
  ハンク見出しの操作子を出さずに理由を出す。「ディスクのファイルは
  この diff より新しい」という文である。**部分的に展開する道は採らない。**
  どこまで信じてよいか読者に判断させることになるからである。
- **展開した行にコメントは付けられない。** 上で確かめたとおり、これは
  いまサーバが保証していないので、**UI で行番号を選択不能にするだけでは
  不十分**である。展開行は `Comment.Snippet` の材料にもならない
  （`Snippet` は `Hunks` しか見ないので、そもそも空になる）。
  この決定は #23 と同じ根拠に立つ: sbnn が保証できない行番号を
  コメントの錨にしない。
- **ディスクに無いとき（`source.FromDiff`）は、展開の操作子を出さない。**
  出して押せなくするより出さないほうがよい。`Reconstruct` が
  `complete=false` を返す状態、つまり gap があることは
  すでに `FileContentResponse.Complete` として API に出ている。
  ハンク見出しには「この diff の外は分からない」旨の静かな注記だけを置く。

**ユーザの決めが要る点:** 展開の上限。「gap を全部開く」は、
ハンク 2 つの間が 5000 行あるファイルでその 5000 行を送る。
既定の上限（例: 1 回 500 行、それ以上は繰り返し押す）を置くか、
上限なしにするかは、sbnn がどれくらいの diff を相手にする道具かという
持ち主の判断である。**それ以外はこの文書で決まっている。**

## 後戻りしない第一歩

**一致検査だけを、独立した関数として決める。**

```go
// package source
//
// Matches reports whether the working tree file agrees with what the diff
// says about it: every context line of every hunk must be byte-identical to
// the line with the same new-side number in content. line is the first
// disagreeing new-side line number, or 0.
func Matches(f *model.File, content string) (ok bool, line int)
```

これを選ぶ理由:

- **展開を作らなくても価値がある。** #40 は「作業ツリーから読んだ、と
  ラベルに書いてあるのに中身が diff と違う」問題であり、
  `previewer.content` が返す `source` フィールドは今この検査をしていない。
  `Matches` があれば #40 はこの関数を呼ぶだけになる。
- **A / B / C のどれを選んでも要る。** 展開の形が変わっても、
  「ディスクを信じてよいか」の判定は同じ 1 つである。
- **`internal/diff` にも `internal/server` にも依存しない。**
  `model.File` と文字列だけを受け取る純粋関数なので、テストが表駆動で書ける。

この PR では実装しない。次の PR の中身である。

## やらないこと

- **展開行へのコメント。** 上で「付けられない」と決めた。将来やるなら
  それは別の issue であり、`Comment` が diff の外の行を指せるように
  なるという別の決定を要する。
- **`--context` 相当の CLI フラグ。** 展開は読む人の操作であって、
  diff を送る側が決めることではない。
- **git を呼ぶこと。** `AGENTS.md` の「sbnn must not shell out to git」に従う。
  ここで足す経路はどれも `os.ReadFile` の 1 本である。
- **ハンク見出し行の共通化そのもの。** `DiffFileSection.tsx` の 2 か所を
  1 つに括るかどうかは実装の判断であり、この提案は形を決めない。
- **エクスポートされたページでの展開。** `internal/export/export.go` の
  `Payload` はサーバを持たないので、展開はサーバのある画面だけの機能とする。
  エクスポート側でどう見せるかは #115 側の話にする。

## 次の 1 PR の範囲

**題: 作業ツリーのファイルが diff と一致するかを判定する。**

触るファイル:

- `internal/source/` に `match.go`（新規）— 上の `Matches` を実装する。
- `internal/source/` に `match_test.go`（新規）— 表駆動。既存の
  `internal/source/source_test.go` の書き方に合わせる。

完了条件:

- `Matches` が次の場合を返り値で区別する:
  一致する / 文脈行が 1 行違う（その行番号を返す） /
  ディスクの行数が足りず文脈行に届かない / ハンクが 0 個（`ok=true, line=0`）/
  `content` が空文字列。
- 改行コードの扱いを決めてテストに書く（`\r\n` で終わる行を
  `\n` の行と等しいとみなすかどうか。**既定は「みなさない」**。
  そこを緩めると「ディスクを信じてよい」の意味が曖昧になる）。
- `model.Line.NoNewline` が立っている最終行の扱いをテストに書く。
- `go build ./... && go vet ./... && go test ./...` が通る。
- `internal/server` からはまだ呼ばない。呼ぶのはその次の PR
  （#40 の修正、または展開 API のどちらか先に来たほう）。

その次に来る PR（この 1 本には含めない）:

1. `GET /_/api/groups/{group}/diffs/{diff}/files/{file}/lines` を足す。
   引数は `side` / `start` / `end`。`Matches` が false なら 409 と理由を返す。
2. `web/src/components/DiffFileSection.tsx` のハンク見出しに操作子を足し、
   返ってきた行を `Hunks` とは別の行として描く（行番号は選択不能）。
