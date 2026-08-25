slug: ci

# ci — CI とサプライチェーンの土台（issue #71 / #126 / #102 / #131）

優先度: #126 = P0、#71 = P1、#102 = P1、#131 = P2
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `ci`）
  - `branch   = gogo/issue-<N>`（`<N>` は今扱っている issue 番号）
- **1 issue = 1 PR。** issue ごとに毎回 `origin/main` から切り直す。前の issue の変更を積み上げない。
- **push と PR 作成まで行う。マージはしない。**

## 担当ファイル（これ以外は 1 バイトも触らない）

| issue | このブランチで触ってよいファイル |
|---|---|
| #71  | `.github/workflows/ci.yml`（新規）**のみ** |
| #126 | `.github/workflows/security.yml`（新規）**のみ** |
| #102 | `renovate.json`（リポジトリ直下・新規）**のみ** |
| #131 | `.github/workflows/tagpr.yml`（既存を編集）**のみ** |

- **`Taskfile.yml` は触らない。** 別 issue（#127）の対応が別ブランチで進行中で、衝突する。
  CI からは既存の `task lint` / `task test` を呼ぶだけにする。
- **`aqua.yaml` は触らない。** 別レーン（release）が担当している。
- `go.mod` / `go.sum` / `web/` / `cmd/` / `internal/` は触らない。
- `web/dist/` は絶対にコミットしない。

## 全 issue 共通の禁止事項（重要）

**`git diff --exit-code web/dist` に相当する「dist が最新か」を検査するステップを、
どのワークフローにも入れてはいけない。** #71 と #126 と #131 の 3 本ともそれを要求しているが、
いまリポジトリでは「dist はコミットしない、波の切れ目にまとめて再生成する」という
運用に切り替わっており、この検査を入れると全 PR が赤くなる。
入れない理由をワークフロー中のコメント 1 行と PR 本文に必ず書くこと。

## issue #71 — CI がない（タスク ID: t-e4c38b）

作るもの: `.github/workflows/ci.yml` 1 本。

- `on:` は `pull_request` と `push: branches: ["main"]` の両方。
- `permissions: contents: read` を明示する。
- `concurrency` で同じ ref の古い実行をキャンセルする。
- ジョブ `go`（ubuntu-latest）:
  - `actions/checkout`（`persist-credentials: false`。メジャーは `.github/workflows/tagpr.yml`
    が既に使っているものと同じにする）
  - `actions/setup-go`（`go-version-file: go.mod`、モジュールキャッシュ有効）
  - `aquaproj/aqua-installer@v4.0.5`（`aqua_version: v2.62.3`。tagpr.yml と同じ値。
    これで `task` が入る）
  - `task lint` → `task test` → `go build ./...` の 3 ステップ。
- ジョブ `web`（ubuntu-latest）:
  - checkout → `pnpm/action-setup` → `actions/setup-node`（Node 22、pnpm キャッシュ、
    `cache-dependency-path: web/pnpm-lock.yaml`）
  - `working-directory: web` で `pnpm install --frozen-lockfile` → `pnpm run build`
  - `pnpm run build` は `tsc -b && vite build` なので、これが #71 の言う TypeScript の
    型検査そのものになる。別に `tsc --noEmit` ステップを足さない。
  - **ビルド後に dist を検査しない。** 上の共通禁止事項のとおり。
- setup-go / setup-node / pnpm/action-setup のバージョンは、**自分で最新の安定メジャーを決めて書く。**
  確認を上げない。決めた値と理由を報告に書く。

完了条件（自分で実行して真偽を決められること）:

```bash
test -f .github/workflows/ci.yml
grep -n "task lint" .github/workflows/ci.yml
grep -n "task test" .github/workflows/ci.yml
grep -n "pnpm run build" .github/workflows/ci.yml
grep -n "pull_request" .github/workflows/ci.yml
# 次の 2 つは「何も返らない」ことが合格（dist 検査を入れていない証明）
grep -n "web/dist" .github/workflows/ci.yml | grep -v "^.*#"
grep -n -- "--exit-code" .github/workflows/ci.yml
# YAML として読めること
python3 -c "import yaml;yaml.safe_load(open('.github/workflows/ci.yml'))"
```

コミット / PR のフッタは `Refs #71` にする（`Fixes #71` にしない）。
理由: issue の Expected に含まれる `git diff --exit-code web/dist` を意図的に入れないため、
issue は open のまま残す。この理由を PR 本文に 1 段落で書く。

## issue #126 — 脆弱性スキャンがない（タスク ID: t-62b8ee、**P0**）

作るもの: `.github/workflows/security.yml` 1 本。

- `on:` は `pull_request`、`push: branches: ["main"]`、`schedule`（週 1 回。cron は自分で決める。
  毎時 0 分ちょうどを避けた値にする）。
