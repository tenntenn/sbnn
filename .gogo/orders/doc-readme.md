slug: doc-readme

# 指示文 DOC-01 / doc-readme — README を読める形にし、その主張を機械で検査できるようにする

優先度: **P3** / 期限: **本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/doc-readme`
- ブランチ: issue ごとに `gogo/issue-<N>`。**毎回 `origin/main` から切り直す**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 3 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/doc-readme origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

| 順番 | issue | タスク ID | 内容 |
|---|---|---|---|
| 1 本目 | #132 | `t-06c82c` | Install 節を「まず 1 コマンド」の形に直す |
| 2 本目 | #107 | `t-9e770f` | 目次・見出しの衝突解消・コマンド／フラグ表 |
| 3 本目 | #123 | `t-2cbe13` | README の主張をコードに突き合わせる Go テストを足す |

**この順番で進めること。** #132 が `## Install` の本文、#107 がイントロ末尾と後半、と
編集領域を分けてあるので、この順で別ブランチにしても衝突しない。

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn）で各 issue の本文を必ず読んでから直す。

## 2. 触ってよいファイル

**この 2 つだけ。** ここから 1 バイトも出ない。

| ファイル | 誰の担当か |
|---|---|
| `README.md` | ただし**下の「README の中で触ってよい領域」に限る** |
| `docs/doccheck/readme_test.go` | 新規作成。あなたの専有 |

### README の中で触ってよい領域

README は 3 つのレーンが別々の節を持っている。**節の外の行は 1 行も変えない。**

| 領域 | 現在の行 | 持ち主 |
|---|---|---|
| イントロ（1〜8 行目）とスクリーンショット行の直前 | 1–9 | **あなた**（目次の挿入先） |
| `## Install` の本文 | 30–46 | **あなた**（#132） |
| `### Installing it` の見出し行 | 477 | **あなた**（#107 の改名） |
| `## Reviewing changes` の見出し行 | 508 | **あなた**（#107 の改名） |
| `### Checking that it took` の直後・`## How the Markdown preview works` の直前 | 522/523 の境界 | **あなた**（#107 の新設節の挿入先） |
| `### Folding away what nobody reads` | 188–222 | doc-collapse（#52）。**触らない** |
| `## Files and ports` | 546–559 | G1 の paths レーン（#104）。**触らない** |
| `## Development` | 561–573 | doc-repo（#106）。**触らない** |

`## Files and ports` の表は macOS / Windows で嘘になっている（#104）が、**あなたは直さない。**
気づいた点は報告に書く。

### 触ってはいけないもの（明示）

- **Go の既存ファイルは 1 バイトも変えない。** 新規に足してよい `.go` は
  `docs/doccheck/readme_test.go` の 1 本だけ。
- `cmd/` 配下の既存ファイル、`internal/` 配下、`web/`、`skills/`、`Taskfile.yml`、
  `go.mod`、`go.sum`、`.github/`、`.tagpr` — 全部触らない。
- `web/dist` はコミットしない（COMMON.md）。今回 `web/` は一切触らないので、
  差分に `web/` が出たらそれは事故。

## 3. 1 本目 — #132 `gogo/issue-132`

### 変えるもの

`README.md` の `## Install` の本文（現 30〜46 行目）だけを書き直す。
**`## Install` という見出し行そのものは変えない。**

issue #132 の言い分は「sbnn 本体が 1 行、任意の依存 mo が 6 行あり、
簡単な導線があるのは mo の方だ」。ここを直す。

やること:

1. **最初のコードブロックを sbnn 自身の導入にする。** 現状 `go install` が
   最初に来ているのは正しい。問題は mo の説明が同じ節に居座って主役を食っていること。
2. **`go install` が Go 1.24 以上を要求することを書く。** 根拠は `go.mod` の
   `go 1.24.0`（自分で `head -3 go.mod` を実行して確かめてから書く）。
   今これはどこにも書かれておらず、古いツールチェインの利用者はモジュールエラーだけを見る。
