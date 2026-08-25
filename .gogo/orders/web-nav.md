slug: web-nav

# 指示文 W-1 / web-nav — 差分ビューの移動・折りたたみ・選択の不具合

## 0. 先に読むもの（例外なし）

- `/home/user/briefs/COMMON.md` — 手順・コミット書式・PR 書式・検証コマンドはすべてここに従う。
- `/home/user/briefs/TASKIDS.tsv` — issue 番号とダッシュボードのタスク ID の対応表。

## 1. 名前

- worktree = `/home/user/wt/<この指示文の冒頭の slug>` すなわち `/home/user/wt/web-nav`
- branch = `gogo/issue-<N>`（issue ごとに 1 本。毎回 `origin/main` から切り直す）
- **1 issue = 1 PR。この指示文では 8 本の PR を作る。**
- **push と PR 作成まで行う。マージはしない。**

## 2. 優先度と期限

- 優先度: **P1**
- 期限: **2026-08-25 中**
- 8 本のうち先に出す順（詰まったら後ろを飛ばして先へ進む）: #91 → #92 → #62 → #63 → #61 → #64 → #66 → #77

## 3. 担当 issue とダッシュボードのタスク ID

| issue | task ID | 表題（詳細は issue 本文を必ず読む） |
|---|---|---|
| #91 | `t-3217f8` | You cannot select text in the diff: any click on a code cell opens a comment draft |
| #92 | `t-091d2b` | A line selection can only grow: clicking another line on the same side never starts a new range |
| #62 | `t-aaf3cc` | A file with comments cannot be folded: the fold button and `f` do nothing |
| #63 | `t-ced6f2` | Folding a file by hand claims "the sender asked for it" |
| #61 | `t-7f22f8` | `n` / `p` get stuck when a file holds three or more comments |
| #64 | `t-e24283` | Jumping to a file scrolls its header under the sticky toolbar |
| #66 | `t-29907b` | "Remove this round" deletes a diff and all its comments with no confirmation |
| #77 | `t-9e29e0` | On a phone the Diff tab shows one file and a screen of blank space, with no way to reach the next one |

作業の節目ごとにダッシュボードへ書く。

```bash
gogodash task set --id t-3217f8 --status running --progress 30
gogodash task log --id t-3217f8 --message "<1 行>"
gogodash task set --id t-3217f8 --status done --progress 100 --result "PR #<番号>"
```

## 4. 触ってよいファイル

**主担当（この波（G4）ではこの指示文だけが触る。次の波（G5）の web-perf / web-a11y / web-search が
`DiffStack.tsx` と `DiffFileSection.tsx` を引き継ぐが、G4 が先に終わるので衝突しない）**

- `web/src/components/DiffFileSection.tsx`
- `web/src/components/DiffStack.tsx`
- `web/src/shortcuts.ts`

**共有（他のレーンも同じサイクルで触る。下の注意を守ること）**

- `web/src/App.tsx` — **web-state（#69 / #93 / #96）も同じファイルを触る。**
  移動・キー操作・折りたたみ・確認ダイアログの配線に必要な箇所だけを直す。
  **整形し直さない。import 順を並べ替えない。関係のない行を動かさない。**
  他人の差分と衝突する面積を最小にすることが、この共有の唯一の条件である。
- `web/src/components/Sidebar.tsx` — **#66 のためだけ**。
  `Sidebar.tsx` の持ち主は web-state である。触ってよいのは
  `title="Remove this round"` を持つ 2 つのボタンとその `onClick`（現在の 210〜270 行付近）だけ。
  それ以外の行は 1 行も変えない。

**触ってはいけないファイル（例外なし）**

- `web/src/styles.css` — 別グループ（G2 / G3）が使用中。**1 バイトも触らない。**
- `web/src/client.ts` / `web/src/api.ts` / `web/src/storage.ts` — 他のレーンの持ち物。
- `web/src/markdown.ts` / `web/src/notebook.ts` / `web/src/wordDiff.ts` /
  `web/src/components/CommentThread.tsx` / `web/src/components/PreviewStack.tsx` /
  `web/src/components/PreviewFileSection.tsx` / `web/src/components/Icon.tsx` /
  `web/src/main.tsx` / `web/vite.config.ts`
