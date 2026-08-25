slug: big-pkg

# big-pkg — diff パーサを公開パッケージにする案の方針（issue #128）

ダッシュボードのタスク ID: t-d22494（issue #128）
優先度: P2
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `big-pkg`）
  - `branch   = gogo/issue-128`
- **1 issue = 1 PR。** `origin/main` から切る。
- **push と PR 作成まで行う。マージはしない。**

## この指示文の性質（ここを読み違えると全部やり直しになる）

**実装をまるごと作らない。** issue #128 は、コードを書き始める前に決めるべきことが
決まっていない大きな提案である。この PR で出すのは**方針の文書 1 本**であり、
それに加えてよいのは「あとで方針が変わっても捨てなくて済む、最小の第一歩」だけ。

**本体の実装ファイルは、いま全部どこかのレーンが使用中である。**
`internal/` `cmd/` `web/src/` `main.go` `go.mod` `go.sum` `Taskfile.yml` `.github/`
`aqua.yaml` `README.md` は**1 バイトも触らない。** 触れば必ず衝突する。

## 担当ファイル（これ以外は 1 バイトも触らない）

- `docs/proposals/128-export-diff-package.md`（新規）
- `internal/diff/apisurface_test.go`（新規。**このファイル名以外にしない**）

`internal/diff/parse.go` などの**実装ファイルは 1 行も変えない。**
既存の `internal/diff/parse_test.go` にも触らない（別レーンの担当）。

`web/dist/` は絶対にコミットしない。

## 作るもの: `docs/proposals/128-export-diff-package.md`

`docs/proposals/` はまだ存在しない。このディレクトリを作る最初の PR の 1 つになる。
索引ファイル（`docs/proposals/README.md`）は**作らない**。同じ波で他の 5 本の提案が
並行して書かれており、索引は共有ファイルになって必ず衝突する。

見出しは**次の 7 つを、この綴りのまま、この順で**使う。増やしてよいが減らさない。

```
## 決めること
## 現状（コードを読んで確かめた事実）
## 選択肢
## 決定
## 後戻りしない第一歩
## やらないこと
## 次の 1 PR の範囲
```

各見出しに入れる中身:

- **決めること** — この issue で先に決着させるべき問いを、箇条書きで 2〜4 個。
  「実装するかどうか」ではなく「どちらの形にするか」を問いにする。
- **現状（コードを読んで確かめた事実）** — **必ず実際にファイルを読んで書く。**
  関数名・型名・ファイルパス・行番号を挙げ、短い引用を添える。
  **推測を事実として書かない。** 読んでいないことは書かない。
- **選択肢** — 2 つ以上。それぞれについて「できるようになること」と「払う代償」を書く。
  issue 本文が挙げている案は必ず含める。
- **決定** — **1 つ選ぶ。「どちらもあり得る」で終わらせない。** 選んだ理由を書く。
  ユーザにしか決められない事柄（外部公開・課金・後戻りできない破壊）が含まれるなら、
  そこだけを「ユーザの決めが要る点」として明示し、**残りは決めて先に進める。**
- **後戻りしない第一歩** — 決定がひっくり返っても捨てずに済む、最小の一歩を 1 つ。
- **やらないこと** — この提案の範囲外を明示する。
- **次の 1 PR の範囲** — 次に出す PR が触るファイルと、その完了条件を、
  **そのまま指示文にできる粒度で**書く。

分量は 60 行以上。長さを競わない。**決まっていないことを決めるのが仕事**であって、
issue の言い直しではない。

## この issue で必ず扱うこと

読んでから書くこと（**実際に開いて確かめる**）:

- `internal/diff/` の公開関数。issue は 9 つと書いている。**自分で数え直す。**
  実測では次の 9 つである（違っていたらそのまま書く）:
  `GeneratedMarker` `VisibleTop`（generated.go）/ `IsMarkdown` `IsImage`
  `ImageContentType` `IsNotebook` `Parse`（parse.go）/ `Reconstruct` `Snippet`（reconstruct.go）
- `internal/model/model.go` の `File` 型。issue が言う「diff が言ったこと」と
  「UI が決めたこと」が同じ構造体に混ざっている、という主張をフィールド単位で確かめる。
  `ID` `Folded` `FoldReason` `ViewMode` `IsMarkdown` が本当にそうかを 1 つずつ見る。
- `internal/diff/` の `finalize` が `ViewMode` と `Folded` を設定しているという主張の真偽。

「決めること」に必ず含める問い:

1. **いま公開するか、v1 まで待つか。** issue はこの 2 択を自分で並べている。決める。
2. 公開するなら **`model` ごと出すのか、`diff` 用の最小の型を切り出すのか。**
3. `model.File` から「UI が決めたこと」を分離するのは、**公開するかどうかと独立に
   やる価値があるか。** issue はそう主張している。真偽を判断して書く。
