slug: srv-preview

# 指示文 SRV-03 — プレビューの正直さと SPA キャッチオール（3 issue）

- 優先度: **P1**（#40, #88, #89）
- 期限: 2026-08-26 中
- グループ: G6

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` — 手順・検証・コミット・PR の書式はすべてここに従う。
  この指示文はそれを上書きしない。食い違ったら COMMON.md が優先。
- `/home/user/briefs/TASKIDS.tsv` — issue とタスク ID の対応。

## 名前

```
worktree = /home/user/wt/srv-preview  # slug から機械的に導出する。他の場所に worktree 名は書かない
branch   = gogo/issue-<N>             # COMMON.md のとおり、issue ごとに origin/main から切り直す
```

worktree が無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/srv-preview origin/main
```

## 着手条件

**すぐ着手してよい。誰の完了も待たない。** 担当ファイルは他のどのレーンとも重なっていない。

## 触ってよいファイル（**この 4 本だけ**）

```
internal/server/preview.go
internal/server/spa.go
internal/server/preview_test.go   （新規に作ってよい。いま存在しない）
internal/server/spa_test.go       （新規に作ってよい。いま存在しない）
```

**この 4 本から出ない。** 次は他のレーンが使用中なので 1 行も触らない:

| ファイル | 使用中のレーン |
|---|---|
| `internal/server/server.go` `store.go` | G1 store → srv-api（SRV-01）→ srv-core（SRV-04） |
| `internal/server/hook.go` | srv-hook（SRV-02） |
| `internal/server/prompt.go` | export-pkg |
| `internal/server/proxy.go` | mo-proxy |
| `internal/source/source.go` | source レーン（G1, issue #41） |
| `internal/diff/` | diff-parse / diff-misc レーン（G1） |
| `web/src/markdown.ts` ほか `web/` 配下すべて | G4 web-markdown ほか web レーン |
| `cmd/` 配下すべて | cmd-* レーン |

`internal/source` と `internal/diff` と `internal/model` は**読む・呼ぶだけ**なら使ってよい。
**編集はしない。**

## 1 件目: #40 — 当てていないパッチが「tree・complete」として表示される（task `t-07ffb7`）

`internal/server/preview.go` の `previewer.resolve` は、`source.NewSide` が
`FromWorktree` を返したらそれを**そのまま信じて** `SourceWorktree` / `complete=true` を返す。

```go
got := source.NewSide(d.BaseDir, f)
if got.Kind == source.FromWorktree {
	return got.Path, SourceWorktree, got.Complete, nil
}
```

`git diff | sbnn`（適用済み）なら正しい。だが README が宣伝している
`cat change.patch | sbnn`（**未適用**）では、ディスクの中身は**古い側**である。
そのとき sbnn は「これが完全な新しい側だ」と積極的に嘘をつく。

やること（**すべて `preview.go` の中**。`internal/source` は触らない）:

1. `resolve` の中で、`got.Kind == source.FromWorktree` のときに
   **ワークツリーのファイルが本当に新しい側かを確かめる関数**を通す。
   例: `func worktreeMatchesNewSide(content string, f *model.File) bool`。
2. 判定の仕方（**この方式を採る。別案を発明しない**）:
   ワークツリーの中身を行に割り、`f` の各ハンクについて、
   **文脈行（context）と追加行（addition）**が、そのハンクの**新しい側の行番号**
   （`Hunk.NewStart` から数える）にあるワークツリーの行と**一致するか**を見る。
   1 か所でも食い違ったら「適用されていない」と判定する。
   行末の改行の扱いだけは寛容にしてよい（`strings.TrimRight(line, "\r")`）。
3. 適用されていないと判定したら、**`SourceWorktree` を返さない。**
   下の再構築の経路（`SourceReconstructed`）へ落とす。
   `complete` は `source.NewSide` が返した `Complete` をそのまま使う
   （再構築が部分的なら `false` のまま出る。それでよい。
   **正直な「rebuilt / partial」は、自信のある間違いより良い。**）。
4. バイナリファイル（`f.IsBinary`）とハンクが 0 本のファイルは、
   この判定の対象外にする（従来どおりワークツリーを使う）。判定材料が無いため。

これは担当ファイルの中で完結する。**PR 本文には `Fixes #40` を書いてよい。**

## 2 件目: #88 — 相対画像が壊れ、サーバが 200 text/html で答える（task `t-0c9008`）

**この issue は担当ファイルの中だけでは全部は直せない。分かっている範囲を先に直す。**

| 求められていること | どこにある | この PR で |
|---|---|---|
| SPA キャッチオールがアセットらしいパスに `index.html` を返すのをやめる | `spa.go` | **やる** |
| Markdown の相対 `src` を書き換える | `web/src/markdown.ts` | **やらない**（G4 web-markdown が使用中） |
| diff の `BaseDir` 基準で兄弟ファイルを配る新エンドポイント | `server.go` の mux 登録が要る | **やらない**（server.go は他レーンが使用中） |

やること（**すべて `spa.go` の中**）:

1. `spaHandler` は、リクエストパスの**最後のセグメントに拡張子がある**
   （`.` を含み、`.` が先頭でなく、`.` の後ろが 1 文字以上）場合、
   組み込みアセットとして見つからなければ **`index.html` を返さず 404 を返す。**
   本文は `text/plain; charset=utf-8` で、その旨が読める 1 行にする
   （例: `not found: /diagram.png is not part of the sbnn UI`）。
2. 組み込みアセットが見つかる場合の挙動（`assets/` の `Cache-Control` を含む）は
   **一切変えない。**
