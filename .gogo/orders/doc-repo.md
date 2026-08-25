slug: doc-repo

# 指示文 DOC-02 / doc-repo — 参加の仕方を書いた文書一式を作る

優先度: **#129 は P1、それ以外は P3** / 期限: **#129 は本サイクル最優先、他は本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/doc-repo`
- ブランチ: issue ごとに `gogo/issue-<N>`。**毎回 `origin/main` から切り直す**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 4 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/doc-repo origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

**この順番で進めること。順番には理由がある。**

| 順番 | issue | タスク ID | 内容 | README を触るか |
|---|---|---|---|---|
| 1 本目 | #129 | `t-86bb4a` | `SECURITY.md`（**P1**） | 触らない |
| 2 本目 | #130 | `t-5f842c` | issue / PR テンプレート・`.editorconfig`・行動規範 | 触らない |
| 3 本目 | #108 | `t-f493f3` | `CHANGELOG.md` と `.tagpr` の changelog 有効化 | 触らない |
| 4 本目 | #106 | `t-faf858` | `CONTRIBUTING.md` と README の Development 節 | **触る（最後に回す理由）** |

**#106 を最後に回すのは、README を doc-readme レーン（#107 / #132）と共有しているため。**
先に README を触らない 3 本を出し切ることで、doc-readme のマージを待たずに手を動かし続けられる。
#106 に着手する時点で `git fetch -q origin main` してから `origin/main` を切り直せば、
doc-readme が入っていればその上に載る。**入っていなくても待たない。**
あなたが触る README の領域は `## Development` 節の中だけで、doc-readme の編集領域
（イントロ・`## Install`・477 行目・508 行目・522/523 の境界）とは 1 行も重ならないので、
どちらが先にマージされても 3-way マージで衝突しない。

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn）で各 issue の本文を必ず読んでから直す。

## 2. 触ってよいファイル

**この一覧だけ。** ここから 1 バイトも出ない。

| ファイル | issue |
|---|---|
| `SECURITY.md`（新規） | #129 |
| `.github/ISSUE_TEMPLATE/bug_report.yml`（新規） | #130 |
| `.github/ISSUE_TEMPLATE/feature_request.yml`（新規） | #130 |
| `.github/ISSUE_TEMPLATE/config.yml`（新規） | #130 |
| `.github/PULL_REQUEST_TEMPLATE.md`（新規） | #130 |
| `.editorconfig`（新規） | #130 |
| `CODE_OF_CONDUCT.md`（新規） | #130 |
| `CHANGELOG.md`（新規） | #108 |
| `.tagpr`（既存・1 行追加） | #108 |
| `CONTRIBUTING.md`（新規） | #106 |
| `README.md` の `## Development` 節の中だけ | #106 |

### 触ってはいけないもの（明示）

- **Go のコードは 1 バイトも変えない。** このレーンは `.go` を 1 本も追加しない。
  `git diff --name-only origin/main -- '*.go'` が**常に空**であること。
- `.github/workflows/` — CI レーン（G10 / #71 #102 #126 #131）の持ち物。**触らない。**
  `.github/ISSUE_TEMPLATE/` と `.github/PULL_REQUEST_TEMPLATE.md` だけがあなたの担当。
- `README.md` の `## Development`（現 561〜573 行目）**以外の全行**。
  特に `## Install`（29–46）・`## Files and ports`（546–559）・
  `### Folding away what nobody reads`（188–222）は他レーンの持ち物。
- `Taskfile.yml`、`go.mod`、`go.sum`、`web/`、`skills/`、`cmd/`、`internal/`、`docs/`。
- `CODEOWNERS` は**作らない。** issue #130 自身が「単一メンテナのリポジトリではほぼ no-op で
  後回しでよい」と書いている。作らないことを報告に書く。

## 3. 1 本目 — #129 `gogo/issue-129`（**P1。ここから始める**）

### 作るもの

`SECURITY.md` を新規作成する。**このファイル 1 本だけ。**

issue #129 が求めているのは 4 つ。全部書く。

1. **どこへ報告するか。** GitHub の private vulnerability reporting を第一の窓口にする
   （Settings → Security で有効化する運用。メールアドレスは腐るので書かない）。
   報告先の URL は `https://github.com/tenntenn/sbnn/security/advisories/new` の形。
   **有効化そのものはリポジトリ設定の操作であり、あなたはやらない。**
   「有効化が要る」ことを PR 本文に 1 行書いて、メンテナに渡す。
