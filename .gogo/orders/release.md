slug: release

# release — タグからリリースバイナリを出す（issue #101）

優先度: P1
期限: 2026-08-25 中（この波の中で）

## 前提（先に読む）

- `/home/user/briefs/COMMON.md` を先に読む。手順・コミット・PR の書式はそこに従う。
- 本体 `/home/user/sbnn` は参照のみ。作業は自分の worktree の中だけ。
- 名前は次の式でだけ決める。自分で別名を付けない。
  - `worktree = /home/user/wt/<slug>`（`<slug>` はこのファイル冒頭の値 = `release`）
  - `branch   = gogo/issue-101`
- **1 issue = 1 PR。** `origin/main` から切る。
- **push と PR 作成まで行う。マージはしない。**

## 担当ファイル（これ以外は 1 バイトも触らない）

- `.goreleaser.yml`（新規）
- `.github/workflows/release.yml`（新規）
- `aqua.yaml`（既存を編集。goreleaser を 1 行足すだけ）

触ってはいけないもの:

- **`README.md` は触らない。** 別レーン（doc-readme）が担当している。
  issue #101 は「README をリリースページに向けろ」とも言っているが、それはこの PR に含めない。
- **`.github/workflows/tagpr.yml` は触らない。** 別 issue（#131）の担当ファイル。
- **`.github/workflows/ci.yml` / `security.yml` は触らない。** 別 issue（#71 / #126）の担当ファイル。
- **`Taskfile.yml` は触らない。** 別ブランチ（#127）と衝突する。
- **`renovate.json` は触らない。** 別 issue（#102）の担当ファイル。
- `go.mod` / `go.sum` / `version/` / `cmd/` / `internal/` / `web/` は触らない。
- `web/dist/` は絶対にコミットしない。

## issue #101 — go install 以外の導線がない（タスク ID: t-41154e）

作るもの: GoReleaser の設定と、タグ push で走るリリースワークフロー。

### `.goreleaser.yml`

- `version: 2`。
- `before.hooks` は**置かない**。`go mod tidy` や `task web` をリリース時に走らせない
  （`web/dist` はコミット済みで、`go:embed` はそれをそのまま使う。リリース時に Node を要求しない、
  というのがこのリポジトリの設計の要点）。この判断を設定ファイル中のコメント 1 行に残す。
- ビルド: `CGO_ENABLED=0`、`goos: [linux, darwin, windows]`、`goarch: [amd64, arm64]`。
  Windows の arm64 を落とすかどうかは自分で決めてよい。決めたら理由を報告に書く。
- ldflags でバージョンを埋める。埋める先は実在する 2 つの変数（`version/version.go` を読んで確かめる）:

```
-s -w
-X github.com/tenntenn/sbnn/version.Version={{.Version}}
-X github.com/tenntenn/sbnn/version.Revision={{.FullCommit}}
```

- `archives`: 既定は tar.gz、Windows だけ zip。
- `checksum`: `checksums.txt`。
- `release`: `prerelease: auto`。
- `changelog`: GitHub のものを使う。

### `.github/workflows/release.yml`

- `on: push: tags: ["v*"]`。`.tagpr` の `vPrefix = true` に合わせる。
- `permissions: contents: write`（リリース作成に要る。それ以上は付けない）。
- steps: `actions/checkout`（`fetch-depth: 0`。GoReleaser は履歴を要求する）→
  `actions/setup-go`（`go-version-file: go.mod`）→ `goreleaser/goreleaser-action`（`args: release --clean`、
  `env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`）。
- `actions/checkout` のメジャーは `.github/workflows/tagpr.yml` が使っているものに合わせる。
  他のアクションのバージョンは自分で最新の安定メジャーを決めて書く。**確認を上げない。**

### `aqua.yaml`

`packages:` に goreleaser を 1 行足す（`goreleaser/goreleaser` の安定版。バージョンは自分で決める）。
`registries:` とその `# renovate:` コメントは**触らない**。既存の 2 パッケージも**触らない**。

## 完了条件（自分で実行して真偽を決められること）

```bash
test -f .goreleaser.yml
test -f .github/workflows/release.yml
python3 -c "import yaml;yaml.safe_load(open('.goreleaser.yml'))"
python3 -c "import yaml;yaml.safe_load(open('.github/workflows/release.yml'))"
python3 -c "import yaml;yaml.safe_load(open('aqua.yaml'))"
grep -n "tenntenn/sbnn/version.Version" .goreleaser.yml
grep -n "tenntenn/sbnn/version.Revision" .goreleaser.yml
grep -n "goreleaser" aqua.yaml
grep -n 'tags:' .github/workflows/release.yml
grep -n "before:" .goreleaser.yml            # 何も返らないのが合格（hooks を置いていない証明）
git diff --name-only origin/main             # 上の 3 ファイルだけが出るのが合格
```

さらに、設定が本当に通ることを 1 回は機械に確かめさせる。次の順に試し、**通ったところまでを報告に書く**:

```bash
# 1) aqua で goreleaser が入るなら、これが本命
aqua i -l && goreleaser check && goreleaser build --snapshot --clean --single-target
# 2) aqua が使えないなら go run で
go run github.com/goreleaser/goreleaser/v2@latest check
# 3) どちらも環境の都合で走らないなら、最低限これを走らせて結果を貼る
go build -ldflags "-X github.com/tenntenn/sbnn/version.Version=v0.0.0-test" -o /tmp/sbnn-verify . && /tmp/sbnn-verify --version
```

1 も 2 も走らなかった場合は「走らなかった」とそのまま書く。
**走らせていないのに「設定は正しい」と書かない。**

## コミット / PR

フッタは `Refs #101` にする（`Fixes #101` にしない）。
理由: issue の Expected の後半「README をリリースページに向ける」を、
README が別レーンの担当ファイルであるため含めていない。issue は open のまま残す。
この理由を PR 本文に 1 段落で書く。

## 全体を通しての決まり

- 担当外のファイルは触らない。見つけた問題は自分で直さず報告に書く。
- 判断に迷って止まらない。既定を自分で決めて進み、決めた内容と理由を報告に書く。
- **issue へのコメントは書かない。** 判断と根拠を報告に書くだけ。メインが書く。
- 報告に、README に足すべき文面（インストール手順の追記案）を**そのまま貼れる形で**書いておく。
  別レーンが後で使う。ただし README ファイル自体は編集しない。
- 報告には `slug` / branch / worktree / commit の 4 つを、この指示文と同じ綴りで書く。