4. 分離するなら、`internal/server` と `web/src/types.ts` にどこまで波及するか
   （見積もりでよい。実装しない）。

## 追加でよい「後戻りしない第一歩」: `internal/diff/apisurface_test.go`

公開するにせよしないにせよ、**いまの公開面が何であるかを機械に固定させておく**のは
捨てずに済む一歩である。次のテストを 1 本だけ書く。

- `package diff`（内部テスト）。
- `internal/diff` パッケージの**公開識別子の一覧**を、テストの中に文字列の表として持ち、
  実際の一覧と一致しなければ失敗する。
- 一覧の取り方は `go/ast` + `go/parser` でこのディレクトリの `*.go`（`_test.go` を除く）を
  読み、`ast.IsExported` で絞る。**外部コマンドを呼ばない。新しい依存を足さない。**
- トップレベルの識別子はすべて `apiSurface` で始める（`apiSurfaceExported` など）。
  テスト関数名は `TestAPISurface`。既存の `parse_test.go` や、別レーンが後で足す
  テストと名前がぶつからないようにするため。
- テストが落ちたときのメッセージに「公開面を変えたなら、この表も更新すること。
  変えるつもりが無かったなら、それは意図しない公開である」と書く。

**このテストは公開面を「凍結」するものではない。** 変更を禁じるのではなく、
変更が意識的な行為になるようにするものである。その意図をテストのコメントに書く。


## 完了条件（自分で実行して真偽を決められること）

```bash
test -f docs/proposals/128-export-diff-package.md
test $(wc -l < docs/proposals/128-export-diff-package.md) -ge 60
grep -n '^## 決めること' docs/proposals/128-export-diff-package.md
grep -n '^## 現状（コードを読んで確かめた事実）' docs/proposals/128-export-diff-package.md
grep -n '^## 選択肢' docs/proposals/128-export-diff-package.md
grep -n '^## 決定' docs/proposals/128-export-diff-package.md
grep -n '^## 後戻りしない第一歩' docs/proposals/128-export-diff-package.md
grep -n '^## やらないこと' docs/proposals/128-export-diff-package.md
grep -n '^## 次の 1 PR の範囲' docs/proposals/128-export-diff-package.md

# 文書が挙げたコードのパスが全部実在すること（何も返らないのが合格）
grep -oE '(internal|cmd|web|version)/[A-Za-z0-9_./-]+\.(go|ts|tsx|css|json|yaml)' docs/proposals/128-export-diff-package.md \
  | sort -u | while read -r p; do test -e "$p" || echo "MISSING: $p"; done

# 実装ファイルに手を出していないことの証明（何も返らないのが合格）
git diff --name-only origin/main | grep -vE '^(docs/proposals/128-export-diff-package\.md|internal/diff/apisurface_test\.go)$'

go build ./... && go vet ./... && go test ./...
```

追加の完了条件（`internal/diff/apisurface_test.go` について）:

```bash
test -f internal/diff/apisurface_test.go
go test ./internal/diff/ -run APISurface -v      # PASS すること
gofmt -l internal/diff/apisurface_test.go        # 何も返らないのが合格
grep -c '^func \|^var \|^type ' internal/diff/apisurface_test.go
# 実装ファイルを触っていないことの証明（何も返らないのが合格）
git diff --name-only origin/main | grep -E '^internal/diff/.*\.go$' | grep -v apisurface_test
```

**「壊したら落ちること」を確かめる。** `internal/diff/` に公開関数を 1 つ一時的に足して
`go test ./internal/diff/ -run APISurface` が FAIL することを確認し、出力を報告に貼り、
`git checkout -- internal/diff/` で必ず戻す。戻したことを `git status --porcelain` で確認する。


## コミット / PR

- 件名は英語・命令形（COMMON.md のとおり）。例: `Propose ...`。
- **フッタは `Refs #128` にする。`Fixes #128` にしない。** 方針文書は issue を閉じない。
  この issue は open のまま残す。
- PR 本文の `## What changed` には、決定の 1 行要約を書く。
- PR 本文の `## Verification` には、上の完了条件のコマンドと結果を貼る。

## 報告に書くこと（**issue へのコメントは書かない。メインが書く**）

1. 何を決めたか（1 行）。
2. **この issue を「過大」または「前提が誤っている」と判断したかどうか。**
   判断したなら、その根拠をコードの引用付きで書く。メインがそれを issue に返信する。
3. ユーザの決めが要る点が残ったなら、その 1 点だけを明示する。
4. `slug` / branch / worktree / commit の 4 つを、この指示文と同じ綴りで。

## 全体を通しての決まり

- 担当外のファイルは触らない。見つけた問題は自分で直さず報告に書く。
- 判断に迷って止まらない。既定を自分で決めて進み、決めた内容と理由を報告に書く。
- 実装を「ついでに」始めない。この PR の価値は、**次の 1 PR がすぐ書けること**にある。