- `web/dist/` — **絶対にコミットしない**（ビルド成果物。メインが波ごとにまとめて再生成する）。
- `web/package.json` / `web/pnpm-lock.yaml` — **npm の依存を増やさない**（理由は §6）。
- `go.mod` / `go.sum` / `Taskfile.yml` / `.github/` / `internal/` / `cmd/` / `docs/` / `skills/`

## 5. 直し方の既定（迷って止まらないための決め）

判断が要る場面はここで決めてある。**確認を上げずにこのとおり進め、違えたときだけ報告に理由を書く。**

- **#91**: コードセルの `mousedown`/`click` でいきなり下書きを開かない。
  「ドラッグして文字を選んだ」場合は下書きを開かない（`window.getSelection()` が
  空でない、またはポインタが押下位置から一定距離動いた場合）。
  下書きを開く導線は行番号セル（gutter）側に残す。**行番号セルのクリックは今までどおり効く。**
- **#92**: 同じ side の別の行をクリックしたら、**範囲を伸ばすのではなく新しい範囲を始める。**
  範囲を伸ばすのは Shift 併用のときだけ。
- **#62 / #63**: コメントを持つファイルも手で折りたためるようにする。
  手で折りたたんだ状態と、送り手が `--collapse` で指定した折りたたみを**別の状態として区別**し、
  ラベル（「the sender asked for it」の文言）は送り手指定のときだけ出す。
  区別は `sectionKey(diffId, fileId)` を鍵にする（`web/src/sectionKey.ts` は読むだけ。変更しない）。
- **#61**: `n` / `p` は「ファイル単位」ではなく「コメント単位」で進める。
  同じファイルに 3 件あるなら 3 回で抜ける。現在位置はコメント ID で覚える。
- **#64**: ジャンプ先はスティッキーツールバーの高さぶん手前で止める。
  高さは実測する（ツールバー要素の `getBoundingClientRect().height`）。
  **`styles.css` を触れないので、`scroll-margin-top` を CSS ファイルへ書く解決は採らない。**
  対象要素のインラインスタイルか `scrollTo` のオフセット計算で実現する。
- **#66**: 確認を挟む。**`window.confirm` で足りる**（新しいモーダル部品を作らない）。
  文言は英語で、消えるものを数えて言う
  （例: `Remove this round? 3 comments on it will be deleted too.`）。
- **#77**: 電話サイズ（幅 640px 未満）の Diff ペインに、前のファイル / 次のファイルへ進む導線を出す。
  **新しい CSS クラスを `styles.css` に足せない**ので、既存のクラスを使い回すか
  インラインスタイルで済ませる。それも無理なら**直さずに報告へ書く**（§8）。

## 6. テストについて（この波の共通の決め）

`web/` には JS のテストランナーが入っていない（`package.json` に vitest も jest も無い）。
`pnpm install --frozen-lockfile --offline` で動かす前提なので、**新しい npm 依存は足せない。**
テストランナーの導入は別のレーン（G10 / #120–#122）の仕事である。

したがって:

- **`web/` に `*.test.ts` を足さない。`package.json` と `pnpm-lock.yaml` を触らない。**
- 純粋関数を直したときは、`node --experimental-strip-types` で叩ける使い捨ての確認スクリプトを
  **worktree の外**（`/tmp` 配下）に書いて実行し、**実際のコマンドと出力を PR の `## Verification` に貼る。**
- PR 本文に次の 1 行を必ず入れる:
  `No unit test: web/ has no JS test runner and one cannot be added offline (see #120).`

## 7. 完了の判定条件（自分で実行して真偽を決められるもの）

**8 本すべてに共通**（`<N>` は issue 番号）:

