slug: skill-fix

# 指示文 O-03 / skill-fix — エージェントスキルの記述をコードの実態に合わせる

優先度: **P3** / 期限: **本サイクル内（2026-08-25 中）**

## 0. 前提（読んでから始める）

- `/home/user/briefs/COMMON.md` を必ず先に読むこと。手順・コミット書式・PR 書式・報告書式は
  すべてそこに書いてある。この指示文はそれを**上書きしない**。差分だけをここに書く。
- 本体リポジトリ `/home/user/sbnn` は**参照のみ**。触らない。
- あなたの worktree: `/home/user/wt/skill-fix`
- ブランチ: issue ごとに `gogo/issue-<N>`。**毎回 `origin/main` から切り直す**（COMMON.md 手順 2）。
- **1 issue = 1 PR。** この指示文は 5 本の PR になる。
- **push と PR 作成まで行う。マージはしない。**

worktree がまだ無ければ自分で作る:

```bash
git -C /home/user/sbnn worktree add /home/user/wt/skill-fix origin/main
```

## 1. 担当する issue とダッシュボードのタスク ID

**この順番で進めること。**

| 順番 | issue | タスク ID | 内容 |
|---|---|---|---|
| 1 本目 | #109 | `t-49bb37` | `--suggest` の例が空のコードブロック |
| 2 本目 | #111 | `t-236122` | hook の環境変数が 2 つ足りない |
| 3 本目 | #110 | `t-10c7a9` | `comments --format json` の形が両方向に間違っている |
| 4 本目 | #115 | `t-94d2a0` | localhost を渡せない相手への分岐が無い |
| 5 本目 | #112 | `t-4db111` | `## Command reference` が本文と食い違っている |

`mcp__github__issue_read`（owner=tenntenn, repo=sbnn）で各 issue の本文を必ず読んでから直す。

**#112 を最後に置いているのは、#113（skill-split レーン）がこの表を
`references/` へ切り出す前提で待っているため。** ここが遅れると後段が丸ごと止まる。
**#112 まで終えたら、報告を出す前に一度ダッシュボードへ完了を打つこと。**

## 2. 触ってよいファイル

**この 1 つだけ。** ここから 1 バイトも出ない。

| ファイル | 誰の担当か |
|---|---|
| `skills/sbnn/SKILL.md` | **あなたの専有。5 本の PR すべてこのファイルだけを変える** |

### 触ってはいけないもの（明示）

- **Go のコードは 1 バイトも変えない。** このレーンは `.go` を 1 本も追加も編集もしない。
  `git diff --name-only origin/main -- '*.go'` が**常に空**であること。
  `skills/skills.go` も `cmd/skill.go` も**触らない。**
- `skills/sbnn/references/` — **作らない。** それは #113（skill-split）の担当。
  あなたは `SKILL.md` を**1 ファイルのまま**直す。切り出しはしない。
- `skills/skills_test.go` — **作らない。** それは #114（skill-split）の担当。
- `README.md` — 触らない。skill について README に書いてある部分は doc-readme の持ち物。
- YAML front matter（1〜5 行目の `name` / `description` / `license`）は
  **#115 以外では変えない。** #115 でも変えるのは `description` だけ（§6 参照）。

## 3. 共通の作法 — 「コードで確かめてから書く」

このレーンの 5 件は全部「文書がコードとずれている」issue である。
**したがって、直した内容の根拠は必ずコードから取る。issue の本文を写経しない。**
issue に書かれた事実も、自分で 1 回実行して確かめる。

最初に 1 回だけバイナリを作っておくと以降が楽になる:

```bash
cd /home/user/wt/skill-fix
go build -o /tmp/sbnn-fix .
/tmp/sbnn-fix --help
```

## 4. 1 本目 — #109 `gogo/issue-109`

### 何が壊れているか（確認済み）

`skills/sbnn/SKILL.md` の 188 行目と 189 行目が連続した裸のフェンスで、
中身が空である。直前の散文は「suggestion ブロックがどう見えるかを示す」と約束している。