3. `/` と拡張子の無いパスの挙動も、この PR では**変えない**（3 件目で扱う）。

いまは「画像を頼んだのにページが 200 で返ってきて、devtools に何のエラーも出ない」ため
原因が追えない。404 になれば、ただの壊れたリンクとして読める。

**PR 本文には `Fixes #88` を書かない。`Refs #88` を書く。**
`## What changed` の末尾に、上の表の「やらない」2 行を英語でそのまま書く。

## 3 件目: #89 — 相対リンクが sbnn のレビューページをもう一度開く（task `t-6883cd`）

**この issue も担当ファイルの中だけでは全部は直せない。**

| 求められていること | どこにある | この PR で |
|---|---|---|
| グループ名に見えるパスだけを SPA として扱う | `spa.go` | **やる** |
| 相対リンクをファイル自身のディレクトリ基準で解決する | `web/src/markdown.ts` | **やらない**（G4 web-markdown） |
| 相対リンクに `target="_blank"` を付けない | `web/src/markdown.ts` | **やらない**（同上） |

やること（**すべて `spa.go` の中**）:

1. `/` 以外のパスで `index.html` を返すのは、
   **パスがちょうど 1 セグメントで、`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$` に一致する場合だけ**にする。
   一致しないもの（スラッシュを含む多段パス、64 文字を超えるもの、
   先頭が記号のもの）は **404**（本文は 2 件目と同じ形式）。
2. `.` を含むが拡張子として成立しないもの（末尾が `.` など）は 404 側に倒す。
   2 件目の拡張子ルールと矛盾しないよう、**先に拡張子ルール、次にグループ名ルール**の順で判定する。
3. **存在するグループかどうかをストアに問い合わせない。**
   diff が届く前にページを開く経路があるので、存在確認で 404 にすると既存の使い方が壊れる。
   **この既定を変える判断はしない。**

**この PR は 2 件目（#88）と同じ `spa.go` の同じ関数を触る。**
COMMON.md のとおり毎回 `origin/main` から切るので、テキストが衝突する。
PR 本文に `Touches spaHandler; overlaps #88 — rebase may be needed.` と 1 行書く。
順番は #88 → #89 で作る。順番を自分で入れ替えない。

**PR 本文には `Fixes #89` を書かない。`Refs #89` を書く。**

## テスト（必須）

- `internal/server/preview_test.go`（新規）— #40 のテーブル駆動テスト。
  最低 3 ケース: 「パッチ適用済みのワークツリー → `SourceWorktree`」、
  「未適用（ディスクが古い側）→ `SourceReconstructed`」、
  「バイナリ → 従来どおり」。
  `t.TempDir()` にファイルを書いて `model.Diff` / `model.File` を組み立てる。
- `internal/server/spa_test.go`（新規）— #88 / #89 のテーブル駆動テスト。
  `httptest` で `spaHandler` を叩き、**ステータスコードと Content-Type の両方**を見る。
  最低: `/`（200 text/html）、`/default`（200 text/html）、
  `/diagram.png`（**404**）、`/other.md`（**404**）、
  `/assets/<実在するアセット名>`（200・`Cache-Control` が付く）、
  `/a/b/c`（404）、64 文字超のパス（404）。

`web.Built()` が false の環境ではハンドラが 503 を返す分岐に落ちる。
テストがその分岐に落ちるなら `t.Skip` ではなく、**アセットを差し替えられる形に
テスト側を組む**（`web.FS()` の戻り値を使う経路をテスト用の `fs.FS` で覆う）。
それが `spa.go` の変更なしにできない場合は、`spa.go` に
テストから差し替えられる小さな受け口を足してよい（担当ファイルの中なので可）。

## 完了条件（**実行すれば真偽が決まるもの。自己申告しない**）

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l $(git diff --name-only origin/main -- '*.go')
```

`gofmt -l` が**何も出力しない**のが合格。加えて:

```bash
# 担当外のファイルを触っていないこと（何も出力しなければ合格）
git diff --name-only origin/main | grep -v -e '^internal/server/preview\.go$' -e '^internal/server/spa\.go$' \
  -e '^internal/server/preview_test\.go$' -e '^internal/server/spa_test\.go$'

# #40: 1 行以上返れば合格
grep -n 'func worktreeMatchesNewSide\|NewStart' internal/server/preview.go

# #88 / #89: 1 行以上返れば合格
grep -n 'StatusNotFound' internal/server/spa.go

# 新規テストが実際に走っていること（1 行以上返れば合格）
go test ./internal/server/ -run 'TestSpaHandler|TestPreviewResolve' -v | grep '^--- PASS'
```

PR は 3 本（#40 は `Fixes`、#88 と #89 は `Refs`）。

## やること・やらないこと

- **push と PR 作成まで行う。マージはしない。**
- 担当外は触らない。特に **`web/` は 1 バイトも触らない**（G4 のレーンが使用中）。
  `web/dist/` は COMMON.md のとおり絶対にコミットしない。
- 見つけた問題は自分で直さず、最終報告に書く。
- 判断に迷って止まらない。既定を決めて進み、決めた内容と理由を報告に書く。
- **`server.go` や `markdown.ts` が空くのを待たない。** 待つくらいなら `Refs` で出す。

## 完了報告

COMMON.md の報告書式に加えて、先頭に次の 4 行をこの綴りのまま書く:

```
slug: srv-preview
branch: gogo/issue-40 / gogo/issue-88 / gogo/issue-89
worktree: /home/user/wt/srv-preview
commit: <PR ごとの commit を全部列挙する>
```
