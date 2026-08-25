slug: srv-core

# 指示文 SRV-04 — サーバ本体の応答・配信・寿命（6 issue）

- 優先度: **P1**（#20, #155） / **P2**（#133, #151, #152, #153）
- 期限: P1 の 2 件は 2026-08-26 中。残り 4 件は 2026-08-27 中。
- グループ: G7

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` — 手順・検証・コミット・PR の書式はすべてここに従う。
  この指示文はそれを上書きしない。食い違ったら COMMON.md が優先。
- `/home/user/briefs/TASKIDS.tsv` — issue とタスク ID の対応。

## 名前

```
worktree = /home/user/wt/srv-core     # slug から機械的に導出する。他の場所に worktree 名は書かない
branch   = gogo/issue-<N>             # COMMON.md のとおり、issue ごとに origin/main から切り直す
```

worktree が無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/srv-core origin/main
```

## 着手条件（**これを満たすまで 1 バイトも編集しない**）

`internal/server/server.go` は同時に 1 レーンしか持てない。順番は
**G1 store レーン → srv-api（SRV-01）→ srv-core（あなた）** で確定している。

- **メインから「srv-api の 7 本の PR が全部作成済みになった」と伝えられるまで、編集を始めない。**
  自分でその判定をしない。srv-api の worktree やブランチを見に行かない。
- 待っている間にやってよいこと（読み取りのみ・コミットしない）:
  下の 6 件の issue 本文を `mcp__github__issue_read` で全部読む、
  `/home/user/sbnn/internal/server/server.go` と `server_test.go` を読む、
  `origin/main` にマージ済みの srv-api の変更を `git log` で確認する。
- 待っている間にやってはいけないこと: 編集・ブランチ作成・コミット・push。

**着手時にやること**: `git fetch -q origin main` してから各ブランチを切る。
srv-api の変更が `main` に入っているなら、それを土台にする。まだ入っていなくても
COMMON.md のとおり `origin/main` から切る。**srv-api のブランチからは切らない。**

## 触ってよいファイル（**この 2 本だけ**）

```
internal/server/server.go
internal/server/server_test.go
```

**この 2 本から出ない。** 次は他のレーンが使用中なので 1 行も触らない:

| ファイル | 使用中のレーン |
|---|---|
| `internal/server/store.go` | G1 store |
| `internal/server/hook.go` | srv-hook（SRV-02） |
| `internal/server/preview.go` `spa.go` | srv-preview（SRV-03） |
| `internal/server/prompt.go` | export-pkg |
| `internal/server/proxy.go` | mo-proxy |
| `cmd/server.go` | cmd-serve（SRV-05） |
| `cmd/root.go` | cmd-* レーンが使用中。**フラグの追加はここでは行わない** |
| `internal/model/model.go` | model レーン（G1） |
| `web/` 配下すべて | G2〜G5 の web レーン |

**重要**: issue が `store.go` の関数（`Store.Group` の `clone` など）や
`cmd/root.go` のフラグ（`--verbose` など）を直せと読める場合、**それはこの PR の範囲外**である。
`server.go` の中で閉じる形に落として直し、残りを PR 本文と報告に書く。

## 作業の順番（**この順で 1 件ずつ、1 issue = 1 PR**）

### 1. #20 — `--dangerously-allow-remote-access` で web UI が読み取り専用になる（task `t-0c21f9`, P1）

`Server.ownOrigin` は `Origin` のホストが `s.opts.Bind` かループバックか `localhost`
のときしか通さない。`--bind 0.0.0.0` で待ち受けていると、ブラウザが送ってくるのは
利用者が実際に打った `http://192.168.1.5:6280` なので、どれにも一致せず
`crossOrigin` が弾く。フラグは「動くリモートのレビューページ」を宣伝しておいて、
実際にはコメントもレビュー送信もクローズも全部 403 になる。

やること:

- `crossOrigin` で `Sec-Fetch-Site: same-origin` を**信用して早期に通す**。
  いまはこの分岐が `Origin` の検査へ落ちてしまっている。ブラウザが本物のページ生成元を
  基準に計算する値なので、これを信じてよい。
- **それに加えて**、`ownOrigin` の比較対象に `s.opts.Bind` ではなく
  **リクエスト自身の `Host` ヘッダ**を含める。`Sec-Fetch-Site` を送らない
  クライアントでも通るようにするため。既存のループバック／`localhost` の許可は残す。
- `Sec-Fetch-Site: cross-site` / `same-site` は**従来どおり弾く**。ここを緩めない。

### 2. #155 — サイズ上限ちょうどの diff が "invalid request: unexpected EOF" で落ちる（task `t-f26e01`, P1）

`io.LimitReader` は打ち切ったことを報告しないので、32MB を超える本文は JSON の
途中で切られ、利用者は「あなたのリクエストは壊れています」と言われる。