- `permissions: contents: read`。
- ジョブ `govulncheck`: checkout → `actions/setup-go`（`go-version-file: go.mod`）→
  `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
- ジョブ `pnpm-audit`: checkout → pnpm / Node をセットアップ →
  `working-directory: web` で `pnpm install --frozen-lockfile` → `pnpm audit --audit-level moderate`
- **dist の同期検査は入れない**（共通禁止事項）。
- **Dependabot のアラート有効化はリポジトリの設定であってファイルではない。**
  PR では手を出さず、「リポジトリ設定で `gomod` と `npm` の Dependabot alerts を
  有効化する必要がある」ことを報告に 1 行で書く。自分で設定を変えに行かない。

完了条件:

```bash
test -f .github/workflows/security.yml
grep -n "govulncheck" .github/workflows/security.yml
grep -n "pnpm audit" .github/workflows/security.yml
grep -n "schedule" .github/workflows/security.yml
grep -n -- "--exit-code" .github/workflows/security.yml   # 何も返らないのが合格
python3 -c "import yaml;yaml.safe_load(open('.github/workflows/security.yml'))"
```

さらに、いまの実際の状態を自分の手で確かめて報告に貼る（ワークフローの動作確認ではなく事実確認）:

```bash
go run golang.org/x/vuln/cmd/govulncheck@latest ./... ; echo "exit=$?"
cd web && pnpm audit --audit-level moderate ; echo "exit=$?"
```

ネットワークが理由で走らなかった場合は、走らなかったことをそのまま報告に書く。
**走らなかったのに「問題なし」と書かない。**

コミット / PR のフッタは `Refs #126`。理由: dist 同期検査と Dependabot alerts の有効化を
このPRに含めないため。issue は open のまま残す。

## issue #102 — Renovate の注釈だけあって設定がない（タスク ID: t-547b8b）

作るもの: リポジトリ直下の `renovate.json` 1 ファイル**のみ**。
`aqua.yaml` のコメントは**消さない**（設定ができた時点で注釈は正しくなる）。

内容は issue #131 が提示している案をそのまま採る。ただし次の 2 点だけ変える。

- `"automerge": true`（patch）は**入れない**。#131 自身が「CI ができるまでは外せ」と書いており、
  CI は同じ波で入るがまだ動作実績がない。入れない理由を PR 本文に 1 行書く。
- `vulnerabilityAlerts` の `"schedule": ["at any time"]` は**残す**。

カバーする対象は 4 つ: `gomod` / `npm`（`web/` 配下）/ `github-actions` / aqua レジストリの ref。
aqua の ref は `aqua.yaml` に既に `# renovate: depName=aquaproj/aqua-registry` の注釈があるので、
Renovate の regex/custom manager をこちらで書き足す必要があるかどうかを自分で判断して決める。
判断がどちらでも、決めた内容と理由を報告に書く。

完了条件:

```bash
test -f renovate.json
python3 -c "import json;d=json.load(open('renovate.json'));print(sorted(d))"
grep -n "gomod" renovate.json
grep -n "npm" renovate.json
grep -n "github-actions" renovate.json
grep -n "vulnerabilityAlerts" renovate.json
grep -n '"automerge"' renovate.json    # 何も返らないのが合格
git diff --name-only origin/main       # renovate.json の 1 行だけが出るのが合格
```

コミット / PR のフッタは `Fixes #102`。
この issue の Expected（「注釈が示す設定を入れる、3 つのエコシステムを覆う」）は
この PR で満たされる。

## issue #131 — 依存更新の仕組み（タスク ID: t-c38a40）

**この issue の renovate.json の部分は #102 の PR で出す。二重に作らない。**
このブランチで出すのは、#131 のもう半分、**GitHub Actions が可変タグに固定されている**問題だけ。

変えるもの: `.github/workflows/tagpr.yml` の `uses:` 2 行を、完全な commit SHA に固定し、
行末コメントに元のバージョンを残す。

```bash
# SHA はこうやって自分で解決する（推測で書かない）
git ls-remote https://github.com/actions/checkout        refs/tags/v7      'refs/tags/v7^{}'
git ls-remote https://github.com/aquaproj/aqua-installer refs/tags/v4.0.5  'refs/tags/v4.0.5^{}'
```

- 注釈付きタグなら `^{}` 側（peeled）の SHA を使う。
- 書式は `uses: actions/checkout@<40桁SHA> # v7` のように、バージョンを行末コメントに残す。
- `aqua_version: v2.62.3` は**そのまま**にする（aqua は導入後に自分でチェックサム検証をするため、
  固定すべき鎖の端はインストーラの側）。この判断を PR 本文に 1 行書く。
- `.github/workflows/ci.yml` と `security.yml` は**このブランチでは触らない**（別 issue のファイル）。

完了条件:

```bash
grep -nE "uses: .+@[0-9a-f]{40} # v" .github/workflows/tagpr.yml   # 2 行返るのが合格
grep -nE "uses: .+@v[0-9]" .github/workflows/tagpr.yml             # 何も返らないのが合格
python3 -c "import yaml;yaml.safe_load(open('.github/workflows/tagpr.yml'))"
git diff --name-only origin/main   # .github/workflows/tagpr.yml だけが出るのが合格
```

コミット / PR のフッタは `Refs #131`。issue は open のまま残す。
報告には次の 3 点を必ず書く（**issue へのコメントはメインが書く。あなたは書かない**）:

1. renovate.json は #102 の PR で入れた（PR 番号を書く）。
2. #131 が必須と書いている「lockfile が上がったら `web/dist` を再ビルドさせる CI 検査」は、
   いまの dist 運用（コミットしない・波の切れ目にまとめて再生成）と正面から衝突するので入れなかった。
   どちらの方針を採るかはリポジトリの決めごとなので、コードだけでは決まらない。
3. Actions の SHA 固定は入れた。

## 全体を通しての決まり

- 担当外のファイルは触らない。見つけた問題は自分で直さず報告に書く。
- 判断に迷って止まらない。既定を自分で決めて進み、決めた内容と理由を報告に書く。
- **issue へのコメントは書かない。** 判断と根拠（コードやコマンド出力の引用付き）を報告に書くだけ。
- Go を触っていないので `go test` は必須ではないが、`task lint` / `task test` が
  ローカルで通ることは 1 回確認して結果を報告に貼る。
- 報告には `slug` / branch / worktree / commit の 4 つを、この指示文と同じ綴りで書く。
  branch と commit は issue ごとに 4 組ある。