2. **何が scope 内か。** ここが sbnn 固有で、書く価値があるところ。
   脅威モデルを自分の言葉で書く: **sbnn は CLI を実行する人間は信頼するが、
   diff のテキスト・diff が名指しする作業ツリーのパス・利用者がたまたま開いている
   任意の Web ページは信頼しない。**
   scope 内の例として、根拠をコードから引いて書くこと（**必ず自分で読んでから書く**）:
   - `internal/source/` の封じ込め検査を**すり抜ける**経路
     （`grep -rn 'AbsPath' internal/source/*.go` で実体を読む）
   - Markdown preview の sanitize 迂回による XSS
   - CSRF 判定（`internal/server/server.go` の `crossOrigin`）の迂回によって、
     外部ページから hook（= シェルコマンド）を登録できる経路
3. **何が scope 外か。**
   - `--dangerously-allow-remote-access` が名前どおりに振る舞うこと
   - 利用者自身が登録した hook がコマンドを実行すること
   - 「diff が任意のパスを名指しできる」こと自体（封じ込め検査までは仕様）
4. **どのバージョンが直るか。** 今は `dev` しか無い（`cat version/version.go` で確かめる）。
   リリースが出たら最新のみ、と書く。#101 でリリースができたら更新が要る旨を 1 行添える。

**書いてよいのは、自分でコードを読んで確かめた事実だけ。** issue の本文を写経しない。
存在しない連絡先・存在しない SLA・存在しない bug bounty は書かない。

### 完了条件