```bash
sed -n '183,194p' skills/sbnn/SKILL.md
python3 -c "import re;print(len(re.findall(r'\`\`\`[a-z]*\n\`\`\`',open('skills/sbnn/SKILL.md').read())))"   # => 1
```

### 直すもの

空のブロックを、**実際の suggestion ブロックの例**で埋める。
issue が提案している形をそのまま使ってよい:

````
```suggestion
if err != nil {
    return fmt.Errorf("read config: %w", err)
}
```
````

**入れ子のフェンスになるので、外側は 4 バックティックにする**（SKILL.md の他の箇所と同じ流儀）。
書いたあと、Markdown として壊れていないことを下の完了条件で確かめる。

### 完了条件

```bash
# 空のフェンスが 0 になった
python3 -c "import re;import sys;n=len(re.findall(r'\`\`\`[a-z]*\n\`\`\`',open('skills/sbnn/SKILL.md').read()));print(n);sys.exit(0 if n==0 else 1)"
# suggestion の実例が入った
grep -n '^```suggestion' skills/sbnn/SKILL.md
# フェンスの開閉が釣り合っている
python3 -c "
import re
t=open('skills/sbnn/SKILL.md').read().splitlines()
d=0
for l in t:
    m=re.match(r'^(\`{3,4})',l)
    if m: d+=1
print('fence lines:',d,'balanced' if d%2==0 else 'UNBALANCED')
"
# バイナリに埋め込まれた側でも同じものが出る
go build -o /tmp/sbnn-109 . && /tmp/sbnn-109 skill | grep -n '^```suggestion'
git diff --name-only origin/main            # => skills/sbnn/SKILL.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

`sbnn skill` が埋め込みから同じ内容を出すことまで確かめること。
**このファイルは `//go:embed` でバイナリに入り、`sbnn skill --install` で利用者の
エージェントディレクトリに書き出される。** 直っていないコピーが配られる。

## 5. 2 本目 — #111 `gogo/issue-111`

### 何が壊れているか（確認済み）

SKILL.md は hook に渡る環境変数を 4 つしか挙げていない。実際は 6 つある。

```bash
grep -n 'SBNN_GROUP' skills/sbnn/SKILL.md
sed -n '80,90p' internal/server/hook.go     # 実際にセットされる 6 つ
grep -n 'SBNN_' cmd/hook.go                 # --help は 6 つとも挙げている
```

実装（`internal/server/hook.go`）がセットするのは
`SBNN_GROUP` `SBNN_URL` `SBNN_SERVER` `SBNN_PORT` `SBNN_COMMENTS` `SBNN_REVIEW_NOTE`。
**SKILL.md にだけ `SBNN_SERVER` と `SBNN_PORT` が無い。**

### 直すもの

`### 3. Hand the URL to the human…` の中の当該段落（現 162〜166 行目）を書き直す。

- **6 つ全部を挙げる。**
- **それぞれが何のためにあるかを 1 行ずつ書く。** issue が強調しているのは
  `SBNN_SERVER` / `SBNN_PORT` が「hook が自分を起動したサーバへ喋り返すための道」だという点。
  hook の中で `sbnn comments` を走らせたいとき、どのポートに聞けばよいかの答えがこれである。
- **同じ節の `--port` の注意書きと矛盾させない。** SKILL.md の
  「`--port` … use it only if the user runs sbnn on a non-default port」は
  「どうやって知るのか」に答えていない。`SBNN_PORT` がその答えだと繋げること。
  該当行は `## Command reference` の直後にある（`grep -n 'non-default port' skills/sbnn/SKILL.md`）。
  **表そのものは #112 で直すので、ここでは表の下の 1 段落だけを触る。**
- **hook が「レビューの判定（approve / request changes）」を受け取れないことは
  ここでは直さない。** それは #25（別レーン）。書き足さない。

### 完了条件

