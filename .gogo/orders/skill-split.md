slug: skill-split

# 指示文 DOC-04 / skill-split — スキルを層に割り、コードとのずれを機械で止める

優先度: **P3** / 期限: **本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/skill-split`
- ブランチ: issue ごとに `gogo/issue-<N>`。**毎回 `origin/main` から切り直す**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 2 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/skill-split origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

| 順番 | issue | タスク ID | 内容 |
|---|---|---|---|
| 1 本目 | #114 | `t-35b888` | スキルを CLI と突き合わせる Go テストを足す |
| 2 本目 | #113 | `t-d5b80c` | 425 行の SKILL.md を本体と `references/` に割る |

**この順番で進めること。** #114 のテストを先に入れておけば、#113 の切り出しで
内容を落としたときに**機械が気づく**。逆順だと切り出しの正しさを目視で保証することになる。

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn）で各 issue の本文を必ず読んでから直す。

## 2. **着手前に必ず確認すること（このレーンだけの前提）**

**このレーンは skill-fix レーン（#109 #110 #111 #112 #115）の後段である。**
理由は 2 つあり、どちらも回避できない:

1. **#114 のテストは、今の `main` では赤くなる。** テストが検査する 3 点のうち
   2 点が skill-fix の直す対象そのものだからである（空フェンス = #109、
   hook 環境変数 = #111、JSON キー = #110）。
   赤いテストを PR にするのは COMMON.md の検証規定に反する。
2. **#113 は `## Command reference` の表を `references/` へ動かす。**
   その表の中身は #112 が直している。先に動かすと #112 が衝突する。

### 前提チェック（実行して真偽が決まる）

```bash
cd /home/user/wt/skill-split
git fetch -q origin main
git checkout -q -B tmp-precheck origin/main

# (a) #109 が入った: 空フェンスが 0
python3 -c "import re,sys;n=len(re.findall(r'\`\`\`[a-z]*\n\`\`\`',open('skills/sbnn/SKILL.md').read()));print('empty fences:',n);sys.exit(0 if n==0 else 1)"
# (b) #111 が入った: 6 つの環境変数が全部ある
for v in SBNN_GROUP SBNN_URL SBNN_SERVER SBNN_PORT SBNN_COMMENTS SBNN_REVIEW_NOTE; do
  grep -q "$v" skills/sbnn/SKILL.md || { echo "NOT READY: $v"; break; }
done
# (c) #110 が入った: 実在するが未記載だったキーが載った
for k in diffId fileId createdAt updatedAt; do
  grep -q "\`$k\`" skills/sbnn/SKILL.md || { echo "NOT READY: $k"; break; }
done
# (d) #112 が入った: 本文が指示しているフラグが表にある
grep -q -- '--approve' skills/sbnn/SKILL.md || echo "NOT READY: --approve"
```

- **(a)(b)(c) が全部満たされていれば #114 に着手してよい。**
- **(d) も満たされていれば #113 にも着手してよい。**
- 満たされていなければ **2 分待って `git fetch -q origin main` からやり直す。**
  バックオフは 2s → 4s → 8s → 16s → 32s → 60s → 以降 120s 間隔。
  **合計 30 分待っても満たされないなら、そこで止めて報告を出すこと。**
  報告には上のチェックの**実際の出力をそのまま貼り**、
  「skill-fix の #109 / #110 / #111 / #112 が main に入るまで着手できない」と書く。
  **自分で SKILL.md を直して先に進むことは禁止。** `skills/sbnn/SKILL.md` の
  文面は skill-fix の専有であり、あなたが直すと 2 本の PR が衝突する。
- **待っている間に他のファイルを触らない。** 準備作業として読んでよいのは
  `internal/model/model.go` `internal/server/hook.go` `cmd/*.go` `skills/skills.go`（参照のみ）。

## 3. 触ってよいファイル

**この 3 つだけ。** ここから 1 バイトも出ない。