3. **mo の導入手順を `## Install` から出す。** 移動先は
   `## How the Markdown preview works`（現 523 行目）の節の末尾。
   そこは preview の話をしている節なので、mo の話はそこが正しい家である。
   **移動であって書き換えではない。** 現在の mo 4 段落（「sbnn renders the Markdown
   preview itself…」から「its published module does not carry the embedded frontend.」まで）
   を、文面をできるだけ保ったまま移す。
   - 注意: `## How the Markdown preview works` の**節の末尾**に足すこと。
     直後の `## Files and ports`（546 行目）は他レーンの持ち物なので、
     `## Files and ports` の見出し行より前で止める。
4. **バイナリ配布・Homebrew・install.sh・aqua・Scoop は書かない。**
   それらはリリースにバイナリが載ってから（#101）の話で、今書くと**存在しない導線を
   案内することになる**。代わりに `## Install` の末尾に 1 行だけ、
   「今は `go install` が唯一の導線である」と現状を明示する短い文を置く。
   **「近日対応」「予定」といった約束は書かない。**

### 完了条件（実行して真偽が決まるもの）

worktree のルートで:

```bash
# Go 1.24 の要求が Install 節に書かれた
grep -n '1\.24' README.md
# mo の brew 行が Install 節から消え、preview の節へ移った
awk '/^## Install/{f=1} /^## Usage/{f=0} f' README.md | grep -c 'brew install'   # => 0
awk '/^## How the Markdown preview works/{f=1} /^## Files and ports/{f=0} f' README.md | grep -c 'brew install'  # => 1 以上
# 他レーンの領域を触っていない
git diff origin/main -- README.md | grep -n '^[-+].*Folding away'      # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*XDG_STATE_HOME'    # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*task build'        # 何も返らないこと
# Go を 1 バイトも変えていない
git diff --name-only origin/main -- '*.go'                             # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

`README.md` 以外が `git diff --name-only origin/main` に出たら不合格。

## 4. 2 本目 — #107 `gogo/issue-107`

### 変えるもの

1. **目次を入れる。**
   挿入位置は**スクリーンショット行（`![sbnn showing a diff on the left …](docs/screenshot.png)`）の直前**。
   `## Install` の直前ではない。**離れた場所に置くのは意図的で、他レーンの編集と行が
   近づかないようにするため。ここ以外に置かない。**
   - 形式は GitHub の Markdown で動く相対リンクのリスト。
     `## ` と `### ` の 2 階層を出す。
   - **アンカーは推測で書かない。** GitHub のアンカー規則（小文字化・空白をハイフン・
     記号を落とす）で自分で導出し、書いたあと下の完了条件のスクリプトで全リンクを検査する。
2. **衝突している見出しを改名する。** issue が挙げているのはこの 2 組:
   - 508 行目 `## Reviewing changes` → `## Reviewing sbnn's own changes`
   - 477 行目 `### Installing it` → `### Installing the skill`
   **見出し行だけを変える。節の本文は変えない。**
   改名したら、README 内でその見出しを指しているリンクがあれば直す
   （`grep -n '#reviewing-changes\|#installing-it' README.md` で確認）。
3. **コマンド／フラグの参照表を足す。**
   新しい節 `## Command and flag reference` を、
   **`### Checking that it took` の節の末尾（`## How the Markdown preview works` の直前）** に挿入する。
   - 中身は**実際に `sbnn --help` と各サブコマンドの `--help` を実行して**作る。推測で書かない。
     ```bash
     go build -o /tmp/sbnn-ref . && /tmp/sbnn-ref --help && for c in comment comments export hook reviews skill submit wait; do /tmp/sbnn-ref $c --help; done
     ```
   - 表はサブコマンド 1 行 + 主要フラグ。**全フラグを写経しない。**
     冒頭に「完全な一覧は `sbnn <command> --help` にある」と 1 行書き、
     README が `--help` の存在を案内していない（issue の 3 点目）状態を解消する。
   - **1 本目（#132）で mo の段落を `## How the Markdown preview works` の末尾へ移している。**
     このブランチは `origin/main` から切るので mo の段落はまだ無い。
     挿入位置は `## How the Markdown preview works` の**見出し行の直前**とし、
     `### Checking that it took` の本文の最後の行との間に空行を 1 行だけ置く。
   - 目次にもこの新しい節を載せる。

