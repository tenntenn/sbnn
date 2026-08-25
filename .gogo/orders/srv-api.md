slug: srv-api

# 指示文 SRV-01 — internal/server の API 入力検証（7 issue）

- 優先度: **P0**（#134, #24） / **P1**（#21, #22, #23, #29, #30）
- 期限: P0 の 2 件は 2026-08-25 中。残り 5 件は 2026-08-26 中。
- グループ: G6

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` — 手順・検証・コミット・PR の書式はすべてここに従う。
  この指示文はそれを上書きしない。食い違ったら COMMON.md が優先。
- `/home/user/briefs/TASKIDS.tsv` — issue とタスク ID の対応。

## 名前

```
worktree = /home/user/wt/srv-api      # slug から機械的に導出する。他の場所に worktree 名は書かない
branch   = gogo/issue-<N>             # COMMON.md のとおり、issue ごとに origin/main から切り直す
```

worktree が無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/srv-api origin/main
```

## 着手条件（**これを満たすまで 1 バイトも編集しない**）

`internal/server/server.go` は同時に 1 レーンしか持てない。いま G1 の store レーンが持っている。

- **メインから「G1 の store レーンが完了した」と伝えられるまで、編集を始めない。**
  自分でその判定をしない。store レーンのブランチや worktree を見に行かない。
- 待っている間にやってよいこと（読み取りのみ・コミットしない）:
  下の 7 件の issue 本文を `mcp__github__issue_read` で全部読む、
  `/home/user/sbnn/internal/server/server.go` と `server_test.go` を読む。
- 待っている間にやってはいけないこと: 編集・ブランチ作成・コミット・push。

## 触ってよいファイル（**この 2 本だけ**）

```
internal/server/server.go
internal/server/server_test.go
```

**この 2 本から出ない。** 特に次は他のレーンが使用中なので、1 行も触らない:

| ファイル | 使用中のレーン |
|---|---|
| `internal/server/store.go` | G1 store |
| `internal/server/prompt.go` | export-pkg |
| `internal/server/proxy.go` | mo-proxy |
| `internal/server/hook.go` | srv-hook（SRV-02） |
| `internal/server/preview.go` `spa.go` | srv-preview（SRV-03） |
| `cmd/server.go` | cmd-serve（SRV-05） |
| `web/` 配下すべて | G2〜G5 の web レーン |

**重要**: issue 本文が `store.go` の関数（`Store.AddComment` など）を直せと読める場合でも、
**直すのはハンドラ側（`server.go`）である。** ストアに入れる前に弾く。
理由は、`store.go` を別レーンが持っているので、ここで触ると相互待ちになるため。
ハンドラ側で弾けば、この 7 件はすべて `server.go` だけで閉じる。

## 作業の順番（**この順で 1 件ずつ、1 issue = 1 PR**）

#21 #22 #23 #134 は **どれも `handleAddComment` を触る**。各 PR は COMMON.md のとおり
毎回 `origin/main` から切るので、**後の PR は先の PR とテキストが衝突する。**
衝突を最小にするため、次の順で、**各 PR の差分をその issue が要求する箇所だけに絞る**こと。
共通のバリデーション関数へまとめるリファクタは**しない**（1 本にまとめると 4 issue が 1 PR になる）。

1. **#22**（task `t-d2b3f1`, P1） — `handleAddComment` で `StartLine < 1` を 400。
   `EndLine` が 0 のときは `StartLine` と同じ扱い、`EndLine < StartLine` も 400。
2. **#134**（task `t-f80c17`, **P0**） — `handleAddComment` の
   `if req.Side != "old" { req.Side = "new" }` を捨て、CLI と同じ厳密さにする。
   空文字は `new` に既定。`strings.ToLower(strings.TrimSpace(...))` した結果が
   `new` / `old` のどちらかなら受け付け、**それ以外は 400**（`OLD` `Old` ` old` は
   `old` として受け付ける／`left` `NEWW` などは 400）。
   ここは **P0**。いまは `"side":"OLD"` を送ると黙って **new 側**に付き、
   レビュアーが誰も頼んでいない行にコメントが出る。壊れ方が「静かに違う場所」なので、
   壊れていることに誰も気づけない。