| ファイル | issue | 備考 |
|---|---|---|
| `skills/skills_test.go` | #114 | 新規作成。**あなたの専有** |
| `skills/sbnn/references/*.md` | #113 | 新規作成。**あなたの専有** |
| `skills/sbnn/SKILL.md` | #113 | **#113 の PR でのみ触る。#114 の PR では 1 行も触らない** |

### 触ってはいけないもの（明示）

- **既存の Go ファイルは 1 バイトも変えない。** 新規に足してよい `.go` は
  `skills/skills_test.go` の 1 本だけ。
  **`skills/skills.go` も `cmd/skill.go` も触らない。**
  → issue #113 は「`sbnn skill`（SKILL.md しか出さない）と `--list` は見直したほうがよい」と
    書いているが、**それは Go の変更なのでこのサイクルの担当外である。**
    §6 の「既知の帰結」に書いてあるとおり、報告に上げて次の回に回す。
- `README.md` — 触らない（doc-readme / doc-repo / doc-collapse の持ち物）。
- `cmd/` `internal/` `web/` `docs/` `Taskfile.yml` `go.mod` `go.sum` `.github/` — 触らない。

## 4. 1 本目 — #114 `gogo/issue-114`

### 作るもの

`skills/skills_test.go` を新規作成する。**この PR の変更はこの 1 ファイルだけ。**
`skills/sbnn/SKILL.md` は**この PR では 1 行も変えない。**

パッケージ名は `skills`（既存の `skills.go` と同じ）で始める。
`cmd` パッケージを import する必要が出たら `package skills_test` に変える
（`cmd` は `skills` を import しているので、内部テストパッケージから import すると
循環になる。外部テストパッケージなら通る）。
**ただし §「フラグの検査」のとおり、import しないやり方を採るのでその必要は無いはずである。**

### 何を検査するか（4 つ。全部書く）

issue #114 が挙げた 4 点をそのまま実装する。

1. **空のフェンスが無いこと。**
   ` ``` ` の直後に閉じフェンスが来る箇所を検出して落とす。
   「フェンスの対の間に何も無い」は意図的であることが無い、という issue の主張どおり。

2. **hook の環境変数を全部言及していること。**
   実装側の一覧は `internal/server/hook.go` の `runHookCommand` にリテラルで並んでいる。
   ソースを読んで `SBNN_[A-Z_]+` を拾い、**そのすべてがスキルの文章に現れる**ことを確かめる。
   ```bash
   sed -n '78,92p' internal/server/hook.go
   ```
   ハードコードした 6 つと比較するのではなく、**ソースから拾って比較する**こと。
   そうしないと 7 つ目が足されたときに気づけない（この issue の目的そのもの）。

3. **`comments --format json` のキーの契約。**
   `model.Comment` をゼロ値でなく**中身のある値で** marshal し、キー集合を得る。
   `omitempty` のキー（`author` `question` `suggestions`）と、常にあるキーを**区別する**。
   - 常にあるキーは、値が空でも出るので、**ゼロ値の `model.Comment` を marshal**すれば分かる。
   - `omitempty` のキーは、**値を入れた `model.Comment`** を marshal すれば出てくる。
     `suggestions` は `MarshalJSON` が `Suggestions(c.Body)` から作るので、
     Body に ` ```suggestion ` ブロックを入れた値で試す。
   - 検査内容: **常にあるキーはすべてスキルの文章に現れること**、および
     **`omitempty` のキーは「条件付き」と分かる書かれ方をしていること**
     （「常にある」側の一覧に混ざっていないこと）。
   - 後者は文面依存になりやすい。**厳しくしすぎて壊れやすいテストにしないこと。**
     最低限「`question` と `suggestions` の近くに『無いことがある』意味の語
     （`only when` / `when set` / `appear only` のいずれか）がある」程度で足りる。
     採った判定と、なぜその強さにしたかを**テストのコメントに英語で書くこと。**