```bash
for v in SBNN_GROUP SBNN_URL SBNN_SERVER SBNN_PORT SBNN_COMMENTS SBNN_REVIEW_NOTE; do
  grep -q "$v" skills/sbnn/SKILL.md && echo "ok $v" || { echo "MISSING $v"; false; }
done
# 実装の一覧と突き合わせる（SKILL.md が実装より短くない）
diff <(grep -o 'SBNN_[A-Z_]*' internal/server/hook.go | sort -u) \
     <(grep -o 'SBNN_[A-Z_]*' skills/sbnn/SKILL.md | sort -u | grep -E 'GROUP|URL|SERVER|PORT|COMMENTS|REVIEW_NOTE')
git diff --name-only origin/main            # => skills/sbnn/SKILL.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

## 6. 3 本目 — #110 `gogo/issue-110`

### 何が壊れているか（確認済み）

SKILL.md の 208〜210 行目付近が `comments --format json` の形を「Every JSON entry has …」
として列挙しているが、**両方向に間違っている。**

```bash
sed -n '205,215p' skills/sbnn/SKILL.md
sed -n '147,180p' internal/model/model.go     # Comment のタグ
sed -n '255,264p' internal/model/model.go     # MarshalJSON が suggestions を足す
```

実測すると:

- **常にあるキー**（`omitempty` でない）:
  `id` `group` `diffId` `fileId` `path` `side` `startLine` `endLine` `body`
  `snippet` `resolved` `createdAt` `updatedAt`
- **条件付きのキー**（`omitempty`）: `author` `question` `suggestions`
- SKILL.md は `suggestions` と `question` を「必ずある」と書いており、
  `group` `diffId` `fileId` `createdAt` `updatedAt` を落としている。

**`author` も `json:"author,omitempty"` である。** issue の Expected は `author` を
「常にある」側に置いているが、**コードのほうが正しい。**
自分で実測して確かめ、**実測に合わせて書くこと。** 実測方法:

```bash
go build -o /tmp/sbnn-110 . && /tmp/sbnn-110 --version
# 実際に 1 件投げて JSON を見る（サーバを立てて、終わったら必ず落とす）
printf 'diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1 @@\n+x\n' | /tmp/sbnn-110 --target t110 --no-open
/tmp/sbnn-110 comment a.go:1 -m hi --target t110
/tmp/sbnn-110 comments --target t110 --format json | python3 -c "import json,sys;print(sorted(json.load(sys.stdin)[0].keys()))"
/tmp/sbnn-110 --clear --target t110
/tmp/sbnn-110 --shutdown
```

**実測したキーの一覧を PR 本文の `## Verification` にそのまま貼ること。**

### 直すもの

当該段落を、**「常にあるキー」と「条件付きのキー」を分けて**書き直す。
issue の Expected の文面を土台にしてよいが、**`author` の扱いは実測に合わせて直すこと。**
「無いキーは『いいえ』『無し』として扱え」と明示する（Python なら `KeyError`、JS なら
`undefined` になる、という失敗の形まで書く必要はない。挙動の指示だけでよい）。

同じ節のあとの 2 段落（`question` で分岐しろ、`suggestions` 配列を適用しろ）も
**「キーが無いことがある」前提と矛盾しないように**言い回しを直す。

### 完了条件

```bash
for k in id group diffId fileId path side startLine endLine body snippet resolved createdAt updatedAt author question suggestions; do
  grep -q "\`$k\`" skills/sbnn/SKILL.md && echo "ok $k" || { echo "MISSING $k"; false; }
done
# 「常に」と「条件付き」が言い分けられている
grep -niE 'only when|appear only|conditional|when set' skills/sbnn/SKILL.md
# 「Every JSON entry has …（旧文）」が残っていない
grep -c 'snippet\`, \`suggestions\`, \`question\`' skills/sbnn/SKILL.md   # => 0
git diff --name-only origin/main            # => skills/sbnn/SKILL.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

## 7. 4 本目 — #115 `gogo/issue-115`

### 何が壊れているか

`### 3. Hand the URL to the human…` は、**人間がその URL を開けるかを一度も確かめない。**
スマートフォンのアプリの中で動いているエージェントは、手順どおりに背景サーバを立て、
`http://localhost:6280/` を出し、開けと言う。相手には何も届かない。

