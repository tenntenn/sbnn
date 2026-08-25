# 105 — デスクトップアプリで包む案

対象 issue: [#105](https://github.com/tenntenn/sbnn/issues/105)
状態: 方針（実装前）

この文書は方針だけを決める。実装は含まない。**ラッパーの実装は始めない。**

#147（端末を前提にしない）が 4 つの入口の**順序**を扱っており、
そこでデスクトップは 3 番目に置かれている。**この提案は 3 番目の中身、
すなわち「デスクトップアプリを作るとしたら何を先に決めるか」だけを扱う。**
順序そのものと、4 つが共通して必要とする HTTP API の文書化は #147 の担当なので、
ここでは決めない。逆に、issue #105 が挙げている問題のうち
**ポートの分断は、デスクトップアプリの話ではない**というのがこの提案の結論であり、
その部分はここで決める。

## 決めること

1. **アプリがサーバを内蔵するのか、常駐サーバに接続するのか。**
2. **`--port` による分断を、デスクトップアプリより先に CLI 側で解けないか。**
   `sbnn ls` 相当の列挙は、アプリが無くても単体で価値があるか。
3. **リリースの重さ**（#101 でバイナリ配布がまだ無いこと）を踏まえた順序。
4. **いまデスクトップアプリを作るのか、作らないのか。**

Wails / Tauri の選択は**この提案では決めない。** 内蔵か接続かが決まる前に
道具を選ぶと、道具の都合でその決定が動く。決めないことをここで明示する。

## 現状（コードを読んで確かめた事実）

**ポートが 3 つあるのは事実である。**

- sbnn 本体: `cmd/root.go:25` の `DefaultPort = 6280`、`--port` / `-p`
  （`cmd/root.go:165`）。宛先は `addr()`（`cmd/root.go:198`）が
  `net.JoinHostPort(bind, port)` で組む。
- mo: `internal/mo/mo.go:26` の `DefaultPort = 6275`、`--mo-port`
  （`cmd/root.go:184`）。
- プレビュープロキシ: `internal/server/proxy.go:38` が
  `"http://" + ln.Addr().String()` を持つ。**起動時に取ったリスナの
  アドレスなので、ポートは毎回変わる。** `Status.MoProxyURL`
  （`internal/server/server.go:303`）として外には出ている。

**`--port` がセッションを分けるのも事実である。** `runServer`
（`cmd/server.go:42`）の最初の 3 行:

```go
sessionFile, err := paths.SessionFile(port)
```

`paths.SessionFile`（`internal/paths/paths.go:43`）は:

```go
return filepath.Join(dir, fmt.Sprintf("session-%d.json", port)), nil
```

コメントも「Servers on different ports keep independent sessions」と
はっきり書いている。**issue の主張はここまで正しい。**

**ただし「二つの互いに素な宇宙」は言い過ぎである。** 同じ
`internal/paths/paths.go:54` の `HistoryFile` は:

```go
// HistoryFile returns the log of submitted reviews. It is one file for the
// whole machine on purpose: the point of keeping reviews is to read them
// together, long after the servers that recorded them are gone.
return filepath.Join(dir, "reviews.jsonl"), nil
```

**提出済みのレビューは、ポートに関係なく 1 本のログに集まっている。**
`sbnn reviews` はどのポートで取られたレビューも読める。
分断されているのは「進行中のレビュー」だけで、
**終わったレビューはすでに横断できている。** これは実装の抜けではなく
書かれた意図であり、`sbnn ls` を作るときに従うべき先例でもある。

**`--status` が 1 ポートしか見ないのも事実である。** `runStatus`
（`cmd/root.go:307`）は `client.New(addr(), …)` を作って
`c.Status(ctx)` を 1 回叩くだけで、繋がらなければ
`"%s  not running\n"` と出して終わる。他のポートは見ない。

**しかし、列挙の材料はすでにディスクにある。** セッションファイルは
`paths.StateDir()`（`internal/paths/paths.go:16`）の下に
`session-<port>.json` という名前で並ぶ。**ポートは名前から読める。**
つまり列挙のためにポート範囲を走査する必要はない:
**状態ディレクトリを読み、名前からポートを取り、それぞれに
`GET /_/api/status` を 1 回投げればよい。**

**1 サーバの中身は `status` が全部返す。** `Status`
（`internal/server/server.go:296`）は `URL` `PID` `Version` `Revision`
`MoURL` `MoAvailable` `MoProxyURL` `SessionError` と
`Groups []GroupSummary` を持つ。`GroupSummary`
（`internal/server/store.go` の `Summary` の直前）は
`Name` `URL` `Diffs` `Files` `Comments` `Unresolved` `ReviewedAt`
`Reviewed` `Hooks` を持つ。**「あなたは 6280 と 6281 にサーバを持っていて、
中身はこれです」に必要なものは、1 フィールドも足さずに揃っている。**

**サーバの起動の仕方。** `ensureServer`（`cmd/root.go:291`）が
500ms 探して、いなければ `spawnServer`（`cmd/server.go:88`）が
**自分自身のバイナリを `--foreground` で起動して `Release()` で切り離す。**
つまり常駐サーバはすでに「どの端末からでも同じものに繋がる」形になっている。

**ページのタイトルとポートの表示。** `internal/export/export.go:140` は
エクスポートしたページに `<title>` を書くが、生きている画面のほうは
`web/index.html` の固定のタイトルで、`web/src/App.tsx` は
`document.title` を触っていない。**issue の「title is always sbnn」は
現時点で正しい**（#95）。

## 選択肢

### A. アプリがサーバを内蔵する（1 プロセス、ローカルにはポートが無い）

Wails / Tauri の中で `internal/server` を直接起動し、
WebView をその上に向ける。

- できるようになること: ポートがユーザから完全に隠れる。
  起動が 1 つで済む。
- 払う代償: **端末からの `git diff | sbnn` が届かない。** CLI は
  HTTP でサーバを探すので、アプリの中のサーバに繋ぐには結局
  ポートを晒すことになり、隠した意味が消える。
  さらに `spawnServer` の経路とアプリの経路の 2 通りの起動が生まれ、
  「どちらが本物か」を決める仕組み（ロック、優先順位）が要る。
  **いまの常駐モデルを壊す。**

### B. アプリは常駐サーバに接続する（issue 自身が「小さい」と言っている方）

アプリは WebView + 集約だけを持ち、サーバは今までどおり
`spawnServer` が立てる。

- できるようになること: CLI の経路が 1 バイトも変わらない。
  アプリはただの 2 つ目のフロントエンドになる。
  アプリを閉じてもレビューは残る。
- 払う代償: アプリが起動していないときに何が起きるかを決める必要がある。
  ポートは「見せない」だけで、無くなりはしない。

### C. デスクトップアプリを作らず、`sbnn ls` を CLI に足す

issue が挙げている 4 つの困りごとのうち、
**ポートの分断と「何が走っているか分からない」を CLI で解く。**
ウィンドウ・トレイ・通知はやらない。

- できるようになること: **端末の利用者にも効く**（issue 自身が
  「fixes the port confusion for everyone, terminal users included」と
  書いている）。バイナリ配布（#101）を待たずに出せる。
  アプリを作る日が来ても、アプリはこの列挙を消費するだけになる。
  上で見たとおり、材料はすでに全部ある。
- 払う代償: 「レビューがブラウザのタブより長生きする」問題は残る。
  トレイからの呼び戻しも通知も無い。

## 決定

**C を採る。いまデスクトップアプリを作らない。代わりに `sbnn ls` を先に出す。**

そして**アプリを作る日が来たときは B（常駐サーバに接続する）を採る**と
ここで決めておく。A は採らない。

理由:

- **#105 が挙げた 4 つの困りごとのうち、3 つはアプリを必要としない。**
  「3 つのポートが見えない」「`--port` が世界を分ける」
  「走っているものを列挙できない」は、どれも列挙とラベルの問題である。
  アプリでしか解けないのは 4 つ目（ブラウザのタブより長生きすること）
  だけで、これは 4 つの中で一番小さい困りごとである。
- **#101 が先にある。** バイナリ配布がまだ無い。デスクトップの
  ターゲットを足すのは、配布する仕組みができてからでないと、
  「配れないものを 2 つ」持つことになる。issue 自身が
  「should come after that, not instead of it」と書いている。
- **A を採らない理由は端末である。** `git diff | sbnn` が
  どの端末からでも同じサーバに届くことは sbnn の中心にある性質で、
  `spawnServer` の切り離し（`cmd/server.go:138` の `Release()`）が
  それを支えている。内蔵はここを壊す。
- **`sbnn ls` はアプリを作っても作らなくても要る。** アプリの集約は
  結局「どのサーバがあり、中に何があるか」を知る必要があり、
  それは `sbnn ls` が答えるものと同じである。

**ユーザの決めが要る点:** **停止したサーバのセッションファイルを
`sbnn ls` がどう扱うか。** 状態ディレクトリには、もう走っていない
ポートの `session-<port>.json` が残る。これを
(a) 「走っていない」として一覧に出す、
(b) 出さない、
(c) 出したうえで消す手段を添える、のどれにするかは
「古いレビューは資産か、ゴミか」という持ち主の判断である。
**この提案の既定は (a)**（出すが、消さない。sbnn は
ユーザが送ったものを勝手に捨てない）。**それ以外はここで決まっている。**

## 後戻りしない第一歩

**`sbnn ls` の出力を、例つきで固定する。**

```
$ sbnn ls
http://localhost:6280  running (pid 41207, sbnn dev)
  api               2 diff(s), 7 file(s), 3 comment(s), 1 open   reviewed
  hotfix            1 diff(s), 2 file(s), 0 comment(s), 0 open
http://localhost:6281  not running (session kept)
  refactor          1 diff(s), 9 file(s), 5 comment(s), 5 open
```

決めていること:

- **列挙の材料は状態ディレクトリのファイル名である。** ポート範囲を
  走査しない。`paths.StateDir()` の `session-<port>.json` から
  ポートを読み、それぞれに `GET /_/api/status` を投げる。
  **走査しない理由**: 走査は他人のプロセスを叩くことであり、
  sbnn が自分の残した名前だけを読むほうが正直で速い。
- **走っているサーバの行は `GET /_/api/status` の返りから作る。**
  `Status.URL` `Status.PID` `Status.Version`、そして各グループは
  `GroupSummary` の `Name` `Diffs` `Files` `Comments` `Unresolved`
  `Reviewed`。**`runStatus`（`cmd/root.go:307`）の 1 サーバ分の出力と
  同じ書式にそろえる。**
- **走っていないポートは、セッションファイルを読んで中身を出す。**
  「not running (session kept)」と言う。ここは HTTP を使わない。
- **`--json` を付ける。** 中身は `[{"url":…, "running":true, "pid":…,
  "groups":[…]}]`。デスクトップアプリが将来消費するのはこれである。
- `sbnn --status` は**変えない。** あれは「このポートはどうか」に
  答えるもので、`ls` は「どれがあるか」に答えるものである。

この出力が決まっていれば、A / B / C のどれに転んでも捨てずに済む。
アプリを内蔵型にしたとしても、「何が走っているか」は同じ形で要る。

## やらないこと

- **Wails / Tauri の選択。** 上で明示的に決めないと書いた。
  内蔵か接続かが決まってから選ぶ。この提案は B を選んだので、
  選ぶ日が来たら「WebView を出せて Go のバイナリに同梱できるもの」が
  条件になる、というところまでで止める。
- **トレイ / ドック常駐、OS の通知、ウィンドウのタイトル。**
  タイトルは #95 の担当であり、そちらはアプリと無関係に直る。
- **サーバの集約そのもの**（複数サーバのグループを 1 つの一覧に混ぜること）。
  `sbnn ls` は**サーバごとに分けて**出す。混ぜるのはアプリの仕事であり、
  混ぜた瞬間に「同じ名前のグループが 2 つのポートにある」問題が生まれる。
  それはアプリを作ると決めた日に決める。
- **`--port` の既定を変えること、あるいはセッションファイルを 1 本にすること。**
  ポートごとに分けるのは書かれた意図である
  （`internal/paths/paths.go:42` のコメント）。**それを変えるのは
  この issue の範囲ではなく、変えれば走っている sbnn のセッションが失われる。**
- **リモート（`--dangerously-allow-remote-access`）の列挙。**
  `sbnn ls` はローカルの状態ディレクトリだけを読む。

## 次の 1 PR の範囲

**題: `sbnn ls` で、走っているサーバとその中身を列挙する。**

触るファイル:

- `cmd/` に `ls.go`（新規）— `lsCmd` を定義し、`cmd/root.go:195` の
  `AddCommand` に足す（**`cmd/root.go` はこの 1 行だけ**）。
- `internal/paths/paths.go` — 状態ディレクトリの `session-*.json` を
  列挙してポートを返す関数を 1 つ足す。
  例: `func Sessions() ([]int, error)`。
- `cmd/` に `ls_test.go`（新規）、`internal/paths/` に `paths_test.go`（新規）。

完了条件:

- `Sessions()` が `session-6280.json` からポート 6280 を返し、
  `session-.json` / `session-abc.json` / `reviews.jsonl` /
  `session-6280.json.bak` を**無視する**（`setAside` が
  `internal/server/store.go:116` で作る退避ファイルを拾わないこと）。
  表駆動でテストする。
- サーバが 1 つも無いとき `sbnn ls` は**エラーにならず**、
  「no sbnn server and no kept session」を出して 0 で終わる。
- 走っているサーバ 1 つと、走っていないセッション 1 つが混在する場合の
  出力を、テストで文字列として固定する。
- `--json` の出力が上の形であることをテストする。
- `go build ./... && go vet ./... && go test ./...` が通り、
  `gofmt -l` が何も出さない。

そのあとに来る PR（この 1 本には含めない）:

1. `sbnn ls` を `skills/sbnn/SKILL.md` と `README.md` に載せる。
2. #101（バイナリ配布）。**デスクトップの検討はこれより後に置く。**