4. **フェンス内の `sbnn …` のフラグが全部実在すること。**
   - **`cmd` パッケージを import しない。** `rootCmd` は非公開で外から辿れない。
     代わりに、テストの中で `go build -o <t.TempDir()>/sbnn <モジュールルート>` を 1 回走らせ、
     `sbnn --help` と各サブコマンドの `--help` からコマンド名とフラグ名を集める。
     ネットワークもブラウザも要らない。ビルドは実測 1 秒未満である。
   - `exec.LookPath("go")` が失敗したら `t.Skip`。それ以外で落ちたら `t.Fatal`。
   - **必ず踏む罠（先に潰しておくこと）**: `$( … )` のコマンド置換の中には
     git などのフラグが入る。**検査の前にコマンド置換を丸ごと取り除くこと。**
   - パイプで繋がれた行は `|` で分割し、`sbnn` で始まる区間だけを見る。
   - この検査は**現在の main でも通る**ことを確認済みである
     （SKILL.md 内 37 件の呼び出し、未知フラグ 0 件）。落ちたら書き方の問題を疑うこと。

### **重要 — 検査の対象は SKILL.md 1 本ではなく、スキルのツリー全体にすること**

次の PR（#113）で内容の一部が `skills/sbnn/references/*.md` へ移る。
**SKILL.md だけを読むテストを書くと、#113 で自分のテストが赤くなる。**

したがって、2 / 3 / 4 の検査は **`skills/sbnn/` 配下の `*.md` を全部連結したもの**に対して行う。
`skills.FS()` を `fs.WalkDir` で歩けば、埋め込まれているものをそのまま辿れる
（`skills.go` が既に `//go:embed all:sbnn` で持っている）。
1 の「空フェンス」の検査も、**各ファイルごとに**行う。

**この設計判断を、テストファイルの冒頭コメントに英語で 1〜2 行書くこと。**
「スキルは複数ファイルになりうるので、契約はツリー全体に対して検査する」。

### 完了条件

```bash
gofmt -l skills/skills_test.go     # 何も返らないこと
go build ./... && go vet ./... && go test ./...
go test ./skills/ -run . -v        # 4 つのテストが PASS
# SKILL.md を触っていない
git diff --name-only origin/main   # => skills/skills_test.go の 1 行だけ
git diff --name-only origin/main -- '*.go' | grep -v '^skills/skills_test\.go$'   # 何も返らないこと
```

**さらに、テストが本当に効いていることを 4 つとも自分で確かめる**（自己申告にしない）。
各検査につき 1 回ずつ、SKILL.md を一時的に壊してテストが落ちることを見る:

```bash
cp skills/sbnn/SKILL.md /tmp/SKILL.bak

# (1) 空フェンスを混ぜる
printf '\n```\n```\n' >> skills/sbnn/SKILL.md
go test ./skills/ >/dev/null 2>&1; echo "empty-fence guard: exit=$? (0 以外なら合格)"
cp /tmp/SKILL.bak skills/sbnn/SKILL.md

# (2) 環境変数を 1 つ消す
sed -i 's/SBNN_PORT/SBNN_XXXX/g' skills/sbnn/SKILL.md
go test ./skills/ >/dev/null 2>&1; echo "hook-env guard: exit=$? (0 以外なら合格)"
cp /tmp/SKILL.bak skills/sbnn/SKILL.md

# (3) JSON キーを 1 つ消す
sed -i 's/`diffId`/`diffID`/g' skills/sbnn/SKILL.md
go test ./skills/ >/dev/null 2>&1; echo "json-key guard: exit=$? (0 以外なら合格)"
cp /tmp/SKILL.bak skills/sbnn/SKILL.md

# (4) 存在しないフラグを混ぜる
printf '\n```\nsbnn comments --no-such-flag\n```\n' >> skills/sbnn/SKILL.md
go test ./skills/ >/dev/null 2>&1; echo "flag guard: exit=$? (0 以外なら合格)"
cp /tmp/SKILL.bak skills/sbnn/SKILL.md

# 元に戻っている
git diff --stat skills/sbnn/SKILL.md   # 何も返らないこと
go test ./skills/ ; echo "clean: exit=$? (0 なら合格)"
```