3. **#21**（task `t-b5b532`, P1） — `handleAddComment` で `FileID` が空でないのに
   その `DiffID` のファイルに存在しないなら 400。`diffId` が未知のときの既存の扱いに合わせる。
   **`store.go` は触らない**。ハンドラ内で対象 diff のファイル一覧を引いて確かめる。
4. **#23**（task `t-9fc944`, P1） — path 指定でスニペットを取ったとき、
   `endLine` が diff の外に出ているなら、**スニペットが実際に覆った最後の行へ clamp する**。
   clamp した事実は 200 のレスポンス（保存された `endLine`）に出る。
   reject ではなく clamp を採る。理由は、既存クライアント（web の範囲選択）が
   端で 1 行はみ出すことがあり、そこで 400 にすると既存の操作が壊れるため。
   **この既定を変える判断はしない。迷ったら clamp。**
5. **#24**（task `t-889ec6`, **P0**） — `handleSubmitReview` の `r.ContentLength > 0` を
   `r.ContentLength != 0` にする。そのうえで `Decode` が `io.EOF` を返した場合だけは
   「本文なし」として**エラーにせず**既定値で続行する。それ以外の decode エラーは従来どおり 400。
   ここは **P0**。chunked で送ると `approved` / `changes-requested` が黙って捨てられて
   `commented` になる。`sbnn wait --exit-code` と `sbnn submit --exit-code` がその値で動くので、
   「承認された」と「変更を求められた」が入れ替わって下流へ流れる。
6. **#29**（task `t-1f40f3`, P1） — `handleGroup` で、**存在するが空のグループ**でも
   `diffs` と `comments` が `null` ではなく `[]` になるようにする。
   既存の「見つからない場合」の分岐と同じ形に揃える。`store.go` は触らない
   （ハンドラで返す直前に nil を空スライスへ差し替える）。
7. **#30**（task `t-c8322d`, P1） — `handleDeleteHooks` は
   `DELETE .../hooks`（全消し）と `DELETE .../hooks/{id}`（1 件）の両方を受けている。
   **`{id}` があって 1 件も消えなかったら 404**。`{id}` が無い全消しは従来どおり 200 と件数。

各 PR の本文に、**同じ関数を触る他の issue 番号を 1 行で書く**こと（例:
`Touches handleAddComment; overlaps #22 #23 #134 — rebase may be needed.`）。
マージ担当が順番を決められるようにするためで、あなたが順番を決めるのではない。

## テスト（必須。COMMON.md の「テストを必ず足す」の具体）

`internal/server/server_test.go` の既存のテーブル駆動テストの書き方に合わせる。
**7 件それぞれに、修正前に落ちて修正後に通る回帰テストを 1 つ以上足す。**
`httptest` でハンドラを直接叩き、**ステータスコードと、保存された値の両方**を見る。
「200 が返った」だけのテストにしない（#134 も #23 も 200 が返るのが問題だった）。

## 完了条件（**実行すれば真偽が決まるもの。自己申告しない**）

worktree で、各 PR のブランチにいる状態で全部通ること:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')
```

`gofmt -l` が**何も出力しない**のが合格。加えて:

```bash
# 担当外のファイルを触っていないこと（何も出力しなければ合格）
git diff --name-only origin/main | grep -v -e '^internal/server/server\.go$' -e '^internal/server/server_test\.go$'

# 7 件すべてで、その issue のテストが存在すること（1 行以上返れば合格）
go test ./internal/server/ -run 'TestHandleAddComment|TestHandleSubmitReview|TestHandleGroup|TestHandleDeleteHooks' -v | grep -c '^=== RUN'
```

PR は 7 本。**それぞれ `Fixes #<N>` を本文に書く**（この 7 件はどれも担当ファイル内で完結する）。

## やること・やらないこと

- **push と PR 作成まで行う。マージはしない。**
- 担当外は触らない。見つけた問題は自分で直さず、最終報告に書く。
- 判断に迷って止まらない。既定を決めて進み、決めた内容と理由を報告に書く。
- issue の前提が事実と違う／再現しない場合は、無理に直さず COMMON.md の
  「issue がおかしいと思ったとき」に従って報告へ書く。

## 完了報告

COMMON.md の報告書式に加えて、先頭に次の 4 行をこの綴りのまま書く:

```
slug: srv-api
branch: gogo/issue-<N> を issue ごとに（全部列挙する）
worktree: /home/user/wt/srv-api
commit: <PR ごとの commit を全部列挙する>
```
