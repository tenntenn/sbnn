slug: doc-api

# 指示文 DOC-05 / doc-api — HTTP API の参照文書を作る

優先度: **P3** / 期限: **本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/doc-api`
- ブランチ: `gogo/issue-148`。**`origin/main` から切る**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 1 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/doc-api origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

| issue | タスク ID | 内容 |
|---|---|---|
| #148 | `t-650017` | `/_/api/` が文書化も版付けもされていない |

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn, issue_number=148）で本文を必ず読んでから書く。

## 2. 触ってよいファイル

**この 1 つだけ。** ここから 1 バイトも出ない。

| ファイル | 備考 |
|---|---|
| `docs/api.md` | 新規作成。**あなたの専有** |

### 触ってはいけないもの（明示）

- **Go のコードは 1 バイトも変えない。** このレーンは `.go` を新規追加も含めて 1 本も触らない。
  `git diff --name-only origin/main -- '*.go'` が**常に空**であること。
- `README.md` — 触らない（doc-readme / doc-repo / doc-collapse の持ち物）。
  特に `## Files and ports` は G1 の paths レーン（#104）が同時に直している。**触らない。**
- `docs/doccheck/` — doc-readme（#123）が同じ `docs/` の下に別のファイルを作る。**触らない。**
- `docs/screenshot.png` — 触らない。
- `skills/` `cmd/` `internal/` `web/` `.github/` `Taskfile.yml` `go.mod` `go.sum` — 触らない。

## 3. 作るもの

`docs/api.md` を新規作成する。**このファイル 1 本だけがこの PR の変更。**

### 何を書くか

issue #148 の Expected は 3 点。全部書く。

#### (1) エンドポイントの一覧と、要求／応答の形

エンドポイントは `internal/server/server.go` の 150〜174 行目にルーティングが
リテラルで並んでいる。**自分で読んで、そこから拾うこと。** issue の本文を写経しない。

```bash
sed -n '145,180p' internal/server/server.go
```

**実測では 24 本ある**（`/_/api/…` が 23 本、`/_/events` の SSE が 1 本。
`DELETE /_/api/groups/{group}/hooks` と `.../hooks/{id}` のように、
同じハンドラに 2 つのパターンが向いているものがあるので、
「ハンドラの数」ではなく「登録されたパターンの数」で数えること）。
**この数も自分で数え直して確かめること。** issue の本文は「20 endpoints」と
言っているが、**コードのほうが正しい。** 食い違いは報告に 1 行書く。

各エンドポイントについて書くもの:

- メソッドとパス（`{group}` `{diff}` `{file}` `{id}` のパラメータを含む）
- 何をするか 1 行
- 要求のボディの形（あるもののみ）
- 応答のボディの形
- 主な状態コード

要求／応答の型は `internal/server/` のハンドラと `internal/model/` に定義がある。
**必ずソースを読んでから書く**:

```bash
grep -n 'type .*Request struct\|type .*Response struct' internal/server/*.go
sed -n '140,300p' internal/model/model.go
```

**特に `model.Comment` の `omitempty` を落とさないこと。**
`author` `question` `suggestions` は値が無いとキーごと消える。
これを取り違えたのが #110 で、この文書はまさにそれを繰り返さないために作る。
**どのキーが常にあり、どれが条件付きかを、表の中で区別できる形にすること。**

実際に叩いて応答を見て、書いたものと合っているか確かめること
（サーバは終わったら**必ず落とす**。並列で動いている他のレーンとポートを取り合わない）:

```bash
go build -o /tmp/sbnn-api .
printf 'diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1 @@\n+x\n' | /tmp/sbnn-api --target t148 --no-open
/tmp/sbnn-api --status --json | python3 -m json.tool | head -30
curl -s localhost:6280/_/api/groups/t148 | python3 -m json.tool | head -40
/tmp/sbnn-api comment a.go:1 -m hi --target t148
curl -s localhost:6280/_/api/groups/t148/comments | python3 -m json.tool
/tmp/sbnn-api --clear --target t148
/tmp/sbnn-api --shutdown
```

**叩いた出力と食い違うことは書かない。** 実測を優先する。

#### (2) 状態を変えるエンドポイントと、cross-origin の規則

issue が「ハンドラを読まないと分からない」と言っている点。書く。

- 判定は `internal/server/server.go` の `crossOrigin` にある。**自分で読むこと**:
  ```bash
  sed -n '215,260p' internal/server/server.go
  ```
- 読み取り（GET / HEAD / OPTIONS）は通る。書き込み（POST / DELETE / PATCH）は、
  `Sec-Fetch-Site` と `Origin` を見て、ブラウザの別サイトからのものを拒む。
- **CLI と curl と hook はヘッダを送らないのでここを通る**、という区別が規則の本体である。
- 拒否されたときのメッセージ（`sbnn only takes changes from its own page or from the
  command line`）を**実際に再現して**引くこと:
  ```bash
  curl -s -X DELETE -H 'Sec-Fetch-Site: cross-site' localhost:6280/_/api/groups/default
  ```
  再現できないなら、その旨を報告に書き、**再現していない文言を引用しない。**
- **なぜそうなっているか**（外部ページからの POST は hook = シェルコマンドを
  登録できてしまう）を 1〜2 行添える。クライアントを書く人が 403 を「バグ」と
  誤解しないようにするのが目的。

