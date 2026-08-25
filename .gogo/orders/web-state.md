slug: web-state

# 指示文 W-2 / web-state — 再読み込み・再接続で失われる状態

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-state`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 3 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**（#69 は P1、#93 / #96 は P2 だがレーンとしては P1 で回す）
- 期限: **2026-08-25 中**
- 出す順: #69 → #93 → #96

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #69 | `t-95d08f` | A page left open across a server restart shows stale data forever: the SSE client never refetches on reconnect |
| #93 | `t-d14dbd` | Which UI state survives a reload is arbitrary |
| #96 | `t-36d335` | There is no way to mark a file as read, so a large review has nowhere to keep your place |

```bash
gogodash task set --id t-95d08f --status running --progress 30
gogodash task log --id t-95d08f --message "<1 行>"
gogodash task set --id t-95d08f --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**主担当（あなたが持ち主）**

- `web/src/api.ts` — この波（G4）ではあなただけ
- `web/src/storage.ts` — この波（G4）ではあなただけ
- `web/src/components/Sidebar.tsx` — **あなたが持ち主。ただし web-nav が #66 のためだけに
  数行だけ触る**（下の「共有」を読むこと）

**共有（他のレーンも同じサイクルで触る。下の注意を守ること）**

- `web/src/App.tsx` — **web-nav（#61 #62 #63 #64 #66 #77 #91 #92）も同じファイルを触る。**
  状態の保存・復元と再取得の配線に必要な箇所だけを直す。
  **整形し直さない。import 順を並べ替えない。関係のない行を動かさない。**
- `web/src/components/Sidebar.tsx` は**あなたが持ち主**だが、web-nav が #66 のために
  `title="Remove this round"` のボタン 2 つとその `onClick` だけを触る。
  その付近を大きく作り替えるときは、**その 2 つのボタンの JSX を移動・改名しない。**

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
- `web/src/client.ts` — web-exportpage（#55 / #59 / #60）の持ち物。
  **#69 の直しを `client.ts` に入れない。**`api.ts` の `subscribe` と `App.tsx` の
  再取得の配線だけで完結させる（§5）。
- `web/src/components/DiffStack.tsx` / `web/src/components/DiffFileSection.tsx` /
  `web/src/shortcuts.ts` — web-nav の持ち物。
- `web/src/markdown.ts` / `web/src/notebook.ts` / `web/src/wordDiff.ts` /
  `web/src/components/CommentThread.tsx` / `web/src/components/Icon.tsx` /
  `web/src/main.tsx` / `web/vite.config.ts` /
  `web/src/components/PreviewStack.tsx` / `web/src/components/PreviewFileSection.tsx`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（§6）。
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

- **#69**: `web/src/api.ts` の `subscribe` は今、`EventSource` を作って `onmessage` だけを見ている。
  - `source.onopen` で `onChange()` を呼ぶ。**これで再接続のたびに再取得が走る。**
    初回接続でも呼ばれるが、二重取得 1 回は「永久に古いまま」より安い。**最適化しない。**
  - `source.onerror` は握り潰さない。`EventSource` は自分で再接続するので `close()` しない。
    再接続していることが分かるログ（`console.debug`）を 1 行だけ残す。
  - 「サーバが再起動した」判定を自前で作らない。**接続が開いたら取り直す、それだけ。**
- **#93**: どの UI 状態が再読み込みを越えるかを**表にして決め、実装をその表に合わせる。**
  - 越える: サイドバー幅（`sbnn.sidebar.width`）、分割比（`sbnn.split`）、
    プレビューの描画方式（`sbnn.preview.renderer`）、テーマ、サイドバーの表示形式（`sbnn.sidebar.layout`）、
    Split / Unified の表示モード、**読んだ印（#96）**
  - 越えない: 開いているコメント下書き、フィルタ文字列、フォーカス位置
  - **鍵の名前は `sbnn.` で始める。** 既存の 3 つに合わせる。読み書きは必ず
    `readSetting` / `writeSetting` を通す（`window.localStorage` を直接触らない）。
  - 壊れた値・範囲外の値は**既定へ落とす。例外を投げない。**
    数値は `Number.isFinite` と範囲で検査する。
  - 決めた表は PR 本文の `## What changed` にそのまま貼る。
- **#96**: ファイルに「読んだ」印を付けられるようにする。
  - 鍵は `sectionKey(diffId, fileId)`（`web/src/sectionKey.ts` は読むだけ。変更しない）。
  - 保存先は `localStorage`、鍵は `sbnn.read.<group>`。`writeSetting` を通す。
  - サイドバーの各ファイル行にトグルを出し、読んだファイルは見た目で分かるようにする。
    **`styles.css` を触れない**ので、既存のクラスを使い回すかインラインスタイルで済ませる。
  - 「全部を未読に戻す」導線を 1 つ置く。
  - 新しいラウンドが来たときに読んだ印を消さない（`sectionKey` はラウンドを含むので自然にそうなる）。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- 純粋関数（`storage.ts` の検査関数など）を直したときは、`node --experimental-strip-types` で
  叩ける使い捨ての確認スクリプトを**worktree の外**（`/tmp` 配下）に書いて実行し、
  **実際のコマンドと出力を PR の `## Verification` に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**3 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-state
git rev-parse --abbrev-ref HEAD             # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"   # => exit=0
cd /home/user/wt/web-state && git checkout -- web/dist
git status --porcelain -- web/dist                     # 何も出力しないのが合格
git show --stat HEAD | grep "web/dist" && echo "NG"    # 何も出力しないのが合格
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|client\.ts|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/'   # 何も出力しないのが合格
git diff --name-only origin/main..HEAD
#   許されるのは App.tsx / api.ts / storage.ts / components/Sidebar.tsx のみ
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（`grep` は 1 行以上返れば合格）:

```bash
cd /home/user/wt/web-state
# #69 接続が開いたら取り直している
grep -n "onopen" web/src/api.ts
# #69 EventSource を自分で閉じて再接続を止めていない（unsubscribe 以外に close が無い）
grep -n "close()" web/src/api.ts
# #93 直接 localStorage を触っていない（App.tsx / Sidebar.tsx で 0 件が合格）
grep -n "window.localStorage" web/src/App.tsx web/src/components/Sidebar.tsx
# #93 鍵が sbnn. で始まっている
grep -nE "'sbnn\.[a-z.]+'" web/src/App.tsx web/src/components/Sidebar.tsx
# #93 壊れた値を既定へ落としている
grep -n "Number.isFinite" web/src/App.tsx
# #96 読んだ印の鍵と保存
grep -n "sbnn.read" web/src/App.tsx web/src/components/Sidebar.tsx
grep -n "sectionKey" web/src/components/Sidebar.tsx
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-state && go run . --foreground
```

- #69: ブラウザで開いたまま `sbnn` サーバを再起動し、**何も触らずに**新しいコメントが現れることを見る。
  現れた時刻と、`console.debug` の再接続ログを報告に書く。
- #93: 表の「越える」項目を全部変えてから再読み込みし、全部残っていることを見る。
  「越えない」項目が残っていないことも見る。
- #96: ファイルに読んだ印を付けて再読み込みし、印が残っていることを見る。

## 8. 止まらないための決め

- **担当外は触らない。** 直したくなったら直さずに報告の「見送り / 疑義」へ書く。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-state | worktree /home/user/wt/web-state | issue #69 | PR #<番号> | branch gogo/issue-69 | commit <短縮 SHA> | <1 行要約>
...
```
