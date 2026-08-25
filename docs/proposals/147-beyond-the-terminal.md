# 147 — 端末以外の入口をどう並べるか

対象 issue: [#147](https://github.com/tenntenn/sbnn/issues/147)
状態: 方針（実装前）

**この文書は順序と共通の前提だけを扱う。** #147 が並べている 4 つ
（MCP・アーティファクト経路・デスクトップ・エディタ拡張 / LSP）を
ここで個別に設計しない。個別の設計はそれぞれの提案に委ねる:

- MCP は [#125](https://github.com/tenntenn/sbnn/issues/125) の提案
- デスクトップは [#105](https://github.com/tenntenn/sbnn/issues/105) の提案
- アーティファクト経路は #115 / #55

同じ波でそれらが並行して書かれている。**この文書はそれらの中身を待たない。**
ここで決めるのは「どの順で出すか」と「4 つが共通して必要としているものの正体」である。

## 決めること

1. **順序を確定する。** issue の 1〜4 の並びをそのまま採るか、変えるか。
2. **4 つが共通して要求している「安定した HTTP API」を、いつ・どこに・
   どういう約束で文書化するか。** これがこの提案の中心である。
3. **エディタ拡張と LSP のどちらを目指すか。**
4. **やらないと決めるものがあるか。** 4 つ全部をやると約束しない。

## 現状（コードを読んで確かめた事実）

**API はもう揃っている。ただし文書がどこにも無い。**

ルートは `internal/server/server.go:154`〜`178` に 1 か所でまとまっている。
`/_/api/` の下に 23 本、それに `GET /_/events` を足して**外向きは 24 本**である
（`GET /` は SPA を返す `spaHandler`）。全表:

| メソッド | パス | 本体 / 引数 | 返り |
|---|---|---|---|
| GET | `/_/api/status` | — | `Status` |
| GET | `/_/api/reviews` | クエリ（`internal/server/server.go:349`） | `ReviewsResponse` |
| GET | `/_/api/groups` | — | グループ一覧 |
| DELETE | `/_/api/groups` | — | 消した数 |
| GET | `/_/api/groups/{group}` | — | `model.Group` |
| DELETE | `/_/api/groups/{group}` | — | — |
| POST | `/_/api/groups/{group}/diffs` | `AddDiffRequest` | `AddDiffResponse` |
| DELETE | `/_/api/groups/{group}/diffs/{diff}` | — | — |
| GET | `.../files/{file}/preview` | — | mo のプレビュー |
| GET | `.../files/{file}/content` | — | `FileContentResponse` |
| GET | `.../files/{file}/image` | — | 画像そのもの |
| GET | `/_/api/groups/{group}/comments` | — | `[]*model.Comment` |
| POST | `/_/api/groups/{group}/comments` | `AddCommentRequest` | `*model.Comment` |
| PATCH | `.../comments/{id}` | `UpdateCommentRequest` | `*model.Comment` |
| DELETE | `.../comments/{id}` | — | — |
| DELETE | `/_/api/groups/{group}/comments` | `?resolved=true` | 消した数 |
| GET | `/_/api/groups/{group}/prompt` | `?resolved=` `?instruction=` | テキスト |
| POST | `/_/api/groups/{group}/review` | `SubmitReviewRequest` | `*model.Group` |
| GET | `/_/api/groups/{group}/hooks` | — | `[]*model.Hook` |
| POST | `/_/api/groups/{group}/hooks` | `model.Hook` | `*model.Hook` |
| DELETE | `/_/api/groups/{group}/hooks` | — | 消した数 |
| DELETE | `/_/api/groups/{group}/hooks/{id}` | — | 消した数 |
| POST | `/_/api/shutdown` | — | — |
| GET | `/_/events` | — | SSE |

本体の型はすべて `internal/server/server.go` に、`AddDiffRequest`（`:416`）
`AddDiffResponse`（`:429`）`AddCommentRequest`（`:574`）
`UpdateCommentRequest`（`:685`）`SubmitReviewRequest`（`:763`）
`Status`（`:296`）`ReviewsResponse`（`:341`）として定義されている。

SSE の中身は 2 種類だけである（`internal/server/server.go` の
`notifyReview` と `notify`）:

```go
json.Marshal(map[string]any{"type": "review", "group": …, "reviewedAt": …,
	"comments": …, "verdict": …})
json.Marshal(map[string]string{"type": "change", "group": group})
```

**そして `README.md` はこの 24 本を 1 本も書いていない。** README が
説明しているのは CLI だけで、`docs/` には `docs/screenshot.png` しか無い。
**issue の最後の主張「3 つとも同じもの（安定した、CLI ではない HTTP API）を
必要としている」は正しく、しかもその「同じもの」は現在 0% 存在する。**
API は動いているが、外向きの契約としては**存在していない**。

**`internal/client` はその契約の Go 版として既にある。** `Status`
`AddDiff` `Group` `AddComment` `Comments` `Prompt` `ClearComments`
`DeleteGroup` `DeleteAllGroups` `Reviews` `Shutdown` `SubmitReview`
`Hooks` `AddHook` `DeleteHooks` `WaitForReview` の 16 メソッド
（`internal/client/client.go`）。ただし `internal/` の下なので外から使えない。

**エクスポート経路の実測（#115 / #55 が「経路 + フォント 1 つ」で済むか）。**
`internal/export/export.go:121` の `Render` は
`readAssets`（`:158`）が集めたものだけを埋め込む:

```go
case strings.HasSuffix(name, ".css"):
	cssParts = append(cssParts, string(b))
case strings.HasSuffix(name, ".js"):
	jsParts = append(jsParts, string(b))
```

**`.css` と `.js` だけである。** ところが `web/src/styles.css:243` に

```css
src: url('./assets/material-symbols-outlined-subset.woff2') format('woff2');
```

があり、この相対 URL は書き換えられない。**単体の HTML として配ると
アイコンフォントが必ず落ちる。** `.icon` の要素はフォントのグリフ名
（`web/src/components/Icon.tsx` が置く文字列）をそのまま素の文字として
表示するので、見出しに意味不明の英単語が並ぶ。**#55 の「garbled header」の
正体はこれ 1 つであり、#147 の「one inlined font」という見積もりは正確である。**
直しは `readAssets` に `.woff2` を data URL として取り込む分岐を足し、
CSS の `url(...)` を置換する、実質 1 関数の変更になる。

**スキルはエクスポート経路に案内している。** `skills/sbnn/SKILL.md` の
「Sharing a review without sbnn」節に
`git diff | sbnn export --target <topic> review.html` があり、
`--fragment` がアーティファクト向けだと書いてある
（`cmd/export.go:61` の `--fragment` フラグ）。**#115 の
「スキルが案内していない」は、少なくとも現在の SKILL.md には当てはまらない。**
案内はある。壊れているのは案内先の出力である。

**コメントは本当に診断の形をしている。** `internal/model/model.go:147` の
`Comment` は `Path` `Side` `StartLine` `EndLine` `Body` を持ち、
LSP の `Diagnostic`（`range` + `message`）に素直に写る。
`Resolved` は診断を出すかどうか、`Question` は `severity` に、
`model.Suggestions(c.Body)`（`internal/model/model.go:185`）が返す置換文字列は
`CodeAction` の `WorkspaceEdit` に対応する。**issue のこの主張は正しい。**
ただし 1 つ食い違う: LSP の `Range` は**0 始まりで文字位置まで持つ**のに対し、
`Comment` は**1 始まりの行だけ**で、しかも `Side` が `"old"` のときは
**作業ツリーに存在しない行番号**である。旧側のコメントは診断にできない。

## 選択肢

順序について、2 つ並べる。

### A. issue の並びのまま（MCP → アーティファクト → デスクトップ → 拡張）

「到達できる人の数 ÷ 手間」で並べたもの。

- できるようになること: 一番効くものから出る。MCP は
  Claude Desktop とシェルの無いホストを丸ごと開ける。
- 払う代償: **1 番目が一番大きい。** MCP は `sbnn mcp` という
  新しいサブコマンド、新しい依存（MCP の SDK）、そして
  ツール説明という**3 つ目の説明面**を同時に持ち込む。
  #109〜#112 が示す「スキルと CLI の食い違い」がまだ直っていないうちに
  3 面目を足すことになる。しかも土台（文書化された API）は
  まだ無いので、MCP のツール定義がそのまま事実上の契約になってしまう。

### B. API の文書化を 0 番目に置き、アーティファクト経路を先に出す

順序を **API リファレンス → アーティファクト（#115 / #55）→ MCP（#125）→
デスクトップ（#105）→ 拡張 / LSP** にする。

- できるようになること: 4 つ全部が要求している 1 つのものが最初に来る。
  MCP のツール表もデスクトップの集約も拡張の診断も、
  同じ 1 枚の表を参照して書けるようになり、**食い違いが 3 倍になる経路が
  そもそも生まれない。** そして 2 番目が一番小さい:
  上で測ったとおり #55 は `readAssets` の 1 関数で、
  #115 は案内先が直れば済む。**手を動かした結果が最初に出るのが早い。**
- 払う代償: シェルの無いホストが使えるようになるのが 1 つ後ろにずれる。
  API リファレンスそのものはユーザに何も新しいことをさせない。

### C. 拡張 / LSP から始める

「レビューが起きる場所」を変えるのはこれだけだから、という理由。

- できるようになること: IDE の中でループが閉じる。
- 払う代償: 4 つの中で最大で、しかも上で見つけた `Side == "old"` の
  食い違いのように、まだ設計の穴がある。土台も無い。**採らない。**

## 決定

**B を採る。順序は次のとおりとする。**

0. **HTTP API リファレンスを書く**（`docs/api.md`）。上の 24 本の表に、
   本体と返りの形、エラーの返り方、そして**何を安定と約束するか**を書く。
1. **アーティファクト経路**（#55 → #115）。#55 が先。案内先が壊れている
   うちに案内を増やしても意味がない。
2. **MCP**（#125）。0 番が済んでいれば、ツール表は API 表の写像として書ける。
3. **デスクトップ**（#105）。
4. **エディタ拡張 / LSP。**

理由:

- **0 番を先に置く理由が、この issue の結論そのものだから。** issue は
  「3 つとも同じものを必要としている」で終わっている。その同じものが
  0% しか無いことを上で確かめた。**先に書かなければ、最初に来た実装の
  都合が契約になる。**
- **1 番と 2 番を入れ替える理由は大きさである。** #55 は `readAssets` の
  1 関数、#115 は文言。#125 は新しいサブコマンドと新しい依存と 3 つ目の
  説明面。**小さいほうを先に出すと、0 番の表が最初の利用者を得て、
  書きっぱなしにならない。**

**API の安定について約束すること**（0 番の中身の核）:

- **安定と約束するのは「パス・メソッド・要求本体のフィールド名・
  返りのフィールド名」まで。** 既存フィールドの意味を変えない、
  既存フィールドを消さない、パスを動かさない。
- **約束しないのは「フィールドが増えないこと」。** JSON にフィールドが
  増えるのは破壊ではない。読む側は知らないキーを無視すること、と書く。
- **`/_/api/shutdown` は契約に入れない。** これはプロセス管理であって
  レビューの API ではない。文書には載せるが「sbnn 自身のための口」と書く。
- **バージョンはパスに入れない。** `/_/api/v1/` にはしない。上の
  「増えるのは破壊ではない」規則で足りる。パスにバージョンを入れると
  2 本目を作る日が来て、そのとき本当に困る。
- **`internal/client` は公開しない。** 契約は HTTP であって Go の型ではない。
  Go から使いたい人は 24 本を叩けばよい。これは #128（`internal/diff` の
  公開）とは別の話であり、混ぜない。

**エディタ拡張と LSP は、いま決めない。** ただし**何が分かれば決まるかを決める**:
上で見つけた `Comment.Side == "old"` の問題 — 旧側のコメントは
作業ツリーの行を指していないので診断にできない — の答えが出れば決まる。
旧側コメントを「診断にしない」で済ませてよいなら LSP は素直に書けるので
LSP を採る。済ませられないなら、エディタごとに diff ビューを持つ拡張が要る。
**この 1 問は 4 番の中で最初に答えるべき問いであり、それ以外は 4 番の
設計に属する。**

**やらないと決めるもの:** **4 つ全部をやると約束しない。**
0 番と 1 番はやる。2 番と 3 番はそれぞれの提案の決定に従う。
**4 番（拡張 / LSP）は、この提案では「やる」と言わない。**
1〜3 が済んだ時点で、IDE の中の人が実際に何に困っているかを見てから決める。

## 後戻りしない第一歩

**上の 24 本の表を、この提案の中に置いたこと自体が第一歩である。**

MCP を作っても作らなくても、デスクトップを作っても作らなくても、
拡張を作っても作らなくても、この表は捨てずに済む。
`docs/api.md` はこの表を移して、各行に本体と返りの形を足したものになる。

**表が今この文書にあることの効果**: #125 の提案は
「7 つのツールが既存 API でまかなえるか」を突き合わせる相手をここに持てる。
#105 の提案は「集約が何を読めばできるか」を `GET /_/api/status` の
`Status.Groups`（`[]GroupSummary`）として指せる。
どちらも他方の提案を待たずに書ける。

## やらないこと

- **#125 / #105 / #115 / #55 の個別設計。** それぞれの提案に属する。
  この文書がそれらに対して持つ拘束は**順序だけ**である。
- **API に認証を足すこと。** `--dangerously-allow-remote-access` が
  いまの立場（ループバックだけ、認証なし）を名前で言っている。
  リモートを本気で支えるなら別 issue が要る。
- **`/_/api/v1/` のようなバージョン付きパス。** 上で採らないと決めた。
- **`internal/client` の公開。** 上で採らないと決めた。
- **CLI をやめること。** 4 つはどれも CLI の**追加**であって代替ではない。
  README の立場（「sbnn is one command among the ones you already run」）は
  変えない。

## 次の 1 PR の範囲

**題: 単体の HTML としてエクスポートしたページのアイコンを直す（#55）。**

順序では 0 番（API リファレンス）が先だが、**次の 1 PR は 1 番の #55 にする。**
0 番は複数のレーンの提案が出そろってから 1 枚にまとめたほうが食い違わないのに対し、
#55 はいま 1 関数で直り、直った瞬間に #115 の案内先が正しくなるからである。

触るファイル:

- `internal/export/export.go` — `readAssets` が `.woff2` も読み、
  返り値に「フォント名 → data URL」を足す。`Render` が CSS の
  `url('./assets/…woff2')` を data URL に置換してから埋め込む。
- `internal/export/export_test.go` — 表駆動で足す。

完了条件:

- 生成された HTML に `url(data:font/woff2;base64,` が現れる。
- 生成された HTML に `./assets/` で始まる URL が 1 つも残らない
  （`--fragment` の場合も含めて）。
- フォントが埋め込まれていない古い `dist` を与えたときに
  `Render` がエラーにならず、CSS をそのまま通す（`web/dist` は
  波ごとに再生成されるので、無いものを必須にしない）。
- `go build ./... && go vet ./... && go test ./...` が通る。

そのあとに来る PR（この 1 本には含めない）:

1. `docs/api.md` — 上の 24 本の表に本体と返りの形を足し、
   安定の約束を書く。`README.md` から 1 行で指す。
2. #115 — `skills/sbnn/SKILL.md` の案内を、モバイル / アーティファクトの
   場合に `sbnn export --fragment` へ確実に向ける。
3. #125 の提案の「次の 1 PR」。