#### (3) 安定性の宣言

issue が「安定か、明示的に不安定か、どちらかを決めて書け」と言っている。
**既定を決めてある。確認を上げずにこれで書くこと:**

> **`/_/api/` は不安定である。** 1.0 より前であり、予告なく変わる。
> これは今日の形の記述であって、互換性の約束ではない。

理由も 1 行添える。「今日の形が分かるほうが、何も言わないより、
クライアントを書く人にとって役に立つ」（issue 自身の言い分）。

そのうえで、**変わらないと言えるものがあるなら区別して書く。**
`sbnn reviews --help` が `reviews.jsonl` について「フィールドは足されるが改名されない」と
約束していることは確認済みなので、そこは別扱いにしてよい:

```bash
/tmp/sbnn-api reviews --help | grep -n -i 'renamed\|added'
```

#### (4) 落とし穴を 2 つ書く

issue が名指ししている、ハンドラを読まないと分からないもの:

- **空の group で `GET /_/api/groups/{group}` は `[]` ではなく `null` を返す**（#29）。
  すべてのクライアントが個別に場合分けしている。**自分で再現して確かめてから書くこと**:
  ```bash
  curl -s localhost:6280/_/api/groups/does-not-exist
  ```
  **#29 自体は直さない。別レーン（G6 / srv-api）の担当。** 現状として書くだけ。
- 上の (2) の cross-origin。

### 書かないもの（明示。書いたら不合格）

- **Go の型からスキーマを生成する仕組み。** issue の 2 点目はそれを求めているが、
  **Go のコードを足すことになるのでこのサイクルの担当外である。**
  代わりに、`docs/api.md` の冒頭に 1 行だけ
  「この文書は手で書かれており、型から生成されていない」と限界を明示し、
  **生成に置き換えるべきである旨を報告の「見送り / 疑義」に書くこと。**
- **OpenAPI / JSON Schema の生成物。** 同上。手書きの Markdown 1 本に留める。
- **MCP サーバ（#125）・デスクトップ（#105）・エディタ拡張（#147）の設計。**
  それらは別の issue。API の説明だけを書く。
- **README からのリンク。** README は他レーンの持ち物なので、この PR では張らない。
  **張るべきである旨を報告に書くこと。**

## 4. 完了条件（実行して真偽が決まるもの）

```bash
test -f docs/api.md
# ルーティングにある全パスが文書に出てくる（1 つでも欠けたら不合格。何も出力しないのが合格）
python3 - <<'PY'
import re,sys
src=open('internal/server/server.go').read()
routes=re.findall(r'mux\.Handle(?:Func)?\("([A-Z]+) (/_[^"]*)"',src)
doc=open('docs/api.md').read()
missing=[r for r in routes if r[1] not in doc]
print("routes:",len(routes),"missing:",len(missing))
for m in missing: print("MISSING",m)
sys.exit(1 if missing else 0)
PY
# 安定性の宣言がある
grep -niE 'unstable|not stable|no compatibility' docs/api.md
# cross-origin の規則が書かれている
grep -ni 'Sec-Fetch-Site' docs/api.md
grep -ni 'crossOrigin\|cross-origin' docs/api.md
# null を返す落とし穴が書かれている
grep -n 'null' docs/api.md
# omitempty のキーが条件付きだと分かる
grep -n 'suggestions' docs/api.md
grep -niE 'only when|when set|omitted|absent' docs/api.md
# 手書きである限界を明示している
grep -niE 'hand-written|by hand|not generated' docs/api.md
# 差分は 1 ファイルだけ
git diff --name-only origin/main            # => docs/api.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

上の Python が `missing: 0` で終了コード 0 になるのが合格。
**その出力を PR 本文の `## Verification` にそのまま貼ること。**

**さらに、§3(1) の curl の実行結果（少なくとも `groups/{group}` と
`groups/{group}/comments` と存在しない group の 3 本）を `## Verification` に貼ること。**
「ソースを読んで書いた」ではなく「叩いて確かめた」を根拠にする。

## 5. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
  安定性の宣言は **§3(3) で決めてある。確認を上げない。**
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
  特に #29（`null` を返す）、#110（skill の JSON の記述）は
  **見つけても直さない。他のレーンが同時に直している。**
- **書いた手順は実際に実行して通ることを確かめる。** `docs/api.md` に curl の例を書いたら、
  その curl を自分で走らせる。走らせていないものは書かない。
  サーバを立てたら**必ず `--shutdown` で落としてから**次へ行く。
- **テストは足さない。** このレーンは `.go` を触らない。
  PR 本文に「文書のみの変更で、対応する自動検査は #114 / #123 の設計に倣って
  別途入れるべきである」旨を 1 行書くこと
  （COMMON.md の「テストが物理的に書けない場合は理由を 1 行」の規定に沿う）。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-650017 --status running --progress 30
  gogodash task log --id t-650017 --message "<何が起きたか 1 行>"
  gogodash task set --id t-650017 --status done --progress 100 --result "PR #<番号>"
  ```

## 6. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : doc-api
worktree : /home/user/wt/doc-api
branch   : gogo/issue-148
commit   : <短縮 SHA>
```

**push と PR 作成まで行う。マージはしない。**