**この 5 回の実行結果（終了コード）を PR 本文の `## Verification` にそのまま貼ること。**
「テストを書いた」ではなく「4 つとも、壊したら落ちることを見た」を根拠にする。
**1 つでもガードが落ちなかったら、そのテストは効いていない。直してから出すこと。**

## 5. 2 本目 — #113 `gogo/issue-113`

### 変えるもの

`skills/sbnn/SKILL.md` から、参照材料を `skills/sbnn/references/` へ切り出す。

issue が挙げている切り出し候補と行数:

| 節 | 目安 | いつ要るか |
|---|---|---|
| `## Command reference`（#112 で改名済み） | ~33 行 | 構文を引くとき |
| `## Fitting sbnn into what you were already doing` | ~33 行 | パイプラインに組むとき |
| `## Learning from past reviews` | ~28 行 | `sbnn reviews` を読むとき |
| `### Sharing a review without sbnn` | ~15 行 | export するとき |
| `## Notes` | ~18 行 | ほぼ要らない |

### 決めてあること（確認を上げずにこれで進める）

1. **7 段階のワークフロー（`## Workflow` 配下の 1〜7 と
   `### Reviewing instead of being reviewed`）は本体に残す。**
   issue が「なぜそうするかを毎段で説明していて、それが使える理由になっている」と
   評価している部分である。**削らない、要約しない。**
2. **`### Sharing a review without sbnn` は本体に残す。**
   #115 がこれを「第三者への共有」から「人間が localhost に届かないときの
   フォールバック」に格上げして、**手順 3 の判断から参照させている。**
   本流の判断が指す先を `references/` へ出すと、#115 の修正が意味を失う。
   **issue の候補表と食い違うが、こちらを優先する。理由を PR 本文に 1 行書くこと。**
3. **切り出すのは残りの 4 つ。** ファイル名は次で固定する（迷わない）:
   - `skills/sbnn/references/commands.md` ← `## Command reference`（#112 後の名前）
   - `skills/sbnn/references/pipelines.md` ← `## Fitting sbnn into what you were already doing`
   - `skills/sbnn/references/review-history.md` ← `## Learning from past reviews`
   - `skills/sbnn/references/notes.md` ← `## Notes`
4. **切り出しは「移動」であって「書き直し」ではない。**
   文面を変えない。各ファイルの先頭に `# <元の見出し>` を 1 行置くだけでよい。
   **#112 が直したばかりの表を、ここでまた書き換えない。**
5. **本体から各ファイルへ、「いつ読むか」を 1 行添えて指す。**
   issue の求めているのはこれ（「when to read each」）。
   本文の末尾に短い節を置き、4 本を 1 行ずつ紹介する。
   リンクは相対パス（`references/commands.md`）で書く。

### 既知の帰結（**PR 本文と報告の両方に書くこと**）

- **`sbnn skill` は `SKILL.md` しか標準出力に出さない**（`skills.Markdown()` は
  `sbnn/SKILL.md` を読むだけ）。したがって README が案内している
  「スキル機構を持たないエージェント向け: `sbnn skill >> AGENTS.md`」の経路では、
  **切り出した 4 本が届かなくなる。**
- **`sbnn skill --list` は `fs.WalkDir` でツリーを歩くので、
  `references/` も一覧に出る**（`--install` も同様にツリーごと書き出す）。
  こちらは今のままで正しく動く。**自分で確かめること**:
  ```bash
  go build -o /tmp/sbnn-113 .
  /tmp/sbnn-113 skill --list
  /tmp/sbnn-113 skill --install /tmp/skilltest && find /tmp/skilltest -type f
  ```