```bash
grep -c -i 'browser' skills/sbnn/SKILL.md   # 8
grep -c -i 'mobile' skills/sbnn/SKILL.md    # 0
grep -c -i 'phone'  skills/sbnn/SKILL.md    # 0
grep -n 'Sharing a review without sbnn' skills/sbnn/SKILL.md
```

`sbnn export` は存在するが `### Sharing a review without sbnn`（現 302 行目）という
**「第三者への共有」の見出しの下**に置かれていて、
「依頼した本人が localhost に届かない」場合の**フォールバック**として繋がっていない。

### 直すもの

issue の Expected が 4 点。全部やる。

1. **判断を本流の手順に入れる。** `### 3.` の冒頭、URL を渡すより**前**に分岐を置く。
   規定の文言を決めること（曖昧にしない）:
   **「この機械の上で人間がブラウザを開けると確信できないなら、
   URL ではなく自己完結した HTML を出してそれを渡す」。**
2. **どちらが既定で、利用者が何を頼めるのかを明記する。**
   issue が引いている利用者の疑問「artifact の生成は自動なのか指示なのか」に、
   **ファイルの中で答えが出ている状態にする。**
   既定は URL、確信が持てないときは export、利用者が明示的に頼めばいつでも export。
3. **標準出力へ書く形を示す。** 既存の例は全部ファイル名付き（`sbnn export … review.html`）。
   artifact に**中身**を入れたいときに要るのは、ファイル名を渡さない `--fragment` の形である。
   **書く前に実際に叩いて確かめること**:
   ```bash
   go build -o /tmp/sbnn-115 .
   printf 'diff --git a/a.md b/a.md\n--- a/a.md\n+++ b/a.md\n@@ -0,0 +1 @@\n+x\n' | /tmp/sbnn-115 export --fragment --target t115 | head -5
   /tmp/sbnn-115 export --help
   ```
   **叩いた結果と違うことは書かない。** `--fragment` が引数なしで標準出力に出ないなら、
   出る形を `--help` から見つけて、そちらを書く。実測を優先する。
4. **往復ができなくなることを、フォールバックを紹介する場所で書く。**
   export したページに書かれたコメントはそのブラウザに留まり、サーバへ来ない。
   したがって手順 5〜7（`sbnn comments` で読み戻す）は成立しない。
   **その帰結まで書く**: `sbnn comments` は何も返さない。人間には
   ページの「Copy prompt」で本文を写して貼り戻してもらう。

さらに、front matter の `description` に**この分岐が起動条件に入るように 1 語足す。**
現在の `description` は「ブラウザで diff を開く」系の依頼で発火すると書いてあり、
まさにこの事故が起きる状況を呼び込んでいる。
**`description` は 1 行の値なので、YAML として壊さないこと**（下の完了条件で検査する）。

**#55（export したページがスマホ幅で見出しが重なる）は直さない。** 別レーンの担当。

### 完了条件

