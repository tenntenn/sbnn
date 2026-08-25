slug: big-reply

# big-reply — コメントへの返信の方針（issue #98）

優先度: P2
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `big-reply`）
  - `branch   = gogo/issue-98`
- **1 issue = 1 PR。** `origin/main` から切る。
- **push と PR 作成まで行う。マージはしない。**

## この指示文の性質（ここを読み違えると全部やり直しになる）

**実装をまるごと作らない。** issue #98 は、コードを書き始める前に決めるべきことが
決まっていない大きな提案である。この PR で出すのは**方針の文書 1 本**であり、
それに加えてよいのは「あとで方針が変わっても捨てなくて済む、最小の第一歩」だけ。

**本体の実装ファイルは、いま全部どこかのレーンが使用中である。**
`internal/` `cmd/` `web/src/` `main.go` `go.mod` `go.sum` `Taskfile.yml` `.github/`
`aqua.yaml` `README.md` は**1 バイトも触らない。** 触れば必ず衝突する。

## 担当ファイル（これ以外は 1 バイトも触らない）

- `docs/proposals/098-comment-replies.md`（新規）**これ 1 ファイルだけ**

`web/dist/` は絶対にコミットしない。

## 作るもの: `docs/proposals/098-comment-replies.md`

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

- `internal/model/model.go` の `type Comment struct`（147 行目付近）。
  いまあるフィールドを全部挙げる。`Question` と `Resolved` しか会話の状態が無いことを確かめる。
- `internal/server/` のコメント API（`POST /_/api/groups/{group}/comments`、
  `DELETE .../comments/{id}`、`GET .../comments`、`GET .../prompt`）。
  実際のハンドラを読んでルートを列挙する。
- `web/src/components/CommentThread.tsx` — いまの「スレッド」が何をしているか。
- `cmd/comment.go` の `runComment` と、そのフラグ定義。

「決めること」に必ず含める問い:

1. **`ParentID` を足すのか、`POST .../comments/{id}/replies` を足すのか、
   既存の作成 API に `parentId` を足すのか。** 3 つのうち 1 つを選ぶ。
2. **保存済みのデータとの互換**。`ParentID` を後から足したとき、既存の
   セッションファイル（`session-*.json`）はどうなるか。実際に保存形式を読んで書く。
3. **「答えの付いた質問は未回答に数えない」をどこで判定するか。**
   `GET .../prompt` の生成側か、それとも保存時か。
4. `sbnn comment --reply-to` の値は何か（コメント ID か、行の指定か）。

「後戻りしない第一歩」の候補（どれか 1 つを選ぶ。**この PR では実装しない**）:
返信を含んだときの `prompt` の出力例（テキストそのもの）を提案に貼り、
「スレッドをスレッドとして読ませる」が具体的に何を意味するかを固定する、など。


## 完了条件（自分で実行して真偽を決められること）

```bash
test -f docs/proposals/098-comment-replies.md
test $(wc -l < docs/proposals/098-comment-replies.md) -ge 60
grep -n '^## 決めること' docs/proposals/098-comment-replies.md
grep -n '^## 現状（コードを読んで確かめた事実）' docs/proposals/098-comment-replies.md
grep -n '^## 選択肢' docs/proposals/098-comment-replies.md
grep -n '^## 決定' docs/proposals/098-comment-replies.md
grep -n '^## 後戻りしない第一歩' docs/proposals/098-comment-replies.md
grep -n '^## やらないこと' docs/proposals/098-comment-replies.md
grep -n '^## 次の 1 PR の範囲' docs/proposals/098-comment-replies.md

# 文書が挙げたコードのパスが全部実在すること（何も返らないのが合格）
grep -oE '(internal|cmd|web|version)/[A-Za-z0-9_./-]+\.(go|ts|tsx|css|json|yaml)' docs/proposals/098-comment-replies.md \
  | sort -u | while read -r p; do test -e "$p" || echo "MISSING: $p"; done

# 実装ファイルに手を出していないことの証明（何も返らないのが合格）
git diff --name-only origin/main | grep -vE '^docs/proposals/098-comment-replies\.md$'

go build ./... && go vet ./... && go test ./...
```


## コミット / PR

- 件名は英語・命令形（COMMON.md のとおり）。例: `Propose ...`。
- **フッタは `Refs #98` にする。`Fixes #98` にしない。** 方針文書は issue を閉じない。
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