### 完了条件

```bash
# 目次がある
grep -n '^## Contents' README.md
# 目次のアンカーが全部実在する（1 つでもずれたら不合格。何も出力しないのが合格）
python3 - <<'PY'
import re,sys
t=open('README.md').read()
def anchor(h):
    a=h.strip().lower()
    a=re.sub(r'[^\w\s-]','',a)
    return re.sub(r'\s+','-',a)
have={anchor(m.group(2)) for m in re.finditer(r'^(#{1,6})\s+(.*)$',t,re.M)}
bad=[l for l in re.findall(r'\]\(#([^)]+)\)',t) if l not in have]
if bad: print("MISSING ANCHORS:",bad); sys.exit(1)
PY
# 衝突していた見出しが消え、新しい名前になった
grep -n '^## Reviewing sbnn' README.md
grep -n '^### Installing the skill' README.md
grep -c '^## Reviewing changes$' README.md   # => 0
grep -c '^### Installing it$' README.md      # => 0
# フラグ参照の節がある
grep -n '^## Command and flag reference' README.md
grep -n 'sbnn .*--help' README.md
# 他レーンの領域を触っていない
git diff origin/main -- README.md | grep -n '^[-+].*Folding away'    # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*XDG_STATE_HOME'  # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*task build'      # 何も返らないこと
git diff --name-only origin/main -- '*.go'                           # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

**表に書いたフラグが実在することを、上のビルドした `/tmp/sbnn-ref` で 1 つずつ確かめてから
PR を出すこと。** 存在しないフラグを表に載せるのは、この issue が直そうとしている問題そのもの。

## 5. 3 本目 — #123 `gogo/issue-123`

### 作るもの

`docs/doccheck/readme_test.go` を新規作成する。**これがこの PR の唯一の変更。**
README はこの PR では 1 行も変えない。

置き場所の理由（迷わないように決めてある。変更しないこと）:
`cmd/` と `internal/` は他のレーンが同時に触っているので新しいファイルを置けない。
`docs/` は空いている。Go のファイルが 1 本も無いディレクトリに `_test.go` だけを置いても
`go build ./...` / `go vet ./...` / `go test ./...` / `gofmt` はすべて通ることを確認済みである。
パッケージ名は `doccheck`。

### 何を検査するか

issue #123 が挙げた検査のうち、**次の 3 つだけを実装する。**

1. **README の fenced code block 内の `sbnn …` 呼び出しが全部実在する。**
   - サブコマンドが実在すること、`--flag` が**そのサブコマンド**に登録されていること。
   - 実装方法: テストの中で `go build -o <t.TempDir()>/sbnn <リポジトリルート>` を 1 回だけ走らせ、
     `sbnn --help` と各サブコマンドの `--help` を実行してコマンド名とフラグ名を集める。
     ネットワークもブラウザも要らない。ビルドは実測 1 秒未満である。
   - `exec.LookPath("go")` が失敗したら `t.Skip`。それ以外で落ちたら `t.Fatal`。
   - **必ず踏む罠（先に潰しておくこと）**: README 424 行目付近に
     `sbnn --label rev=$(git rev-parse --short HEAD)` がある。`--short` は git のフラグであって
     sbnn のものではない。**検査の前に `$( … )` のコマンド置換を丸ごと取り除くこと。**
     取り除かないと偽陽性が 1 件出る。
   - パイプで繋がれた行（`git diff | sbnn …`）は `|` で分割し、`sbnn` で始まる区間だけを見る。
   - 現在の README で**この検査が通る**ことを確認済みである（62 件の呼び出し、未知フラグ 0 件）。
     落ちたら、それはテストの書き方の問題であって README の問題ではない可能性が高い。
2. **README が `cmd/util.go` の環境変数の const を全部言及している。**
   - `cmd/util.go` に `TargetEnv = "SBNN_TARGET"` と `HistoryEnv = "SBNN_HISTORY"` がある。
     ソースを `go/parser` で読むか、単純に `cmd/util.go` を読んで `"SBNN_[A-Z_]+"` を拾い、
     README に各名前が現れることを確かめる。現在どちらも README にある（確認済み）。
3. **README に空の fenced code block が無い。** ` ``` ` の直後に閉じフェンスが来る箇所を検出する。
   現在 0 件（確認済み）。skills 側で同じ事故が起きている（#109）ので、README 側も固定しておく。