```bash
# 分岐が手順 3 の中にある（節の中に export への言及がある）
awk '/^### 3\. Hand the URL/{f=1} /^### 4\./{f=0} f' skills/sbnn/SKILL.md | grep -n 'export'
# 既定がどちらか明言されている
awk '/^### 3\. Hand the URL/{f=1} /^### 4\./{f=0} f' skills/sbnn/SKILL.md | grep -niE 'default|by default'
# 標準出力の形が載っている
grep -n 'export --fragment' skills/sbnn/SKILL.md
# 往復できないことの帰結が書かれている
grep -niE 'sbnn comments (will )?return|nothing to read back|copy prompt' skills/sbnn/SKILL.md
# front matter が YAML として妥当
python3 -c "
import yaml
t=open('skills/sbnn/SKILL.md').read().split('---')
d=yaml.safe_load(t[1])
print(sorted(d.keys()))
assert 'name' in d and 'description' in d
print('front matter ok')
"
git diff --name-only origin/main            # => skills/sbnn/SKILL.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

## 8. 5 本目 — #112 `gogo/issue-112`

### 何が壊れているか

`## Command reference`（現 376 行目からの 25 行の表）が、
**「全フラグ」でも「本文が使うフラグ」でもない。** 本文が「必ず渡せ」と言っている
`--author`、本文の一節を丸ごと使う `--suggest`、`--label`、`--timeout`、
`--approve` / `--request-changes`、`--exit-code`、`-q` が表に無い。
一方で本文が一度も触れない `--no-open` `--title` `--include-resolved` は載っている。

### 直すもの

**issue の Expected の 2 択のうち「このワークフローが使うコマンド」を採る。**
これは既定として決めてある。**確認を上げずにこれで進めること。** 理由は issue 自身が
「スキルにはたぶんそちらが正しく、今より短くなる」と書いており、
完全なフラグ一覧は `sbnn <command> --help` が既に持っているため。

1. **見出しを、選んだほうが分かる名前にする。**
   例: `## The commands this workflow uses`。
   **「完全な一覧ではない」ことと、完全な一覧は `sbnn <command> --help` にあることを、
   表の直前に 1 行書く。** これが無いと、表に無いフラグを「存在しない」と読まれる
   （issue が指摘している一番の害）。
2. **本文が使うよう指示しているフラグを全部入れる。**
   `--author` `--suggest` `--question` `--label` `--timeout` `--target/-t`
   `--approve` `--request-changes` `--exit-code` `-q` `--format json`
   `--clear` `--json` `--collapse` `--fragment` `--port`。
   **入れる前に、本文を実際に読んで「本文が指示しているか」を確かめること。**
   本文が使っていないものは入れない。
3. **本文が一度も触れないものは落とす。** `--no-open` `--title` `--include-resolved` は
   落とすか、本文で触れているか確かめてから残すかを決める。**決めた理由を報告に書く。**
4. **表に載せる全フラグが実在することを、ビルドしたバイナリで 1 つずつ確認する。**
   存在しないフラグを載せるのは、この issue が直そうとしている問題そのもの。

**表を `references/` へ切り出すのはあなたの仕事ではない**（#113 / skill-split の担当）。
あなたは**同じファイルの中で表を直すだけ**。

### 完了条件

```bash
# 見出しが「全部」ではなく「このワークフローが使うもの」だと言っている
grep -n '^## ' skills/sbnn/SKILL.md | grep -i 'command'
grep -niE 'not (a )?complete|full list|--help' skills/sbnn/SKILL.md | head
# 本文が必ず渡せと言っているフラグが表にある
for f in -- author suggest question label timeout approve request-changes exit-code; do :; done
for f in author suggest question label timeout approve request-changes exit-code; do
  grep -q -- "--$f" skills/sbnn/SKILL.md && echo "ok --$f" || { echo "MISSING --$f"; false; }
done
# 表に書いた sbnn 呼び出しのフラグが全部実在する（未知フラグ 0 件が合格）
go build -o /tmp/sbnn-112 .
python3 - <<'PY'
import re,subprocess,sys
S='/tmp/sbnn-112'
h=subprocess.run([S,'--help'],capture_output=True,text=True).stdout
subs={m.group(1) for m in (re.match(r'^  ([a-z]+)\s{2,}',l) for l in h.splitlines()) if m}
cache={}
def F(c):
    if c not in cache:
        o=subprocess.run([S]+([c] if c else [])+['--help'],capture_output=True,text=True).stdout
        cache[c]=set(re.findall(r'(?<![\w-])(--[a-z][a-z0-9-]*)',o))
    return cache[c]
bad=[];n=0
for i,l in enumerate(open('skills/sbnn/SKILL.md'),1):
    s=re.sub(r'\$\([^)]*\)','X',l)
    for seg in s.split('|'):
        seg=seg.strip().strip('`')
        if not seg.startswith('sbnn'): continue
        toks=seg.split();n+=1
        sub=toks[1] if len(toks)>1 and not toks[1].startswith('-') else ''
        if sub not in subs: sub=''
        for t in toks[1:]:
            t=t.split('=')[0]
            if t.startswith('--') and t not in F(sub): bad.append((i,sub or '<root>',t))