やること:

- `server.go` の 5 か所（diff 投稿の 32MB、`handleAddComment` /
  `handleUpdateComment` / `handleSubmitReview` / `handleAddHook` の 1MB）を
  `io.LimitReader` から **`http.MaxBytesReader`** へ変える。
- `*http.MaxBytesError` を `errors.As` で判別し、**413** と、
  上限を名指しするメッセージを返す（例: `the diff is too large (max 32MB)`）。
  上限以外の decode エラーは従来どおり 400。
- 定数は `server` パッケージから**エクスポートして**おく（例: `MaxDiffSize`）。
  `cmd` 側と 1 本にまとめるのは `cmd/root.go` の担当なので**ここではやらない**。
  PR 本文に `Sharing the constant with cmd is left to cmd/root.go (owned by another lane).` と書く。
  この PR には **`Fixes #155` を書いてよい**（サーバ側の誤ったメッセージは直り切るため）。

### 3. #151 — イベントストリームに購読数の上限が無く、遅い購読者はメッセージを落とす（task `t-81e4e9`, P2）

やること:

- `broker` に**同時購読数の上限**を設ける（既定 64。定数で置く）。
  `handleEvents` は上限を超えたら購読せず **503** を返す。
  いまは `/_/events` が `GET` なので `crossOrigin` が意図的に守っておらず、
  利用者が開いた任意のページが無制限に `EventSource` を張れる。
  読めはしないが、goroutine とチャネルと 25 秒ティッカーは積み上がる。
- **レビュー通知にだけ配信保証を付ける。** グループごとに「最後のレビューイベント」を
  保持し、購読時に**再送する**（SSE の `id:` と `Last-Event-ID` の形に載せる）。
  変更通知（change）には要らない。次のイベントで取り戻せるため。
  レビュー通知だけは、取りこぼすと `sbnn wait` が
  **すでに終わったレビューを永遠に待つ**ので、ここだけ落としてはいけない。
- `retry:` フィールドを出して再接続間隔をサーバ側から指定する。
- `broker.publish` の `default:` による取りこぼしは、change については**そのまま残す**。
  全部を保証しようとしてバッファを増やすのではなく、**保証が要るものだけ再送で担保する。**

### 4. #133 — グループの取得が毎回レビュー全体を再直列化する（task `t-f31ad7`, P2）

計測値は 500 ファイルで 6.61MB / 200 ファイルで 49ms/req。しかも web は
`change` のたびに `reload()` するので、コメントを 3 つ打つと 3 回全部が流れる。

**この issue は担当ファイルの中だけでは全部は直せない。**

| 求められていること | どこにある | この PR で |
|---|---|---|
| `Raw` をグループ応答から落とす | `handleGroup`（`server.go`） | **やる** |
| `clone` の JSON 往復をやめる | `Store.Group`（`store.go`） | **やらない**（G1 store が使用中） |
| 差分更新のイベントを配る | `server.go` + `web/src` | **やらない**（web は G4 が使用中） |

やること: `handleGroup` が返す直前に、各 `model.Diff` の `Raw` を空にする
（**ストアの中身は変えない**。返す用のコピーだけを落とす。計測で 6.61MB のうち 1.00MB）。
`export.Build` がすでに同じ理由で落としているので、方針は既存と一致する。

**PR 本文には `Fixes #133` を書かない。`Refs #133` を書く。**
上の表の「やらない」2 行を英語で `## What changed` の末尾に書く。

### 5. #152 — サーバがほとんど何もログに残さないので、背後の不調が診断できない（task `t-46994b`, P2）

**この issue は担当ファイルの中だけでは全部は直せない。**

| 求められていること | どこにある | この PR で |
|---|---|---|
| リクエストログ（method / path / status / duration） | `server.go` | **やる** |
| 出力レベルの切り替え | `SBNN_LOG` 環境変数（`server.go`） | **やる** |
| `--verbose` フラグ | `cmd/root.go` | **やらない**（他レーンが使用中） |
| `persist` の失敗を warn に出す | `store.go` | **やらない**（G1 store が使用中） |
| ログの場所を `sbnn --status` で表示 | `cmd/root.go` | **やらない**（同上） |
| ログのローテーション | 範囲外 | **やらない**（別 issue に切り出す価値があると報告に書く） |

やること:

- `server.go` の mux をラップするミドルウェアを足し、
  **リクエストごとに 1 行**（method / path / status / duration）を info で出す。
  既定は**静か**にする。`SBNN_LOG=debug|info|warn|error` で切り替え、
  未設定なら `warn`（＝リクエストログは出ない）。
- `SBNN_LOG` の読み取りは `server.go` の中で完結させる。**フラグを足さない。**
- `/_/events` のような長時間つながる経路で、duration が接続終了まで出ないのは構わない
  （終了時に 1 行出る）。**判断に迷って止まらない。**

