# 128 — diff パーサをパッケージとして公開する案

対象 issue: [#128](https://github.com/tenntenn/sbnn/issues/128)
状態: 方針（実装前）

この文書は方針だけを決める。実装ファイルは 1 行も変えない。
同じ PR で足すのは `internal/diff/apisurface_test.go` の 1 本だけである。

## 決めること

1. **いま公開するか、v1 まで待つか。**
2. 公開するなら **`internal/model` ごと出すのか、`diff` 用の最小の型を切り出すのか。**
3. **`model.File` から「UI が決めたこと」を分離するのは、公開するかどうかと
   独立にやる価値があるか。** issue はそう主張している。真偽を判断する。
4. 分離するなら、`internal/server` と `web/src/types.ts` にどこまで波及するか。

## 現状（コードを読んで確かめた事実）

**公開されている識別子は 9 つではなく 11 個である。**

issue は 9 つの関数を挙げている。実際に `go/ast` で数え直すと、
関数 9 つに加えて**公開された定数が 2 つある**:

| 種類 | 名前 | 場所 |
|---|---|---|
| const | `GapMarker` | `internal/diff/reconstruct.go:13` |
| const | `UnnamedPath` | `internal/diff/parse.go:628` |
| func | `GeneratedMarker` | `internal/diff/generated.go:35` |
| func | `VisibleTop` | `internal/diff/generated.go:55` |
| func | `IsMarkdown` | `internal/diff/parse.go:30` |
| func | `IsImage` | `internal/diff/parse.go:51` |
| func | `ImageContentType` | `internal/diff/parse.go:58` |
| func | `IsNotebook` | `internal/diff/parse.go:63` |
| func | `Parse` | `internal/diff/parse.go:68` |
| func | `Reconstruct` | `internal/diff/reconstruct.go:20` |
| func | `Snippet` | `internal/diff/reconstruct.go:51` |

`GapMarker` は特に見落としてはいけない。printf の書式である:

```go
const GapMarker = "<!-- sbnn: %d line(s) not included in the diff -->"
```

`Reconstruct` の出力を読む側は、この文字列に合わせて gap を見つけることになる。
**公開すれば、この書式は約束の一部になる。**
「9 つの関数」という数え方はこれを勘定に入れていない。

**`internal/model` は一緒に出るしかない。** `Parse` の型は
`func Parse(src string) []*model.File` であり、`Reconstruct` `Snippet`
`VisibleTop` も `*model.File` を取る。`internal/diff` の 3 ファイル
（`parse.go` `generated.go` `reconstruct.go`）はすべて
`github.com/tenntenn/sbnn/internal/model` を import している。
**issue の言うとおりで、ここに選択肢は無い。**

**「diff が言ったこと」と「UI が決めたこと」の混在 — フィールド単位で見た結果。**

issue は `ID` `Folded` `FoldReason` `ViewMode` `IsMarkdown` を挙げ、
「だから `finalize` が `ViewMode` と `Folded` をパーサの中で設定している」
と書いている。**この最後の一文は事実と違う。** 5 つを 1 つずつ見る:

- **`Folded` / `FoldReason` — パーサは触っていない。**
  `internal/diff/` に `Folded` という文字列は**1 回も現れない**。
  設定しているのは `internal/server/fold.go` の 5 か所だけである
  （`:31` `:32` `:36` `:37` `:41`）:

  ```go
  f.Folded = true
  f.FoldReason = "the sender asked for it (--collapse " + pattern + ")"
  ```

  **つまりこの 2 つについては、分離はすでに済んでいる。**
  パーサは触らず、アプリの層だけが設定している。
- **`ViewMode` — `finalize` が設定しているのは事実。**
  `internal/diff/parse.go:515` 付近:

  ```go
  case f.Status == model.StatusAdded, f.Status == model.StatusDeleted, f.IsBinary, f.Deletions == 0:
  	f.ViewMode = model.ViewUnified
  default:
  	f.ViewMode = model.ViewSplit
  ```

  ただし**その材料はすべて diff が言ったこと**（`Status` `IsBinary`
  `Deletions`）である。UI の「決定」ではなく、diff の事実から出る
  **既定値の先計算**である。読む側は無視して自分で決められる。
- **`IsMarkdown` / `IsImage` / `IsNotebook` — 同じく先計算である。**
  `finalize` は `f.IsMarkdown = IsMarkdown(f.Path())` を呼ぶだけで、
  **その関数自体が公開 API の一部**である。つまり
  「パスから決まる純粋な関数の結果を、構造体にも入れてある」だけで、
  情報は増えても減ってもいない。
- **`ID` — これだけが本当に sbnn のものである。**
  `fileID`（`internal/diff/parse.go:525`）が
  `fmt.Sprintf("f%d-%s", index+1, sha256(path)[:8])` を作る。
  **diff の中の位置（index）に依存する**ので、同じファイルでも
  何番目に出てくるかで変わる。そして `model.Comment.FileID` が
  これを指しており、`internal/server/server.go` の
  `handleAddComment` はこの ID で錨を検証する。
  **公開したら、この採番規則が約束になる。**

まとめると、**issue が挙げた 5 つのうち、パーサが設定していて
かつ diff の外から来る値は `ID` 1 つだけ**である。
2 つは既に分離済み、3 つは diff の事実からの派生である。

**分離したときの波及（見積もり。実装しない）。** 参照数を数えた:

| フィールド | 非テストの参照数 | どのファイル |
|---|---|---|
| `Folded` | 3 | `internal/server/fold.go` のみ |
| `FoldReason` | 3 | `internal/server/fold.go` のみ |
| `ViewMode` | 2 | `internal/diff/parse.go` のみ |
| `IsMarkdown` | 5 | `cmd/root.go` `internal/server/preview.go` `internal/export/export.go` `internal/diff/parse.go` |
| `IsImage` | 3 | `internal/server/preview.go` `internal/export/export.go` `internal/diff/parse.go` |
| `IsNotebook` | 3 | `internal/server/preview.go` `internal/export/export.go` `internal/diff/parse.go` |

**Go 側の波及は小さい。** しかし `web/src/types.ts` の `FileDiff` は
これら 6 つを**全部持っている**（`viewMode` `isMarkdown` `isImage`
`isNotebook` `folded?` `foldReason?`）。`model.File` は
JSON タグ付きの構造体で、**そのまま線を渡る**
（`internal/export/export.go` の `Payload.Diffs []*model.Diff` にも入る）。
**したがって分離の本当の代償は Go の型ではなく、
JSON の形が変わることである。** エクスポート済みのページ
（`export.PayloadVersion = 1`）も読めなくなる。

## 選択肢

### A. いま公開する（`diff` と `model` を `pkg/` などへ移す）

- できるようになること: linter やボットが `Parse` を使える。
  issue の言う差別化（git を呼ばない）が届く。
- 払う代償: **`model.File` への今後の変更が全部互換の問題になる。**
  進行中のものだけでも #96（ファイルごとの「読んだ」印）、
  #124（ファイル単位のコメント）、#97（展開した文脈）が
  `model.File` を触る。しかも `version.Version` は `"dev"` で、
  タグ付きのリリースもバイナリも無い（#101）。
  **`ID` の採番規則と `GapMarker` の書式まで約束することになる。**

### B. v1 まで待つ

- できるようになること: `model` が落ち着くまで自由でいられる。
  待つ間に失うものは、いま `Parse` を使いたい第三者だけである。
- 払う代償: その第三者は今日は使えない。

### C. `model` を分けてから公開する（issue の「Split the model」）

`diff` 専用の最小の型を出し、sbnn の関心（`ID` など）は
アプリ側の構造体に残す。

- できるようになること: パーサが本当に再利用可能な形になる。
- 払う代償: **上で測ったとおり、代償は JSON の形の変更である。**
  `web/src/types.ts`、`internal/export` の払い出し、
  保存済みの `session-<port>.json`、エクスポート済みの HTML が
  全部その形を持っている。そして**それをやっても
  「いま公開するか」は別に決まらない** —— 型が綺麗になっても
  `model.File` を変える issue が 3 本控えていることは変わらない。

## 決定

**B を採る。いまは公開しない。v1 まで待つ。**

理由:

- **`internal/` は、この若さのプロジェクトが持っておくべき自由そのものだから。**
  issue 自身がそう書いている。そして待つことで失うものは小さい
  ——`Parse` を今日使いたい人は、`internal/` の中身をコピーするか、
  v1 を待つかを選べる。公開したあとに約束を破るほうがずっと高くつく。
- **`model.File` を触る issue が同時に 3 本開いている。**
  #96 / #124 / #97 のどれも、公開後なら互換の議論になる。
  **いま公開するのは、決まっていないものを約束することである。**
- **リリースがまだ無い**（`version.Version = "dev"`、#101）。
  import できても、バージョンを指定して固定する相手がいない。

決めること 2〜4 の答え:

- **公開する日が来たら `model` ごと出す。** C の「最小の型を切り出す」は
  採らない。理由は上の測定で、**分離の代償は Go の型ではなく JSON の形**であり、
  それは `web/src/types.ts` と保存済みのセッションとエクスポート済みの
  ページに及ぶ。パーサの見た目のために、動いている 3 つの経路を
  作り直す取引にはならない。
- **決めること 3 の答え: issue の主張は、前提が事実と違うので成り立たない。**
  issue は「`finalize` が `ViewMode` と `Folded` をパーサで設定しているのが
  その証拠だ」と言うが、**`Folded` はパーサが設定していない**
  （`internal/diff/` に 1 回も現れず、`internal/server/fold.go` だけが書く）。
  残る混在は `ID` 1 つで、あとは diff の事実からの派生である。
  **したがって「公開とは独立に分離する価値がある」は、
  いまのコードに対しては当てはまらない。** すでにおおむね分かれている。
- **決めること 4 の見積もり（実装しない）:** Go 側は上の表のとおり
  10 数か所。`internal/server/fold.go` が `Folded` を持つ別の型を
  受け取るようになり、`internal/server/preview.go` と
  `internal/export/export.go` が 3 つの真偽値を自分で計算するようになる
  （`diff.IsMarkdown` などが公開されているのでそれ自体は 1 行）。
  **重いのは `web/src/types.ts` と JSON である**: `FileDiff` の 6 フィールドの
  出どころが変わり、`export.PayloadVersion` を上げる必要が出る。

**ユーザの決めが要る点:** **公開するとしたら、どの import パスで出すか。**
`github.com/tenntenn/sbnn/diff` としてこのリポジトリから出すのか、
別のリポジトリに切り出すのか。これは「sbnn というプロジェクトが
ライブラリも配るのか」という、後戻りしにくく、
リポジトリの中だけでは決まらない判断である。
**それ以外はこの文書で決まっている。**

## 後戻りしない第一歩

**いまの公開面を機械に固定させる。** 公開してもしなくても捨てずに済む。

この PR で `internal/diff/apisurface_test.go` を 1 本足した。

- `package diff`（内部テスト）。既存の `internal/diff/parse_test.go` は
  `package diff_test` なので、同居して名前がぶつからない。
- 公開識別子の一覧を `apiSurfaceExported` という文字列の表として持ち、
  `go/ast` + `go/parser` でこのディレクトリの `*.go`（`_test.go` を除く）を
  読んだ結果と突き合わせる。**外部コマンドを呼ばず、依存も足さない。**
- トップレベルの識別子はすべて `apiSurface` で始まる
  （テスト関数だけが `TestAPISurface`）。他のレーンが後で足すテストと
  ぶつからないようにするためである。
- 落ちたときのメッセージは、増えた分と減った分を並べたうえで
  「公開面を変えたなら、この表も更新すること。変えるつもりが
  無かったなら、それは意図しない公開である」と言う。

**これは公開面を「凍結」するものではない。** 変更を禁じるのではなく、
変更が意識的な行為になるようにするものである。その意図はテストの
先頭のコメントに書いた。

この表が最初に役に立った証拠が、上の「11 個であって 9 個ではない」である。

## やらないこと

- **`internal/diff` / `internal/model` を動かすこと。** この提案では公開しない。
- **`model.File` からフィールドを外すこと。** 上で価値が無いと判断した。
- **`internal/diff` の実装の変更。** この PR は既存の `.go` を 1 行も変えない。
- **`internal/diff/parse_test.go` に触ること。** 別レーンの担当である。
- **他のパッケージの公開面のテスト。** issue の言うとおり
  `export` は埋め込み UI に縛られ、`history` は独自のログ形式、
  `client` は動いている sbnn にしか使えず、`server` はアプリ本体である。
  `diff` だけが候補なので、固定するのも `diff` だけでよい。
- **`ID` の採番規則を変えること。** `model.Comment.FileID` が指している。

## 次の 1 PR の範囲

**題: `model.File` の JSON の形を、公開の判断とは切り離して固定する。**

`internal/diff` の公開面は上のテストで押さえた。**押さえられていないのは
線を渡る JSON のほうである** —— `web/src/types.ts` の `FileDiff`、
保存された `session-<port>.json`、エクスポートされたページの
`window.__SBNN_DATA__` が同じ形に依存しており、
そこを崩す変更は #96 / #124 / #97 のどれでも起きうる。

触るファイル:

- `internal/model/` に `jsonshape_test.go`（新規）。
  `model.File` `model.Hunk` `model.Line` `model.Comment` を
  `json.Marshal` し、**キーの一覧**をテストの中の表と突き合わせる。
  `apisurface_test.go` と同じ趣旨で、同じ書き方にそろえる
  （トップレベルの識別子は `jsonShape` で始め、テスト関数は
  `TestJSONShape`）。
- `web/src/types.ts` には**触らない**。

完了条件:

- 4 つの型のキーの一覧が表と一致する。`omitempty` のフィールドが
  ゼロ値のときに消えることも表に書く（`folded` `foldReason` は消える）。
- `model.Comment` の `MarshalJSON`（`internal/model/model.go:310`）が
  足す `suggestions` が一覧に含まれる。
- テストが落ちたときのメッセージが
  「`web/src/types.ts` と `export.PayloadVersion` も見直すこと」と言う。
- `go build ./... && go vet ./... && go test ./...` が通り、
  `gofmt -l` が何も出さない。

そのあとに来る PR（この 1 本には含めない）:

1. #101（リリースとバイナリ）。**公開の再検討はこれより後に置く。**
2. v1 を切るときに、この提案の「決定」を読み直す。
   そのとき決めるべき 1 点は import パスだけである。
