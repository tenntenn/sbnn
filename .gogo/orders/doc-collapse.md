slug: doc-collapse

# 指示文 DOC-06 / doc-collapse — `--collapse` の区切り文字を README に書く

優先度: **P3** / 期限: **本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/doc-collapse`
- ブランチ: `gogo/issue-52`。**`origin/main` から切る**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 1 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/doc-collapse origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

| issue | タスク ID | 内容 |
|---|---|---|
| #52 | `t-fac06f` | `--collapse` がカンマで分割されるのに、そう書かれていない |

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn, issue_number=52）で本文を必ず読んでから直す。

## 2. **このレーンの担当範囲 — ここを読み違えると衝突する**

issue #52 は 2 つのことを言っている。**あなたが担当するのは片方だけ。**

| issue が言っていること | 担当 |
|---|---|
| 区切り文字（カンマと改行）が**文書化されていない** | **あなた（README の記述のみ）** |
| カンマ分割は**やめることを検討すべき**（正当なファイル名を潰すため） | **あなたではない** |

**`cmd/root.go` の `splitPatterns` の挙動そのものと、`--collapse` のフラグヘルプ文字列には
1 バイトも触らないこと。** そこは `planner-cli` が出す `cmd-flags` レーン（G8 / #142 #160 #161）の
持ち物で、同じ関数を同時に触ると衝突する。

**あなたは README の記述だけを直す。**

参考までに、あなたが**触らない**現物はここにある（読むのはよい。変えてはいけない）:

```bash
sed -n '440,455p' cmd/root.go        # splitPatterns
sed -n '168,175p' cmd/root.go        # --collapse のフラグヘルプ
```

## 3. 触ってよいファイル

**この 1 つだけ、しかも 1 つの節の中だけ。**

| ファイル | 領域 |
|---|---|
| `README.md` | **`### Folding away what nobody reads` の節の中だけ**（現 188〜222 行目） |

**節の外の行は 1 行も変えない。** README は 4 つのレーンが別々の節を持っている:

| 領域 | 現在の行 | 持ち主 |
|---|---|---|
| イントロ・`## Install`・477・508・522/523 の境界 | — | doc-readme（#107 / #132）。**触らない** |
| `### Folding away what nobody reads` | 188–222 | **あなた** |
| `## Files and ports` | 546–559 | G1 の paths レーン（#104）。**触らない** |
| `## Development` | 561–573 | doc-repo（#106）。**触らない** |

着手前に自分の領域の行番号を確かめること:

```bash
grep -n '^### Folding away what nobody reads' README.md
grep -n '^### Comments from an agent' README.md    # 次の見出し。ここより前で止める
```

### 触ってはいけないもの（明示）

- **`cmd/root.go`** — §2 のとおり。**Go のコードは 1 バイトも変えない。**
  このレーンは `.go` を新規追加も含めて 1 本も触らない。
  `git diff --name-only origin/main -- '*.go'` が**常に空**であること。
- **`skills/sbnn/SKILL.md`** — skill-fix レーン（#109〜#115）の専有。**触らない。**
  スキルの `## Command reference` にも `--collapse` の行があるが、
  その表は #112 が同時に作り直している。**あなたは触らない。**
  スキル側にも同じ説明が要る、という指摘は**報告に書いて次の回に回すこと。**
- `docs/` `.github/` `Taskfile.yml` `go.mod` `go.sum` `web/` — 触らない。

## 4. 変えるもの

`README.md` の `### Folding away what nobody reads` の節に、**区切り文字の説明を足す。**

### 先に自分で確かめること（推測で書かない）

```bash
go build -o /tmp/sbnn-52 .
/tmp/sbnn-52 --help | grep -n -A2 'collapse'
sed -n '440,455p' cmd/root.go
```

実装は `strings.FieldsFunc` で `,` と `\n` の両方を区切りとして扱い、
各要素を `TrimSpace` して空を捨てている。つまり:

- `--collapse 'go.sum,web/dist/**'` は 2 つのパターンになる。
- `--collapse "$(cat .sbnnignore)"` は改行区切りのファイルをそのまま渡せる。
- `--collapse` 自体が繰り返し可能である（`StringArrayVar`）。

**実際に動かして、この 3 つが本当にそうなることを見てから書くこと。**
（サーバは終わったら**必ず落とす**。並列で動いている他のレーンとポートを取り合わない）:

```bash
printf 'diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n@@ -0,0 +1 @@\n+x\n' | /tmp/sbnn-52 --target t52 --collapse 'go.sum,web/dist/**' --no-open
curl -s localhost:6280/_/api/groups/t52 | python3 -m json.tool | grep -n -i 'collapse\|go.sum' | head
/tmp/sbnn-52 --clear --target t52
/tmp/sbnn-52 --shutdown
```

### 書くこと

1. **区切りは 2 つある**（カンマと改行）ことを平文で書く。
   今のヘルプの `gitignore-style: go.sum, web/dist/**` は、
   **構文ではなく「例を 2 つ並べた散文」に読める。** そこが issue の指摘。
   README では取り違えようのない書き方にする。
2. **改行区切りの使い道を書く。** `--collapse "$(cat .sbnnignore)"` の形。
   issue が「区切りのうち有用なのはこちら」と言っている。**動かしてから書く。**
3. **カンマ区切りの代償を書く。** カンマを含むパスは `--collapse` で畳めない。
   `--collapse 'docs/a,b.md'` は 2 つのパターンになり、どちらも何にも当たらない。
   **`--collapse` は繰り返せるので、そういうパスは繰り返しで渡せばよい**、という
   回避策まで書く。読者がその場で解決できる状態にする。
4. **カンマ分割をやめる提案は書かない。** それは挙動の変更で `cmd-flags` レーンの判断。
   README に「将来やめるかもしれない」と書くと、**決まっていないことを約束することになる。**
   書かない。

**節の既存の文章は、必要な範囲でしか変えない。** 全面書き直しをしない。
既存の説明は良く書けている。**足りないのは区切り文字の説明だけ**である。

## 5. 完了条件（実行して真偽が決まるもの）

```bash
# 区切りが 2 つあることが書かれている
awk '/^### Folding away what nobody reads/{f=1} /^### Comments from an agent/{f=0} f' README.md | grep -niE 'comma'
awk '/^### Folding away what nobody reads/{f=1} /^### Comments from an agent/{f=0} f' README.md | grep -niE 'newline|new line|one per line'
# 改行区切りの使い道が書かれている
grep -n 'collapse "\$(cat\|collapse "$(cat' README.md
# カンマを含むパスが畳めないことと、繰り返しで回避できることが書かれている
awk '/^### Folding away what nobody reads/{f=1} /^### Comments from an agent/{f=0} f' README.md | grep -niE 'repeat'
# 自分の節の外を触っていない（すべて何も返らないこと）
git diff origin/main -- README.md | grep -n '^[-+].*go install github'
git diff origin/main -- README.md | grep -n '^[-+].*XDG_STATE_HOME'
git diff origin/main -- README.md | grep -n '^[-+].*task build'
git diff origin/main -- README.md | grep -n '^[-+].*Contents'
# 差分は README.md 1 ファイルだけ
git diff --name-only origin/main            # => README.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

**書いた例が実際に動くことを確かめた出力を、PR 本文の `## Verification` に貼ること。**
README に新しいコマンド例を書いたなら、**その例をそのまま走らせた結果**を貼る。
走らせていない例は書かない。

**差分が `### Folding away what nobody reads` の節に収まっていることを、
`git diff origin/main -- README.md` を目で読んで確かめること。**
節の外に 1 行でも出ていたら、他のレーンとマージで衝突する。

## 6. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
  特に次の 2 つは**見つけても直さないこと**:
  - `--collapse` のフラグヘルプ文字列と `splitPatterns` の挙動（`cmd-flags` レーン）
  - `skills/sbnn/SKILL.md` の `--collapse` の行（skill-fix レーン #112）
  **どちらも「同じ説明が要る」と報告に書いて渡す。**
- **テストは足さない。** このレーンは `.go` を触らない。
  PR 本文に「README の文面のみの変更で、README の主張を検査する仕組みは #123 で入る」旨を
  1 行書くこと（COMMON.md の「テストが物理的に書けない場合は理由を 1 行」の規定に沿う）。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-fac06f --status running --progress 30
  gogodash task log --id t-fac06f --message "<何が起きたか 1 行>"
  gogodash task set --id t-fac06f --status done --progress 100 --result "PR #<番号>"
  ```

## 7. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : doc-collapse
worktree : /home/user/wt/doc-collapse
branch   : gogo/issue-52
commit   : <短縮 SHA>
```

**push と PR 作成まで行う。マージはしない。**