```bash
cd /home/user/wt/web-nav
# 1) ブランチが origin/main から 1 本だけ生えている
git rev-parse --abbrev-ref HEAD            # => gogo/issue-<N>
git log --oneline origin/main..HEAD | wc -l # => 1
# 2) ビルドと型検査が通る（終了コード 0）
cd web && pnpm install --frozen-lockfile --offline && pnpm run build; echo "exit=$?"
# 3) dist が差分に入っていない（何も出力しないのが合格）
cd /home/user/wt/web-nav && git checkout -- web/dist
git status --porcelain -- web/dist
git show --stat HEAD | grep "web/dist" && echo "NG: dist をコミットしている"
# 4) 触ってはいけないファイルが差分に入っていない（何も出力しないのが合格）
git diff --name-only origin/main..HEAD | grep -E 'styles\.css|^web/(package\.json|pnpm-lock\.yaml)|^internal/|^cmd/|^go\.(mod|sum)'
# 5) 差分が担当ファイルの中だけに収まっている（下の一覧以外が出たら NG）
git diff --name-only origin/main..HEAD
#    許されるのは App.tsx / components/DiffFileSection.tsx / components/DiffStack.tsx /
#    shortcuts.ts / components/Sidebar.tsx のみ
# 6) PR が立っている
gh pr view gogo/issue-<N> --json number,baseRefName,headRefName
```

**issue ごとの確認**（`grep` は 1 行以上返れば合格。`<N>` の PR の差分に対して行う）:

```bash
cd /home/user/wt/web-nav
# #91 選択を潰さない分岐が入った
grep -n "getSelection" web/src/components/DiffFileSection.tsx
# #92 Shift でだけ範囲を伸ばす分岐が入った
grep -n "shiftKey" web/src/components/DiffFileSection.tsx
# #62/#63 手動の折りたたみと送り手指定を別々に持っている
grep -nE "collapsedBySender|senderCollapsed|manualFold|foldOverride" web/src/components/DiffStack.tsx web/src/components/DiffFileSection.tsx web/src/App.tsx
# #61 コメント単位で進んでいる（ファイル単位の index ではない）
grep -n "commentId\|c.id ===" web/src/App.tsx
# #64 ツールバーの高さを実測している
grep -n "getBoundingClientRect" web/src/App.tsx web/src/components/DiffStack.tsx
# #66 確認が入った
grep -n "window.confirm\|confirm(" web/src/components/Sidebar.tsx
# #66 で Sidebar の他の場所を触っていない（変更行数が 20 行を超えていたら見直す）
git diff --stat origin/main..HEAD -- web/src/components/Sidebar.tsx
# #77 電話幅の導線が入った
grep -nE "useMediaQuery|max-width: *639|640" web/src/App.tsx web/src/components/DiffStack.tsx
```

**手で確かめること（PR の `## Verification` に、やった操作と見えた結果を書く）**

```bash
cd /home/user/wt/web-nav && go run . --foreground   # 別端末でブラウザから開く
```

- #91: 差分の行をドラッグして文字が選べる。ドラッグしただけでは下書きが開かない。
- #92: 同じ side の離れた行をクリックすると、範囲が新しく始まる。
- #62: コメントを持つファイルで `f` とボタンの両方が折りたたむ。
- #66: 「Remove this round」で確認が出て、キャンセルすると何も消えない。

## 8. 止まらないための決め

- **担当外は触らない。** 直したくなったら直さずに報告の「見送り / 疑義」へ書く。
- **issue の前提が事実と違う / 再現しない / 仕様の決めが要る**ときは、無理に直さず
  コードの引用付きで報告へ書き、**次の issue へ進む。1 本で全体を止めない。**
- issue へのコメントは書かない（メインが書く）。
- **判断に迷ったら §5 の既定を採る。確認を上げない。**
  §5 に無い判断をしたときは、決めた内容と理由を報告に 1 行で書く。

## 9. 報告（最終出力）

`/home/user/briefs/COMMON.md` の「報告」の書式で返す。加えて、`## 完了` の各行に
**この指示文と同じ綴りの `slug` と worktree** を添える。

```
## 完了
- slug web-nav | worktree /home/user/wt/web-nav | issue #91 | PR #<番号> | branch gogo/issue-91 | commit <短縮 SHA> | <1 行要約>
...
```