**PR 本文には `Fixes #152` を書かない。`Refs #152` を書く。**

### 6. #153 — 背後のサーバが自分では止まらない（task `t-4202c6`, P2）

3 か月前のレビュー 1 回が、ポートとセッションファイルとパース済み diff を
再起動まで抱え続ける。

**この issue は担当ファイルの中だけでは全部は直せない。**

| 求められていること | どこにある | この PR で |
|---|---|---|
| アイドルタイムアウトで終了する | `Server.Run`（`server.go`） | **やる** |
| `sbnn --status` が全サーバを報告 | `cmd/root.go` | **やらない**（他レーンが使用中） |
| `sbnn --shutdown --all` | `cmd/root.go` | **やらない**（同上） |
| README の "Starting and stopping" に書く | `README.md` | **やらない**（G9 doc-readme が使用中） |

やること:

- `Server.Run` に**保守的なアイドルタイムアウト**を入れる。
  条件は issue が指定しているとおり **「diff を 1 つも持っていない」**にする
  （「無活動」ではない）。**人を待っているレビューを絶対に回収しないため。**
  さらに、つながっているイベントストリームが 0 本、実行中のフックが 0 件を条件に加える。
- 既定は **30 分**。`SBNN_IDLE_TIMEOUT`（Go の `time.ParseDuration` 形式）で上書き可能、
  `0` で無効。**フラグは足さない。**
- 終了するときは、その理由を warn で 1 行ログに残す
  （利用者が「勝手に消えた」と思わないようにするため）。

**PR 本文には `Fixes #153` を書かない。`Refs #153` を書く。**

## テスト（必須）

`internal/server/server_test.go` の既存のテーブル駆動テストの書き方に合わせる。
**6 件それぞれに、修正前に落ちて修正後に通る回帰テストを 1 つ以上足す。**

- #20: `Origin: http://192.168.1.5:6280` + `Sec-Fetch-Site: same-origin` で
  **403 にならない**こと。`Sec-Fetch-Site: cross-site` は**403 のまま**であること。
- #155: 上限を超える本文で **413** と、メッセージに `32MB` / `1MB` が含まれること。
  上限内の壊れた JSON は **400** のままであること。
- #151: 上限（テストでは小さくする）を超えた購読が **503** になること。
  レビューイベントを publish したあとに購読した客が、それを**受け取る**こと。
- #133: `GET /_/api/groups/{group}` の応答 JSON に `"raw"` が**現れない**こと。
  かつストア側の `Raw` が**消えていない**こと。
- #152: `SBNN_LOG=info` でリクエスト行が出て、未設定では**出ない**こと。
- #153: diff の無いサーバが短いタイムアウトで `Run` から返ること。
  **diff を 1 つ持っているサーバは返らない**こと（こちらのほうが大事なテスト）。

## 完了条件（**実行すれば真偽が決まるもの。自己申告しない**）

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')
```

`gofmt -l` が**何も出力しない**のが合格。加えて:

```bash
# 担当外のファイルを触っていないこと（何も出力しなければ合格）
git diff --name-only origin/main | grep -v -e '^internal/server/server\.go$' -e '^internal/server/server_test\.go$'

# 各 issue の痕跡（それぞれ 1 行以上返れば合格）
grep -n 'Sec-Fetch-Site' internal/server/server.go            # #20
grep -n 'MaxBytesReader' internal/server/server.go            # #155
grep -n 'MaxBytesError' internal/server/server.go             # #155
grep -n 'Last-Event-ID\|retry:' internal/server/server.go     # #151
grep -n 'StatusServiceUnavailable' internal/server/server.go  # #151
grep -n 'SBNN_LOG' internal/server/server.go                  # #152
grep -n 'SBNN_IDLE_TIMEOUT' internal/server/server.go         # #153

# 上限まわりで LimitReader が残っていないこと（何も出力しなければ合格）
grep -n 'io.LimitReader(r.Body' internal/server/server.go
```

PR は 6 本（#20 と #155 は `Fixes`、#133 #151 #152 #153 は `Refs`）。

## やること・やらないこと

- **push と PR 作成まで行う。マージはしない。**
- 担当外は触らない。特に **`store.go` と `cmd/root.go` は 1 バイトも触らない**。
- 見つけた問題は自分で直さず、最終報告に書く。
- 判断に迷って止まらない。既定を決めて進み、決めた内容と理由を報告に書く。
- **`store.go` や `cmd/root.go` が空くのを待たない。** 待つくらいなら `Refs` で出す。

## 完了報告

COMMON.md の報告書式に加えて、先頭に次の 4 行をこの綴りのまま書く:

```
slug: srv-core
branch: gogo/issue-<N> を issue ごとに（全部列挙する）
worktree: /home/user/wt/srv-core
commit: <PR ごとの commit を全部列挙する>
```
