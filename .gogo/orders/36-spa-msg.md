slug: spa-msg

# 指示文 36 — #36 が PR #221 で充足済みかを確認して決着させる

- 対応 issue: #36
- ダッシュボードのタスク ID: `t-8f48db`
- 作業者: 1 名
- 優先度: 中
- 期限: 2026-08-25 中
- 共通規約: `/home/user/briefs/COMMON.md`。ただし**下の「この指示文の性質」が優先する。**

## この指示文の性質 — ソースを 1 行も変更しない

**担当ファイル: なし。**

`internal/server/spa.go` は **`srv-preview` レーンの担当ファイル**である
（`.gogo/orders/srv-preview.md` が #88 / #89 のために保持している）。
このレーンは `spa.go` を**書き換えない**。読むだけ。

- **worktree を使わない。ブランチを切らない。コミットしない。push しない。PR を作らない。**
  したがって slug から決まる `.gogo/wt/spa-msg` / `gogo/spa-msg` は**作らない**。
- リポジトリ (`/home/user/sbnn`) は**読むだけ**。`grep` で裏を取るのは構わない。
- 使うツールは `mcp__github__pull_request_read` と `mcp__github__issue_read`、
  それに読み取り用の `grep` だけ。

ファイルを 1 つも触らないので、他のどの指示文とも担当ファイルは重ならない。

## 前提（調査済み。再調査は不要）

1. #36 が問題にしている文字列は `web/web.go` ではなく
   **`internal/server/spa.go:22`** にある。`main` (`7aa2d78`) 時点の実物:

   ```go
   io.WriteString(w, "the sbnn web UI is not built into this binary.\n"+
       "Run `make build` (it runs `pnpm build` in web/) and reinstall sbnn.\n")
   ```

2. リポジトリに `Makefile` は無い。`Taskfile.yml` に `build` タスクが実在する。
   したがって正しい案内は `task build`。

3. **この issue には既に PR #221「Tell the reader to run task build, not make build」が
   出ている**（open・未マージ・`mergeable_state: clean`・head `gogo/issue-36`）。
   見たかぎり #36 の Expected を完全に満たしている。メッセージを定数
   `uiNotBuiltMessage` に括り出して `task build` に直し、
   `internal/server/spa_message_test.go` を新規追加して、
   メッセージが名指しする `task <名前>` が `Taskfile.yml` に実在することまで検査している。

4. `internal/server/spa.go` には未マージの PR が他に 2 本かかっている。
   - PR #210「Answer asset-looking paths with 404 instead of the SPA」(issue #88)
   - PR #215「Render the SPA only for the root and a group name」(issue #89)
   どちらも `srv-preview` レーンのもので、`main` には入っていない。
   **つまり `spa.go` には既に 3 本の未マージ PR が重なっている。
   4 本目を作ってはならない。**

## やること

### 手順 1 — PR #221 が #36 を満たしているかを確認する

`mcp__github__pull_request_read`（owner=tenntenn, repo=sbnn, pullNumber=221, method=`get_diff`）で
diff を取得し、次の 4 点を自分の目で確認する。

1. `internal/server/spa.go` の案内文が `task build` になっている
2. `make build` という文字列が `internal/server/spa.go` から消えている
3. 1 行目 `the sbnn web UI is not built into this binary.` が変わっていない
4. 回帰テストが付いている（`internal/server/spa_message_test.go`）

あわせて `mcp__github__pull_request_read`（pullNumber=221, method=`get`）で
`state` が `open`、`base.ref` が `main` であることを確認する。

### 手順 2 — 結果で分岐する

**A. 4 点すべて満たしていた場合（想定される結末）**

この issue の実装は完了済み。**何も実装しない。何も作らない。**
「#36 は PR #221 で充足済み。新規 PR なし」と報告して終わる。
**これは失敗ではなく、この指示文の正常な結末である。**
マージ担当が #221 をマージすれば `Fixes #36` で issue は自動的に閉じる。

**B. 満たしていない点があった、または PR #221 が閉じられていた場合**

**このレーンでは実装しない。** `spa.go` は `srv-preview` の担当ファイルであり、
ここで触ると #210 / #215 と衝突する。

代わりに、**足りない点を具体的に書いて報告する。**

- どの点（上の 1〜4 のどれ）が満たされていないか
- `spa.go` の何行目がどうあるべきか
- 「`srv-preview` レーンに回すか、`srv-preview` の完了後に別レーンを立てる必要がある」

と報告に明記する。メインが差配する。**自分で実装に踏み込まない。**

## 完了条件（実行すれば真偽が決まるもの）

以下を実行し、出力を報告に貼ること。

```bash
# main 側の現状。1 行返るのが現状（まだ直っていない）
grep -n 'make build' /home/user/sbnn/internal/server/spa.go
# main 側にはまだ task build は無い。何も返らないのが現状
grep -n 'task build' /home/user/sbnn/internal/server/spa.go
# Taskfile に build タスクが実在すること。1 行以上返れば合格
grep -n '^  build:' /home/user/sbnn/Taskfile.yml
# このレーンがソースを変更していないこと。何も返さないのが合格
git status --short
```

加えて、PR #221 の diff のうち
`internal/server/spa.go` の該当箇所を報告に**引用する**こと。
「確認した」だけでは完了条件を満たさない。

## 守ること

- **ソースを変更しない。** 直したくなっても直さない。報告に書く。
- **担当外は触らない。** #37（`web/web.go` の `make` 残骸）はこのレーンの担当ではない。
- **判断に迷って止まらない。** 上の分岐どおり実行し、迷った点を報告に書く。
- モデル名（Opus / Claude / AI 等）を報告本文に書かない。

## 報告に書くこと

冒頭に `slug: spa-msg` と書く。
**branch / worktree / commit は「なし（ソース無変更）」と明記する。**
そのあと、A / B のどちらの結末だったかを 1 行で書き、
上の完了条件の出力と PR #221 の引用を続ける。