```bash
test -f SECURITY.md
grep -n 'security/advisories/new' SECURITY.md
grep -ni 'dangerously-allow-remote-access' SECURITY.md
grep -ni 'out of scope\|Out of scope' SECURITY.md
# 根拠にしたコードが実在することを自分で確認した証跡
grep -rn 'crossOrigin' internal/server/server.go | head -1
grep -rn 'func AbsPath' internal/source/ | head -1
# 差分は 1 ファイルだけ
git diff --name-only origin/main            # => SECURITY.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

## 4. 2 本目 — #130 `gogo/issue-130`

### 作るもの

1. **`.github/ISSUE_TEMPLATE/bug_report.yml`**（GitHub の issue form 形式）
   issue #130 が「sbnn 固有で一番効く」と言っているのがこれ。
   **必須項目として次を聞くこと**（理由も issue に書いてある）:
   - sbnn のバージョン（`sbnn --version` の出力）
   - OS（state の置き場が OS で変わるため。#104）
   - diff を作ったコマンド
   - **diff 本体**。sbnn の入力はそのテキストが全部であり、パースのバグは
     diff が無いと再現しない。「必要なら伏せ字にしてよい」と 1 行添える。
   - 期待した挙動と実際の挙動
2. **`.github/ISSUE_TEMPLATE/feature_request.yml`** — 短くてよい。
3. **`.github/ISSUE_TEMPLATE/config.yml`** — `blank_issues_enabled: true`。
   フォームを迂回したい人を締め出さない。
4. **`.github/PULL_REQUEST_TEMPLATE.md`**
   チェックボックスで、contributor が推測できない 2 つのルールを持たせる:
   - [ ] `task lint` を走らせた
   - [ ] `web/src` を触ったなら `task web` を走らせて `web/dist` をコミットした
   根拠は `Taskfile.yml` の `lint` タスクと `AGENTS.md` の記述。**自分で両方読んでから書く。**
5. **`.editorconfig`**
   リポジトリの実態に合わせる。**推測しないで測ること**:
   ```bash
   grep -c $'\t' cmd/root.go
   sed -n '1,5p' web/src/*.ts* 2>/dev/null | cat -A | head -5
   sed -n '1,5p' Taskfile.yml | cat -A | head -5
   ```
   Go はタブ、TypeScript / CSS は 2 スペース、YAML は 2 スペース、というのが issue の主張。
   **自分で確かめて、違っていたら実測のほうに合わせ、その旨を報告に書く。**
   `insert_final_newline` と `trim_trailing_whitespace` も入れる。
   ただし `web/dist` と `web/node_modules` は除外する（生成物）。
6. **`CODE_OF_CONDUCT.md`** — Contributor Covenant 2.1 の標準文面。
   連絡先は #129 で決めた窓口と食い違わせない。
   **メールアドレスを新しく発明しない。** GitHub の private reporting か、
   リポジトリの issue を指す。決めた内容と理由を報告に書く。

### 完了条件

```bash
test -f .github/ISSUE_TEMPLATE/bug_report.yml
test -f .github/ISSUE_TEMPLATE/feature_request.yml
test -f .github/ISSUE_TEMPLATE/config.yml
test -f .github/PULL_REQUEST_TEMPLATE.md
test -f .editorconfig
test -f CODE_OF_CONDUCT.md
# issue フォームが YAML として妥当（GitHub に出す前に自分で検査する）
python3 -c "import yaml,sys;[yaml.safe_load(open(f)) for f in ['.github/ISSUE_TEMPLATE/bug_report.yml','.github/ISSUE_TEMPLATE/feature_request.yml','.github/ISSUE_TEMPLATE/config.yml']];print('yaml ok')"
# バグ報告フォームが diff 本体を聞いている
grep -ni 'diff' .github/ISSUE_TEMPLATE/bug_report.yml
# PR テンプレートが 2 つのルールを持っている
grep -n 'task lint' .github/PULL_REQUEST_TEMPLATE.md
grep -n 'web/dist' .github/PULL_REQUEST_TEMPLATE.md
# workflows を触っていない
git diff --name-only origin/main | grep '^\.github/workflows/'   # 何も返らないこと
git diff --name-only origin/main -- '*.go'                       # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

`python3 -c "import yaml"` が入っていなければ、代わりに
`ruby -ryaml -e 'YAML.load_file(...)'` でも `go run` の小物でもよいが、
**何らかの YAML パーサに通してから PR を出すこと。** 目視は検証ではない。

## 5. 3 本目 — #108 `gogo/issue-108`

### 変えるもの

1. **`.tagpr` に `changelog = true` を足す。** 現在の中身:
   ```ini
   [tagpr]
       vPrefix = true
       releaseBranch = main
       versionFile = version/version.go
   ```
   **既存の 3 行は変えない。1 行足すだけ。** インデントは既存に合わせる（タブ）。
2. **`CHANGELOG.md` を新規作成する。**
   - 冒頭に、これ以降は tagpr がリリース PR で追記する旨を書く。
   - **`## Unreleased` の節を置く。** 中身は現時点の事実だけ。
     **過去のリリース履歴を捏造しない。** タグが無いことを
     `git tag --list | head` で確かめてから書く。
   - issue #108 が本題にしている「format changes」の節を置く。
     sbnn は 3 つの永続フォーマットを持ち、それぞれ互換性の約束をしている:
     - セッションファイル（`version` フィールドと `persisted{Version, Seq, Groups}`）
     - `reviews.jsonl`（`sbnn reviews --help` が「フィールドは足されるが改名されない」と約束している）
     - `sbnn export` のペイロードに埋まる `version`
     **3 つそれぞれをコードで確認してから書く**:
     ```bash
     grep -rn 'persisted' internal/store/*.go | head
     go build -o /tmp/sbnn-ch . && /tmp/sbnn-ch reviews --help | grep -n -i 'renamed\|added'
     grep -rn 'Version' internal/export/export.go | head
     ```
     「どのバージョンでどのフィールドが増えたか」を今後ここに書く、という**運用の宣言**を
     見出しとして置く。**過去分を埋めない**（記録が無いので埋めれば嘘になる）。

### 完了条件

```bash
grep -n 'changelog' .tagpr
git diff origin/main -- .tagpr | grep -c '^-'   # => 1（--- の行だけ。既存行を消していない）
test -f CHANGELOG.md
grep -n '^## Unreleased' CHANGELOG.md
grep -ni 'reviews.jsonl' CHANGELOG.md
grep -ni 'export' CHANGELOG.md
git tag --list | head        # 空であることを確認した上で、過去リリースを書いていない
git diff --name-only origin/main -- '*.go'   # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

**`## Install` 節からリリースへのリンクを張る話（issue #108 の最後の 1 文）は
この PR では扱わない。** `## Install` は doc-readme（#132）の持ち物で、
リリース自体もまだ存在しない（#101）。**扱わないことを報告に書く。**

## 6. 4 本目 — #106 `gogo/issue-106`

**着手前に必ず:**

```bash
cd /home/user/wt/doc-repo
git fetch -q origin main
git checkout -q -B gogo/issue-106 origin/main
grep -n '^## Development' README.md   # 行番号を控える。ここから次の '^## ' までがあなたの領域
```

### 作るもの / 変えるもの

1. **`CONTRIBUTING.md` を新規作成する。** issue #106 が挙げた項目を全部入れる:
   - **前提**: Go の最低バージョン（`go.mod` の `go 1.24.0` を自分で読んで書く）、
     aqua（`aqua install` で `task` が入る）、pnpm。
   - **`task lint` を build / test と並べて書く。** これが issue の 1 点目。
     `Taskfile.yml` の `lint` は `go vet ./...` と `gofmt -l` の検査を走らせる。
     **CI が無いので、走らせる責任は書いた人にある**ことを明記する（#71）。
   - **`web/dist` を手で作り直すルールを `AGENTS.md` から昇格させる。** これが 2 点目で、
     issue が「リポジトリで最も意外なルールが、人間の読まない場所にしか無い」と言っている核心。
     `AGENTS.md` の当該記述を自分で読んでから、人間向けの言葉で書き直す
     （`web/src` を触ったら `task web` を走らせて `web/dist` をコミットする）。
     **`AGENTS.md` 自体は編集しない**（担当外）。
   - **良い PR の形。** コミット件名の実際の書き方を、憶測ではなく
     `git log --oneline -20` を自分で走らせて読み取ってから書く。
   - **変更したものをどう動かすか**（`task dev` の説明）。
2. **README の `## Development` 節を短くして `CONTRIBUTING.md` へのリンクにする。**
   issue の言葉では「3 行とリンク」。
   - **`## Development` の見出し行から、次の `## License` の直前までだけを書き換える。**
     その外は 1 行も触らない。
   - `task build` / `task test` / `task dev` の 3 行に加えて **`task lint` を必ず載せる**
     （抜けていたことが issue の 1 点目そのもの）。
   - 詳細は `CONTRIBUTING.md` へ、というリンクを 1 行。

### 完了条件

```bash
test -f CONTRIBUTING.md
grep -n 'task lint' CONTRIBUTING.md
grep -n 'task lint' README.md
grep -n 'web/dist' CONTRIBUTING.md
grep -n '1\.24' CONTRIBUTING.md
grep -n 'CONTRIBUTING.md' README.md
# README は Development 節の中だけを触った
git diff origin/main -- README.md | grep -n '^[-+].*XDG_STATE_HOME'   # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*go install github'# 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*Folding away'     # 何も返らないこと
git diff origin/main -- README.md | grep -n '^[-+].*## License'       # 何も返らないこと
git diff --name-only origin/main -- '*.go'                            # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

**さらに、書いた手順を実際に実行すること**（この指示文の中で一番落ちやすいのがここ）:

```bash
which task || aqua install       # task が無ければ入れる
task lint  ; echo "task lint exit=$?"
task test  ; echo "task test exit=$?"
task build ; echo "task build exit=$?"
```

`task build` は `web/dist` を作り直す。**COMMON.md のとおり、
`git checkout -- web/dist` で捨ててからコミットすること。** `web/dist` は絶対にコミットしない。

**3 つの終了コードを PR 本文の `## Verification` にそのまま貼ること。**
どれかが動かなかったなら、**CONTRIBUTING.md にそう書かない。**
動かない手順を書くのは、この issue が直そうとしている問題そのもの。
動かなかった事実と、その環境の理由を報告の「見送り / 疑義」に書く。

## 7. 全 PR 共通の検証（COMMON.md の「検証」に上乗せ）

```bash
go build ./... && go vet ./... && go test ./...
git diff --name-only origin/main -- '*.go'    # 常に何も返らないこと
git diff --name-only origin/main | grep '^web/dist/'   # 何も返らないこと
git status --short                             # 未追跡の残骸が無いこと
```

**「Go のコードは 1 バイトも変えない」の解釈:** このレーンは `.go` を新規追加も含めて
1 本も触らない。テストも足さない。**足すべき Go のテストは #114 / #123 の担当である。**
COMMON.md の「テストを必ず足す」は、テストの書けない文書追加には適用されない。
**その場合は PR 本文に「なぜ書けないか」を 1 行書く**（COMMON.md の規定どおり）。
文書の正しさは §6 のように**手順を実際に走らせること**で担保する。

## 8. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
  特に行動規範の連絡先、`.editorconfig` の値、CHANGELOG の初期内容は、
  **確認を上げずに自分で決めて、理由を書く。**
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
- **doc-readme のマージを待たない。** #106 に着手する時点で入っていなければ、
  `## Development` 節の中だけを編集してそのまま出す。編集領域が重ならないので衝突しない。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-86bb4a --status running --progress 30
  gogodash task log --id t-86bb4a --message "<何が起きたか 1 行>"
  gogodash task set --id t-86bb4a --status done --progress 100 --result "PR #<番号>"
  ```
  タスク ID は §1 の表のとおり（#129=`t-86bb4a` / #130=`t-5f842c` / #108=`t-f493f3` / #106=`t-faf858`）。

## 9. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : doc-repo
worktree : /home/user/wt/doc-repo
branch   : gogo/issue-129 / gogo/issue-130 / gogo/issue-108 / gogo/issue-106
commit   : <各ブランチの短縮 SHA を 1 行ずつ>
```

**push と PR 作成まで行う。マージはしない。**