print("invocations:",n,"unknown flags:",len(bad))
for b in bad: print("BAD",b)
sys.exit(1 if bad else 0)
PY
git diff --name-only origin/main            # => skills/sbnn/SKILL.md のみ
git diff --name-only origin/main -- '*.go'  # 何も返らないこと
go build ./... && go vet ./... && go test ./...
```

上の Python が `unknown flags: 0` で終了コード 0 になるのが合格。
**その出力を PR 本文の `## Verification` にそのまま貼ること。**

## 9. 全 PR 共通の検証（COMMON.md の「検証」に上乗せ）

```bash
go build ./... && go vet ./... && go test ./...
git diff --name-only origin/main             # 常に skills/sbnn/SKILL.md の 1 行だけ
git diff --name-only origin/main -- '*.go'   # 常に何も返らないこと
git status --short                            # 未追跡の残骸が無いこと
# 埋め込み側にも反映されている
go build -o /tmp/sbnn-chk . && diff <(/tmp/sbnn-chk skill) skills/sbnn/SKILL.md && echo "embed ok"
```

**「Go のコードは 1 バイトも変えない」の解釈:** このレーンは `.go` を新規追加も含めて
1 本も触らない。COMMON.md の「テストを必ず足す」は、
**このレーンでは #114（skill-split の担当）が引き受ける。**
各 PR 本文に「この変更は SKILL.md の文面のみで、対応する自動検査は #114 で入る」旨を
1 行書くこと（COMMON.md の「テストが物理的に書けない場合は理由を 1 行」の規定に沿う）。

## 10. 進め方の規約

- **判断に迷って止まらない。** 既定を自分で決めて進み、決めた内容と理由を報告に書く。
  #112 の「どちらの表にするか」は**既に §8 で決めてある。確認を上げない。**
- **担当外は触らない。** 見つけた問題は自分で直さず、報告の「見送り / 疑義」に書く。
  特に #25（hook が判定を受け取れない）、#55（export のスマホ幅）、#29（空の group が
  `null` を返す）は**見つけても直さない。**
- **書いた手順は実際に実行して通ることを確かめる。** SKILL.md に新しいコマンド例を書いたら、
  そのコマンドを自分で走らせる。走らせていないものは書かない。
  サーバを立てたら**必ず `--shutdown` で落としてから**次へ行く（並列で動いている他の
  レーンとポートを取り合わないこと）。
- ダッシュボードの更新（節目ごと）:
  ```bash
  gogodash task set --id t-49bb37 --status running --progress 30
  gogodash task log --id t-49bb37 --message "<何が起きたか 1 行>"
  gogodash task set --id t-49bb37 --status done --progress 100 --result "PR #<番号>"
  ```
  タスク ID は §1 の表のとおり
  （#109=`t-49bb37` / #111=`t-236122` / #110=`t-10c7a9` / #115=`t-94d2a0` / #112=`t-4db111`）。

## 11. 報告

COMMON.md の報告書式に従う。加えて、**完了報告に次の 4 つを、この指示文と同じ綴りでそのまま書くこと。**

```
slug     : skill-fix
worktree : /home/user/wt/skill-fix
branch   : gogo/issue-109 / gogo/issue-111 / gogo/issue-110 / gogo/issue-115 / gogo/issue-112
commit   : <各ブランチの短縮 SHA を 1 行ずつ>
```

**push と PR 作成まで行う。マージはしない。**
