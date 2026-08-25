# 147 — 端末以外の入口をどう並べるか

対象 issue: [#147](https://github.com/tenntenn/sbnn/issues/147)
状態: 方針（実装前）

**この文書は順序と共通の前提だけを扱う。** #147 が並べている 4 つ
（MCP・アーティファクト経路・デスクトップ・エディタ拡張 / LSP）を
ここで個別に設計しない。個別の設計はそれぞれの提案に委ねる:

- MCP は [#125](https://github.com/tenntenn/sbnn/issues/125) の提案
  （`docs/proposals/125-mcp-server.md`、main に入っている）
- デスクトップは [#105](https://github.com/tenntenn/sbnn/issues/105) の提案
  （`docs/proposals/105-desktop-wrapper.md`、main に入っている）
- アーティファクト経路は #115（#55 は `daf7acb` で閉じている）

ここで決めるのは「どの順で出すか」と「4 つが共通して必要としているものの正体」である。

## 決めること

1. **順序を確定する。** issue の 1〜4 の並びをそのまま採るか、変えるか。
2. **4 つが共通して要求している「安定した HTTP API」を、いつ・どこに・
   どういう約束で文書化するか。** これがこの提案の中心である。
3. **エディタ拡張と LSP のどちらを目指すか。**
4. **やらないと決めるものがあるか。** 4 つ全部をやると約束しない。

## 現状（コードを読んで確かめた事実）

**API はもう揃っている。ただし文書がどこにも無い。**

ルートは `internal/server/server.go` の `handler()` に 1 か所でまとまっている。
`/_/api/` の下に 23 本、それに `GET /_/events` を足して**外向きは 24 本**である
（`GET /` は SPA を返す `spaHandler`）。全表:

| メソッド | パス | 本体 / 引数 | 返り |
|---|---|---|---|
| GET | `/_/api/status` | — | `Status` |
| GET | `/_/api/reviews` | クエリ（`handleReviews`） | `ReviewsResponse` |
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

本体の型はすべて `internal/server/server.go` に、`AddDiffRequest`
`AddDiffResponse` `AddCommentRequest` `UpdateCommentRequest`
`SubmitReviewRequest` `Status` `ReviewsResponse` として定義されている。
（行番号は波ごとに動くので、この文書はどこも関数名・型名で指す。）

SSE の中身は 2 種類だけである（`internal/server/server.go` の
`notifyReview` と `notify`）:

```go
json.Marshal(map[string]any{"type": "review", "group": …, "reviewedAt": …,
	"comments": …, "verdict": …})
json.Marshal(map[string]string{"type": "change", "group": group})
```

**そして `README.md` はこの 24 本を 1 本も書いていない**（`grep -c "_/api" README.md`
→ 0）。README が説明しているのは CLI だけである。`docs/` にあるのは
`docs/screenshot.png`、README の主張をバイナリと突き合わせる `docs/doccheck`、
そして提案文書を置く `docs/proposals/` で、**API の文書は 1 本も無い**
（`ls docs/api.md` → No such file or directory）。
**issue の最後の主張「3 つとも同じもの（安定した、CLI ではない HTTP API）を
必要としている」は正しく、しかもその「同じもの」は現在 0% 存在する。**
API は動いているが、外向きの契約としては**存在していない**。

**`internal/client` はその契約の Go 版として既にある。** `Status`
`AddDiff` `Group` `AddComment` `Comments` `Prompt` `ClearComments`
`DeleteGroup` `DeleteAllGroups` `Reviews` `Shutdown` `SubmitReview`
`Hooks` `AddHook` `DeleteHooks` `DeleteHook` `WaitForReview` の 17 メソッド
（`internal/client/client.go`。`BaseURL` / `url` / `do` を除いた数。
`DeleteHook` は `sbnn hook --remove` と一緒に増えた）。
ただし `internal/` の下なので外から使えない。

**エクスポート経路の実測（#55 は済んでいる）。** この文書の初稿は
「単体の HTML にするとアイコンフォントが必ず落ちるので、`readAssets` に
`.woff2` の分岐を足すのが次の 1 PR だ」と書いていた。**その前提は
`daf7acb`（#298, closes #55）で消えている。** #298 が触ったのは Go ではなく
web で、`internal/export` は 1 行も変わっていない。

- `web/src/components/Icon.tsx` が `document.fonts` を見るようになった。
  フォントが取れないときはグリフ名を素の文字として出さず、幅 1em の空箱に
  畳む。「見出しに意味不明の英単語が並ぶ」症状はこれで出なくなった。
- `web/vite.config.ts` に

  ```ts
  assetsInlineLimit: (filePath: string) =>
    filePath.endsWith('.woff2') ? true : undefined,
  ```

  が入り、**`.woff2` はビルド時に CSS へ data URL として内包される。**
  `web/src/styles.css` のソースは `url('./assets/…woff2')` のままだが、
  `readAssets` が読むのは `web/dist` の CSS なので結果に影響しない。

実測（Go は 1 行も触らず、現 main で `web/dist` を作り直しただけ）:

```console
$ cd web && pnpm install --frozen-lockfile --offline && pnpm run build
dist/assets/index-mi7SVdIL.css  374.95 kB
$ grep -o "url(data:font/woff2;base64,[A-Za-z0-9+/]\{0,20\}" web/dist/assets/*.css
url(data:font/woff2;base64,d09GMgABAAAAA/kMAA0A
$ grep -c "url(/assets/[^)]*woff2[^)]*)" web/dist/assets/*.css
0
$ go build -o /tmp/sbnn . && git diff | /tmp/sbnn export /tmp/out.html
$ grep -c "url(data:font/woff2;base64," /tmp/out.html
1
$ grep -o "/assets/[^\"')]*" /tmp/out.html | wc -l
0
$ git diff | /tmp/sbnn export --fragment | grep -o "/assets/[^\"')]*" | wc -l
0
```

**初稿が「次の 1 PR」の完了条件として挙げた 2 つ**
（`url(data:font/woff2;base64,` が現れる / `assets/` 参照が 1 つも残らない）
**は、現 main で `web/dist` を再生成するだけで満たされる。**
`readAssets` に `.woff2` の分岐を足す作業はもう無い。

残っているのは Go の問題ではなく、コミット済み `web/dist` が古いことだけである。
`web/dist` は波ごとにまとめて再生成される運用なので、いま木にあるビルドは
#298 より前のものである。そのままの木から建てたバイナリで測ると:

```console
$ go build -o /tmp/sbnn-stale . && git diff | /tmp/sbnn-stale export /tmp/stale.html
$ grep -o "/assets/[^\"')]*" /tmp/stale.html | sort -u
/assets/material-symbols-outlined-subset-CC1o3iId.woff2
$ grep -c "url(data:font/woff2;base64," /tmp/stale.html
0
```

つまり**いまリリースすればまだフォントは落ちる**が、落ちても `Icon.tsx` が
空箱に畳むので #55 の見た目の症状は出ないし、直すのに要るのは
`task web` と `web/dist` のコミットであって、この文書が扱うような設計の
PR ではない。**アーティファクト経路で残っている設計の作業は #115 だけである。**

**スキルはエクスポート経路に案内している。** `skills/sbnn/SKILL.md` の
「Sharing a review without sbnn」節に
`git diff | sbnn export --target <topic> review.html` があり、
`--fragment` がアーティファクト向けだと書いてある
（`cmd/export.go` の `--fragment` フラグ）。**ただし #115 が問題にしているのは
「案内が無いこと」ではない。** #115 は、到達できるかどうかを確かめないまま
localhost の URL を人に渡すことを問題にしている。現 main の step 3 は今も

```
### 3. Hand the URL to the human, and decide how you come back

Tell the user the URL sbnn printed and say what you want reviewed.
```

であり、電話やチャットの向こうの人に `http://localhost:6280/` を渡させる。
**初稿の「壊れているのは案内先の出力である」というまとめは二重に外れていた。**
出力はもう壊れておらず、壊れているのは案内の分岐のほうである。

**コメントは本当に診断の形をしている。** `internal/model` の
`Comment` は `Path` `Side` `StartLine` `EndLine` `Body` を持ち、
LSP の `Diagnostic`（`range` + `message`）に素直に写る。
`Resolved` は診断を出すかどうか、`Question` は `severity` に、
`model.Suggestions(c.Body)` が返す置換文字列は
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
  上で測ったとおり #55 は #298 で済んでおり、アーティファクト経路に
  残っているのは #115 の文言だけである。**手を動かした結果が最初に出るのが早い。**
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
1. **アーティファクト経路**（#115）。#55 は `daf7acb`（#298）で閉じており、
   出力はもう壊れていない。残るのは、到達できない URL を渡さないよう
   スキルの案内を分岐させることだけである。
2. **MCP**（#125）。0 番が済んでいれば、ツール表は API 表の写像として書ける。
3. **デスクトップ**（#105）。
4. **エディタ拡張 / LSP。**

理由:

- **0 番を先に置く理由が、この issue の結論そのものだから。** issue は
  「3 つとも同じものを必要としている」で終わっている。その同じものが
  0% しか無いことを上で確かめた。**先に書かなければ、最初に来た実装の
  都合が契約になる。**
- **1 番と 2 番を入れ替える理由は大きさである。** #55 は #298 で終わり、
  #115 に残っているのは文言だけである。#125 は新しいサブコマンドと
  新しい依存と 3 つ目の説明面。**小さいほうを先に出すと、0 番の表が最初の利用者を得て、
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
  公開、`docs/proposals/128-export-diff-package.md`）とは別の話であり、混ぜない。

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

**表が今この文書にあることの効果**は既に出ている。main に入った
`docs/proposals/125-mcp-server.md` は 7 つのツールを 1 本ずつ `/_/api/...` に
対応づけた表を持ち、`docs/proposals/105-desktop-wrapper.md` は集約を
`GET /_/api/status` の返りとして指している。**どちらも他方の提案を
待たずに書けた。** 残っているのは、その参照先を提案文書ではなく
`docs/api.md` にすることだけである。

## やらないこと

- **#125 / #105 / #115 の個別設計。** それぞれの提案に属する。
  この文書がそれらに対して持つ拘束は**順序だけ**である。
  （#55 は `daf7acb` で閉じたので、もうこの並びに入らない。）
- **API に認証を足すこと。** `--dangerously-allow-remote-access` が
  いまの立場（ループバックだけ、認証なし）を名前で言っている。
  リモートを本気で支えるなら別 issue が要る。
- **`/_/api/v1/` のようなバージョン付きパス。** 上で採らないと決めた。
- **`internal/client` の公開。** 上で採らないと決めた。
- **CLI をやめること。** 4 つはどれも CLI の**追加**であって代替ではない。
  README の立場（「sbnn is one command among the ones you already run」）は
  変えない。

## 次の 1 PR の範囲

**題: HTTP API リファレンスを書く（`docs/api.md`）。**

初稿はここに「単体の HTML としてエクスポートしたページのアイコンを直す（#55）」を
置いていた。**その仕事はもう無い。** `daf7acb`（#298）が `Icon.tsx` と
`vite.config.ts` で閉じており、上の実測のとおり `web/dist` を再生成した現 main の
バイナリの `sbnn export` 出力には `/assets/` 参照が 0 件、
`url(data:font/woff2;base64,` が 1 件ある。初稿が挙げた完了条件は
既に満たされているので、そのまま書けば `readAssets` に要らない分岐を足す作業になる。

初稿が 0 番を後回しにした理由（「複数のレーンの提案が出そろってから
1 枚にまとめたほうが食い違わない」）も、もう成り立たない。
`docs/proposals/125-mcp-server.md` と `docs/proposals/105-desktop-wrapper.md` は
どちらも main に入っており、**どちらも既に `/_/api/...` を名指しで参照している**:

```console
$ grep -c "_/api" docs/proposals/125-mcp-server.md docs/proposals/105-desktop-wrapper.md
docs/proposals/125-mcp-server.md:8
docs/proposals/105-desktop-wrapper.md:3
```

`105-desktop-wrapper.md` は「4 つが共通して必要とする HTTP API の文書化は
#147 の担当なので」と書いて、この文書に投げてきている。**待つ相手はもういない。**

触るファイル:

- `docs/api.md`（新規）— 上の 24 本の表に、要求本体と返りの形、
  エラーの返り方、そして「何を安定と約束するか」（この文書の
  「API の安定について約束すること」をそのまま規約として書く）を足す。
- `README.md` — Development 節あたりから 1 行で指す。
- `docs/doccheck` — README の主張をバイナリと突き合わせる既存のテストと
  同じやり方で、`docs/api.md` に書いたパスとメソッドが
  `internal/server` の `handler()` の登録と一致することを確かめる。
  **表を手で書き写した瞬間からずれ始めるので、これは同じ PR に入れる。**

完了条件（いずれも現 main では未達であることを実測済み）:

- `ls docs/api.md` が通る（現状: No such file or directory）。
- `grep -c "_/api" README.md` が 0 より大きい（現状: 0）。
- `handler()` に登録されている 23 本の `/_/api/...` と `GET /_/events` が
  `docs/api.md` に 1 本残らず載っている。載っていない行、あるいは
  実在しない行があればテストが落ちる。
- `go build ./... && go vet ./... && go test ./...` が通る。

そのあとに来る PR（この 1 本には含めない）:

1. #115 — `skills/sbnn/SKILL.md` の step 3 を、URL が到達できるかどうかで
   分岐させる。到達できないときは `sbnn export` で人が開けるページを渡す。
2. `web/dist` の再生成（#298 のビルド結果を木に入れる）。設計の判断は
   要らないので、波ごとの再生成に任せてよい。
3. #125 の提案の「次の 1 PR」。