### 実装しないもの（明示。書いたら不合格）

- **paths の表の検査。** `## Files and ports` は G1 の paths レーン（#104）が同時に直している。
  今の README は macOS / Windows で嘘なので、この検査を書くと**テストが赤くなる。** 書かない。
- **`make` / `Makefile` の残骸の grep。** `internal/server/spa.go` と `web/web.go` に
  まだ残っている（#36 / #37、G1 の担当）。書くと**テストが赤くなる。** 書かない。
- **exit code の検査。** 定数は `cmd` パッケージの非公開で、外から読めない。書かない。
- **SKILL.md の検査。** それは #114（skill-split の担当）。README だけを見る。

上の 3 つを「なぜ今は入れないか」を、テストファイルの冒頭コメントに英語で 1〜2 行ずつ書き、
対応する issue 番号（#104 / #36 / #37 / #114）を添えること。**後から入れる人が迷わないようにする。**

### 完了条件

```bash
gofmt -l docs/doccheck/readme_test.go     # 何も返らないこと
go build ./... && go vet ./... && go test ./...
go test ./docs/doccheck/ -run . -v        # 3 つのテストが PASS
# README を触っていない
git diff --name-only origin/main          # docs/doccheck/readme_test.go の 1 行だけ
# 既存の Go を触っていない
git diff --name-only origin/main -- '*.go' | grep -v '^docs/doccheck/readme_test\.go$'   # 何も返らないこと
```

**さらに、テストが本当に効いていることを自分で確かめる**（自己申告にしない）:

```bash
# README に存在しないフラグを一時的に混ぜてテストが落ちることを見る
cp README.md /tmp/README.bak
printf '\n```console\n$ sbnn comments --no-such-flag\n```\n' >> README.md
go test ./docs/doccheck/ ; echo "落ちるはず: exit=$?"
cp /tmp/README.bak README.md
go test ./docs/doccheck/ ; echo "通るはず: exit=$?"
git diff --stat README.md    # 何も返らないこと（元に戻っている）
```

**この 2 回の実行結果（終了コード）を PR 本文の `## Verification` にそのまま貼ること。**
「テストを書いた」ではなく「壊したら落ちることを見た」を根拠にする。

## 6. 全 PR 共通の検証（COMMON.md の「検証」に上乗せ）

```bash
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')   # 何も返らないこと
git diff --name-only origin/main -- '*.go' | grep -v '_test\.go$'   # 何も返らないこと
git status --short                                        # 未追跡の残骸が無いこと
```

**「Go のコードは 1 バイトも変えない」の解釈はこれで固定する:**
既存の `.go` ファイルは 1 バイトも変えない。新規に足してよいのは `_test.go` だけで、
今回それは `docs/doccheck/readme_test.go` の 1 本のみ。実装（非テスト）の `.go` は
新規作成も禁止。**迷ったら足さない。**

## 7. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
  ここで確認を上げてよいのは、無いと物理的に進めないものだけ。
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
  特に `## Files and ports` の嘘（#104）、`make` の残骸（#36 / #37）は
  **見つけても直さないこと。他のレーンが同時に直している。**
- **書いた手順は実際に実行して通ることを確かめる。** README に新しいコマンドを書いたら、
  そのコマンドを自分で走らせる。走らせていないものは書かない。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-06c82c --status running --progress 30
  gogodash task log --id t-06c82c --message "<何が起きたか 1 行>"
  gogodash task set --id t-06c82c --status done --progress 100 --result "PR #<番号>"
  ```
  タスク ID は §1 の表のとおり（#132=`t-06c82c` / #107=`t-9e770f` / #123=`t-2cbe13`）。

## 8. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : doc-readme
worktree : /home/user/wt/doc-readme
branch   : gogo/issue-132 / gogo/issue-107 / gogo/issue-123
commit   : <各ブランチの短縮 SHA を 1 行ずつ>
```

**push と PR 作成まで行う。マージはしない。**
