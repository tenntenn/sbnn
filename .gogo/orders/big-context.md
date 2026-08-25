slug: big-context

# big-context — ハンクの前後を開く機能の方針（issue #97）

ダッシュボードのタスク ID: t-af00b4（issue #97）
優先度: P2
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `big-context`）
  - `branch   = gogo/issue-97`
- **1 issue = 1 PR。** `origin/main` から切る。
- **push と PR 作成まで行う。マージはしない。**

## この指示文の性質（ここを読み違えると全部やり直しになる）

**実装をまるごと作らない。** issue #97 は、コードを書き始める前に決めるべきことが
決まっていない大きな提案である。この PR で出すのは**方針の文書 1 本**であり、
それに加えてよいのは「あとで方針が変わっても捨てなくて済む、最小の第一歩」だけ。

**本体の実装ファイルは、いま全部どこかのレーンが使用中である。**
`internal/` `cmd/` `web/src/` `main.go` `go.mod` `go.sum` `Taskfile.yml` `.github/`
`aqua.yaml` `README.md` は**1 バイトも触らない。** 触れば必ず衝突する。

## 担当ファイル（これ以外は 1 バイトも触らない）

- `docs/proposals/097-expand-hunk-context.md`（新規）**これ 1 ファイルだけ**

`web/dist/` は絶対にコミットしない。

## 作るもの: `docs/proposals/097-expand-hunk-context.md`

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

- `internal/source/source.go` — `NewSide` が作業ツリーのファイルを読み、
  `Result.Complete` を返していること。`AbsPath` の封じ込め検査。
- `internal/diff/reconstruct.go` — `Reconstruct(f *model.File) (content string, complete bool)` と
  `Snippet(f *model.File, side string, start, end int) string`。
  **`Snippet` が既にあるという事実は、この提案の要**である。何が足りないのかを正確に書く。
- `internal/model/model.go` の `File` / `Hunk` / `Line` の各型。
- `web/src/components/DiffFileSection.tsx` — ハンクの見出し行がどこで描かれているか。

「決めること」に必ず含める問い:

1. 展開した行を**サーバが返すのかブラウザが持つのか**。
   （`Snippet` がサーバ側にある以上、新しい API 1 本で済む可能性が高い。API の形を決める）
2. **ディスク上のファイルが diff の文脈行と一致しない場合**にどう振る舞うか。
   issue はこれを #40 の誤ラベル問題と同じ検査だと言っている。何を比べて、一致しなければ何を出すか。
3. 展開した行に**コメントを付けられるのかどうか**。
   issue は「付けられてはいけない、さもないと #23 が裏側から再発する」と言っている。
   その主張が正しいかをコードで確かめ、決める。
4. ファイルがディスクに無い場合（`Result.Kind` が `FromDiff` のとき）の見せ方。

「後戻りしない第一歩」の候補（どれか 1 つを選んで書く。**この PR では実装しない**）:
文脈行とディスク上のファイルが一致するかを判定する関数の仕様、あるいは
展開 API のリクエスト / レスポンス JSON の定義。


## 完了条件（自分で実行して真偽を決められること）

```bash
test -f docs/proposals/097-expand-hunk-context.md
test $(wc -l < docs/proposals/097-expand-hunk-context.md) -ge 60
grep -n '^## 決めること' docs/proposals/097-expand-hunk-context.md
grep -n '^## 現状（コードを読んで確かめた事実）' docs/proposals/097-expand-hunk-context.md
grep -n '^## 選択肢' docs/proposals/097-expand-hunk-context.md
grep -n '^## 決定' docs/proposals/097-expand-hunk-context.md
grep -n '^## 後戻りしない第一歩' docs/proposals/097-expand-hunk-context.md
grep -n '^## やらないこと' docs/proposals/097-expand-hunk-context.md
grep -n '^## 次の 1 PR の範囲' docs/proposals/097-expand-hunk-context.md

# 文書が挙げたコードのパスが全部実在すること（何も返らないのが合格）
grep -oE '(internal|cmd|web|version)/[A-Za-z0-9_./-]+\.(go|ts|tsx|css|json|yaml)' docs/proposals/097-expand-hunk-context.md \
  | sort -u | while read -r p; do test -e "$p" || echo "MISSING: $p"; done

# 実装ファイルに手を出していないことの証明（何も返らないのが合格）
git diff --name-only origin/main | grep -vE '^docs/proposals/097-expand-hunk-context\.md$'

go build ./... && go vet ./... && go test ./...
```


## コミット / PR

- 件名は英語・命令形（COMMON.md のとおり）。例: `Propose ...`。
- **フッタは `Refs #97` にする。`Fixes #97` にしない。** 方針文書は issue を閉じない。
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