- 上の 1 点目は **Go の変更が要るので、このサイクルでは直さない。**
  issue #113 自身も「`sbnn skill` と `--list` は見直したほうがよい」と書いている。
  **報告の「見送り / 疑義」に、新しい issue の種として書くこと。**

### 完了条件

```bash
# 4 本ができた
test -f skills/sbnn/references/commands.md
test -f skills/sbnn/references/pipelines.md
test -f skills/sbnn/references/review-history.md
test -f skills/sbnn/references/notes.md
# 本体が短くなった（425 行から 300 行未満へ）
wc -l skills/sbnn/SKILL.md
python3 -c "import sys;n=sum(1 for _ in open('skills/sbnn/SKILL.md'));print(n);sys.exit(0 if n<300 else 1)"
# 本体が 4 本を指している
for f in commands pipelines review-history notes; do
  grep -q "references/$f.md" skills/sbnn/SKILL.md && echo "ok $f" || { echo "MISSING link $f"; false; }
done
# ワークフローと export のフォールバックは本体に残っている
grep -n '^### 1\.' skills/sbnn/SKILL.md
grep -n '^### 7\.' skills/sbnn/SKILL.md
grep -n 'Sharing a review without sbnn' skills/sbnn/SKILL.md
# 内容を落としていない（#114 のテストがツリー全体を見ているので、これが効く）
go test ./skills/ -v
# 埋め込みとインストールが壊れていない
go build -o /tmp/sbnn-113 . && /tmp/sbnn-113 skill --list
rm -rf /tmp/skilltest && /tmp/sbnn-113 skill --install /tmp/skilltest && find /tmp/skilltest -type f | sort
# Go を触っていない
git diff --name-only origin/main -- '*.go'   # 何も返らないこと
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')   # 何も返らないこと（対象が無ければ空）
```

`sbnn skill --list` と `--install` の出力を **PR 本文の `## Verification` にそのまま貼ること。**
`--install` が `references/` の 4 本を書き出していることが目で見えるようにする。

## 6. 全 PR 共通の検証（COMMON.md の「検証」に上乗せ）

```bash
go build ./... && go vet ./... && go test ./...
gofmt -l $(git diff --name-only origin/main -- '*.go')            # 何も返らないこと
git diff --name-only origin/main -- '*.go' | grep -v '_test\.go$' # 何も返らないこと
git status --short                                                 # 未追跡の残骸が無いこと
```

**「Go のコードは 1 バイトも変えない」の解釈はこれで固定する:**
既存の `.go` ファイルは 1 バイトも変えない。新規に足してよいのは `_test.go` だけで、
今回それは `skills/skills_test.go` の 1 本のみ。実装（非テスト）の `.go` は
新規作成も禁止。**迷ったら足さない。**

## 7. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
  #113 の切り出し先のファイル名、`### Sharing a review without sbnn` を残す判断は
  **既に §5 で決めてある。確認を上げない。**
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
- **§2 の前提チェックだけは例外で、待ってよい。** それ以外で待たない。
  30 分で打ち切って報告を出す。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-35b888 --status running --progress 30
  gogodash task log --id t-35b888 --message "<何が起きたか 1 行>"
  gogodash task set --id t-35b888 --status done --progress 100 --result "PR #<番号>"
  ```
  待ちに入ったら `--status blocked` を打ち、理由を `task log` に残すこと。
  タスク ID は §1 の表のとおり（#114=`t-35b888` / #113=`t-d5b80c`）。

## 8. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : skill-split
worktree : /home/user/wt/skill-split
branch   : gogo/issue-114 / gogo/issue-113
commit   : <各ブランチの短縮 SHA を 1 行ずつ>
```

**push と PR 作成まで行う。マージはしない。**
