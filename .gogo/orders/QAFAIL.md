## #137 Generated-file detection only understands English, so a Japanese "自動生成" header is never recognised
- task: t-ae70a9 / branch: gogo/issue-137 / worktree: /home/user/wt/diff-misc
- status: failed

QA(code) 不合格: 手書きコードの誤検知。再現= internal/diff で diff.GeneratedMarker("// ID を生成する。結果は変更しないこと。") が非空を返す(want "")。同様に "// このコードはテンプレートから生成されるデータを扱う" と "// この定数は変更しないこと。値は生成する側と揃える。"。いずれも server/fold.go で Folded=true になり実コードがレビューから外れる。修正案= jaGenerated から素の '生成する'/'生成され' を外し、英語側と同じく『生成された(もの)』の連体・完了形に限定する。'この(ファイル|コード)は.{0,30}生成さ' も、生成された対象がこのファイル自身であること(文末が生成され(た|ます|ました)で終わる)を要求する。判定OKだった正例12件(自動生成されたファイルです / protocにより生成されました / 編集不可: generator が上書きします / DO NOT EDIT など)は維持すること

## #31 The session file's `version` is written but never checked on load
- task: t-82b504 / branch: gogo/issue-31 / worktree: /home/user/wt/store
- status: failed

QA(code) 不合格: 前回の上書き破壊は解消済み。新規の無音データ喪失2件。再現(1)= XDG_STATE_HOME=<dir> で {"version":2,"seq":7,"groups":[...]} を session-<port>.json に置き --foreground 起動 → POST /_/api/groups/default/diffs → GET /_/api/status に sessionError キーが無い(空)。しかし追加したdiffはメモリのみで再起動後に消える。Status.SessionError は「ディスクに書かれていない理由。セッションファイルが最新の間だけ空」と定義されており契約違反。修正= Load が seal する箇所で s.persistErr に拒否エラーを入れる。再現(2)= 同じ起動状態でメッセージの指示どおり session ファイルを退避 → diff を追加 → 新しい session ファイルは作られず、ログにもstatusにも何も出ず、再起動で groups=[] 。修正= メッセージに「sbnn を再起動してください」を加える、または path が消えたら seal を解く。併せて (1)(2) それぞれに回帰テストを追加すること。参考: code-review が指摘した cmd/server.go:logError の競合(拒否行を死亡と誤判定しうる)は main 側にも既存のため本PRのブロッカーとはしないが後続issue推奨

## #56 `sbnn export` drops the verdict, the review note and reviewedAt
- task: t-124271 / branch: gogo/issue-56 / worktree: /home/user/wt/export-pkg
- status: failed

QA(code) 不合格 commit=dee8ec3 (PR#189)。issue #56 の Expected は「Payload に3項目を足し、createStaticClient.load から返し、buildPrompt にも渡す」の3点だが、入っているのは1点目（internal/export/export.go）だけで web 側が未着手。確認方法: web/src/client.ts の interface StaticPayload に version/saVersion/generatedAt/group/diffs/comments/previews/images しか無く review 3項目が無い。createStaticClient の load() は return { diffs: data.diffs ?? [], comments: read(), status: null } で reviewedAt / reviewVerdict を返さない。App.tsx:148-149 の setReviewedAt(data.reviewedAt ?? null) / setReviewVerdict(data.reviewVerdict ?? null) が常に null になるので、App.tsx:566 のボタンは 'Submit review' のまま、App.tsx:716 の 'Review submitted ...' バナーも出ない。static の prompt() も buildPrompt(group, diffs, comments) で note/verdict を渡していない。Go 側の変更自体は妥当なので、web 側（StaticPayload・load・buildPrompt）を足せば合格見込み。付随（中）: Payload は Group.Reviewed() 相当の「最後の submit より後に来た diff があるか」を持たないため、承認後に diff を1本足して export すると、web 側が読むようになった時点で古い Approved が新しい diff に対して表示される（exported page は status が常に null なので App.tsx:242 のフォールバックに落ちる）

## #57 The exported page's prompt is not the same text as `sbnn comments`, though it says it is
- task: t-511b9f / branch: gogo/issue-57 / worktree: /home/user/wt/export-pkg
- status: failed

QA(code) 不合格 commit=d80f302 (PR#209)。確認方法: (1) git checkout origin/main -- internal/server/prompt.go して go test ./internal/server/ → ok のまま。prompt.go の実変更は PromptOptions の JSON タグだけで、Go は既定でフィールド名を大文字小文字無視で照合するため挙動が変わらない。(2) grep -n 'same text as' cmd/export.go web/src/prompt.ts → 両方に「sbnn comments と同じテキスト」の主張が残っている。(3) grep -n 'approved|verdict|reviewer wrote' web/src/prompt.ts → 一致なし。issue が挙げた4つの欠落（verdict 文 / The reviewer wrote / 'N comment(s) came with the approval' / approved 用の締め）が全て未修正。(4) grep -rn testdata web/ → corpus を読む TS 側は存在しない。corpus 自体は有用な土台なので、buildPrompt を corpus に照らす TS テストと欠落4点の実装を足せば合格見込み。付随: prompt_test.go の DisallowUnknownFields は model.Comment の MarshalJSON が足す suggestions フィールドを弾くため、サーバの出力をそのまま fixture にできない

## #138 A suggestion block written inside another code fence is parsed as a real suggestion
- task: t-fa407e / branch: gogo/issue-138 / worktree: /home/user/wt/model
- status: failed

QA(code) 不合格: --suggest が既存の提案入り本文で新しい提案を無言で落とす(本PRが作り込んだ回帰)。再現手順: (1) git worktree add --detach wt e02879704f36029467fc2d346cefff2f3e4ba5b1; (2) internal/model に外部テストを置き、本文 = 三連バッククォートsuggestion 改行 三連バッククォートgo 改行 x 改行 三連バッククォート 改行 三連バッククォート に対し model.WithSuggestion(body, "REAL") を呼び、その結果を model.Suggestions に通す; (3) PR head では入れ子コードブロック1件のみが返り REAL が消える。merge-base 910e409 では2件返り REAL が残る。チルダ(~~~)版も同じ。原因: danglingFence が Suggestions と異なり suggestion ブロック内の入れ子フェンスを追跡しないため、内側の閉じフェンスでブロックが閉じたと誤判定し、本来の閉じフェンスを新たな未閉フェンスと見なして余分な閉じフェンスを付け足す。その余分なフェンスが引用ブロックを開き、新しい追記 suggestion をその中に飲み込む。差し戻し内容: danglingFence を Suggestions と同じ走査(suggestionFence + 入れ子追跡、非 suggestion ブロックは closesFence)に揃える。受け入れ条件として TestWithSuggestionRoundTrip に『本文が既にコードブロックを提案する suggestion を含む』行を足すこと(現在の表にこのケースが無いため素通りしている)。あわせて2件目: 未閉のフェンスの後ろにある suggestion が読めなくなる回帰も実測。Suggestions(三連バッククォートdiff 改行 -a 改行 +b 改行 空行 改行 三連バッククォートsuggestion 改行 real 改行 三連バッククォート) が PR head=0件、merge-base=[real]。エージェントが diff を引用してから提案を書く現実的な本文で提案が消えるので、この扱い(未閉フェンス以降を全て引用とみなす)が意図どおりか要判断。3件目(参考): web/src/suggestion.ts には今回の引用スキップが無いため、同じ本文でブラウザ側は提案ありと表示し Go 側は提案なしと扱う不一致が生じる。

## #38 `mo.Result.URLFor` silently falls back to the group URL, opening the wrong file's preview
- task: t-eda57e / branch: gogo/issue-38 / worktree: /home/user/wt/mo-proxy
- status: failed

QA(code) 不合格 PR#183. 再現1(空パスすり抜け): res := &mo.Result{URL:"http://g", Files: []mo.File{{URL:"http://f", Path:""}}}; res.URLFor("") は "" ではなく "http://f" を返す。修正は want=="" ガードを先頭ループより前へ出すか、ループ内で f.Path=="" を飛ばす。あわせて 'empty path' テストの fixture に Path:"" のエントリを足さないと固定できない。再現2(報告されない): internal/server/preview.go:161 で moURL が "" でも out.MoURL="" のまま error 無しで返るため、mo がファイルを skip した場合に 200 + 空URL となり、サーバ側に error もログも残らない。コミット表題の『Report』が未達

## #45 `sbnn comments --exit-code` ignores the verdict, unlike `sbnn wait` and `sbnn submit`
- task: t-7d82e6 / branch: gogo/issue-45 / worktree: /home/user/wt/cmd-wait
- status: failed

QA(code) 不合格（既に main へ merge 済みなので後追い修正が必要）: cmd/comments.go の 3 箇所が exitReview に切り替わったが、exitReview (cmd/wait.go:113) は g.ReviewVerdict をそのまま exitWithVerdict に渡すだけで、その verdict が現在のラウンドのものかを確認しない。ReviewVerdict は Store.SubmitReview でしか書かれず、AddDiff でも ClearComments でも消えない。再現[A]: submit --approve 済みの group に comments --clear して新しい diff を送り、未解決コメントを1件付けて submit しないと、sbnn comments -q が exit=0（実測。正しくは 1）。help が勧める 'sbnn wait && sbnn comments -q && git commit' が未解決コメントを踏み越える。再現[B]: submit --request-changes 済みの group では、次ラウンドでコメント 0 件でも exit=1 のまま。sbnn wait は runWait が g.Reviewed() で守っているため同じ状態で正しく待つ（実測 exit=124）ので、help が交換可能と案内する 2 コマンドが食い違う。修正案: exitReview で !g.Reviewed() のときは exitWithComments(g.Comments) に落とす。なお新テストの reviewedGroup は Diffs を持たないグループを組み立てるため Reviewed() が常に true になり、このケースを検出できない。あわせて cmd/comments_test.go:145 は子テストバイナリの終了コードをそのまま解釈するため、setup 失敗(=1)が want:1 の行を、Skip(=0) が want:0 の行を素通りさせる。cmd/comments.go:38 の例 '# 1 when there is something to address' も直下の新しい規則と矛盾したまま。

## #143 `--history-file false` writes a log to a file called "false"
- task: t-dda9b8 / branch: gogo/issue-143 / worktree: /home/user/wt/cmd-wait
- status: failed

QA(runtime) 不合格: 挙動は全て直っているが issue #143 Expected 第3項（ヘルプに受理語一式を書く）が未実施。再現: sbnn --help | grep history-file → 'Where submitted reviews are written down ("off" for nowhere, or $SBNN_HISTORY)'、sbnn reviews --help | grep history-file → 'Where the log is kept (or $SBNN_HISTORY)'。どちらも grep -ci 'false|disabled' = 0。git diff origin/main..HEAD --stat は cmd/server.go / cmd/util.go とその test のみでヘルプ文字列に未着手。HistoryOffWords が {off,none,no} → {off,none,no,false,0,disabled} と倍増した分だけ未文書の綴りが増えている。差し戻し内容: cmd/root.go:192 と cmd/reviews.go:77 の --history-file ヘルプ 1 行ずつに受理語一式と '-' が拒否される旨を書く。副次: サーバ起動済みのときだけ --history-file - が exit=0 で黙って無視される（ファイルは作られないので害は無いが文言も出ない）

## #61 `n` / `p` get stuck when a file holds three or more comments
- task: t-7f22f8 / branch: gogo/issue-61 / worktree: /home/user/wt/web-nav
- status: failed

QA(code) 不合格 commit=24c4573 (PR#256)。(1) 変更点に対応するテストが 0 本。web/package.json に test スクリプトも vitest も無く、*.test.ts も存在しない。stepToComment は純関数として切り出されているので、テストランナー（vitest 等）と stepToComment の単体テストを足せば守れる。確認方法: cd web && cat package.json / ls src/*.test.ts。(2) 論理自体は正しいことを QA 側で確認済み。npx tsc src/shortcuts.ts で JS に落とし、issue #61 の再現（stops=[c1(A),c2(A),c3(A),c4(B)]、活性ファイル A、goToKey が activeKey を対象ファイルに更新する前提）で n を6回押すと c1->c2->c3->c4->c1->c2、p は c3->c2->c1->c4->c3->c2。空配列・current が解決済みで消えた場合・活性ファイルにコメントが無い場合・端の巻き戻りも妥当。(3) 要修正: shortcuts.ts の findIndex にある s.key === activeKey の条件。App.tsx:322 が goToKey（DiffStack.tsx:122 の block:'start'）の 50ms 後に block:'center' で再スクロールし、DiffStack.tsx:157 の rootMargin -70%（ACTIVE_BAND=0.7）で判定するため、対象コメントがファイル先頭寄りだと onActiveChange={setActiveKey}（App.tsx:457）が読者の操作なしに前のファイルへ activeKey を戻しうる。そうなると次の n は記録した currentCommentId を使わず rejoin 分岐に落ちて逆走する。id は一意なので条件から activeKey を外すのが素直。実ブラウザでの発生確認は動作 QA 側に要依頼（コード読みまでが本判定の範囲）

## #65 Comment bodies are documented as Markdown but rendered as plain text in the browser
- task: t-6ae734 / branch: gogo/issue-65 / worktree: /home/user/wt/web-markdown
- status: failed

QA(visual) 不合格 commit=b26d297: 再現=(1) git diff | sbnn -t g (2) sbnn comment 'README.md:12' -m '![remote](https://example.com/tracker.png) and <img src="https://example.org/pixel.gif">' (3) 1280px でページを開き Playwright の request イベントを数える → localhost 以外へ4件。期待=コメント本文からの外部リクエストは0件。現行 origin/main の同手順は0件。Markdown 描画自体(強調/コード/リスト/リンク/コードフェンス)と onerror 除去は正しく動いているので、リモート画像の扱いだけを決めれば直る(相対パスのみ許可する、画像を描画しない、等)。証拠 .gogo/qa/web-markdown/visual/issue-65/pr250-remote-img.png と main-remote-img.png

## #23 A comment range that runs past the end of the diff is stored as given, and the comment becomes invisible
- task: t-9fc944 / branch: gogo/issue-23 / worktree: /home/user/wt/srv-api
- status: review

PR #274 (reworked)

## #25 Review hooks are never told the verdict
- task: t-b196a1 / branch: gogo/issue-25 / worktree: /home/user/wt/srv-hook
- status: review

PR #208 (reworked)

## #40 A patch that has not been applied is previewed as the old file, labelled "tree" and "complete"
- task: t-07ffb7 / branch: gogo/issue-40 / worktree: /home/user/wt/srv-preview
- status: failed

QA(code) 不合格: (1) combined diff で NewStart=0 のため worktreeMatchesNewSide が常に false になり、ディスク上の正しいファイルを捨てて diff のマーカー列が混入したテキスト（' alpha\n-bravo-feat\n+bravo-merged\n charlie\n+DELTA\n'）を complete=true として返す。再現: マージコミットを作り git show --cc HEAD -- doc.md | sbnn -t combined、その後 /content を GET。main は worktree で正しい内容を返す。NewStart==0 のハンクを持つファイルは「ハンク無し」と同様に素通しすること。(2) パッチ適用後にハンクより上へ 1 行挿入しただけで作業ツリーが不一致と判定され、部分再構成に落ちる。挿入行もハンク外の行も消えるのに complete=true。位置厳密一致ではなく「旧側と一致するときだけ拒否する」か、再構成が complete でないときは作業ツリーへ戻す方が安全。(3) 直っているのは報告された単純な未適用パッチのケースのみ。なお internal/export/export.go と internal/server/fold.go は source.NewSide を直接呼ぶため、エクスポート結果と画面が食い違う。

## #88 Relative images in a Markdown preview are always broken, and the server answers them with 200 text/html
- task: t-0c9008 / branch: gogo/issue-88 / worktree: /home/user/wt/srv-preview
- status: review

PR #210 (picked up)

## #89 A relative link in a Markdown preview opens the sbnn review page again in a new tab
- task: t-6883cd / branch: gogo/issue-89 / worktree: /home/user/wt/srv-preview
- status: review

PR #215 (picked up)

## #151 The event stream has no limit on subscribers and drops messages for anyone slow
- task: t-81e4e9 / branch: gogo/issue-151 / worktree: /home/user/wt/srv-core
- status: failed

QA(code) 不合格: sbnn wait が古いレビューを即返す(再現: submit --approve -> 新 diff 追加 -> sbnn wait は main rc=2/8s, PR rc=0/0s)

## #47 `sbnn hook` cannot remove one hook, although the server has a by-ID route
- task: t-b44856 / branch: gogo/issue-47 / worktree: /home/user/wt/cmd-hook
- status: failed

QA(code) 不合格: 再現手順 = サーバ起動後 'sbnn hook -p PORT --on-review "echo one"' で h2 を登録し、HOOK_ID を unset のまま 'sbnn hook -p PORT --remove "$HOOK_ID"' を実行すると rc=0・フック一覧を stdout に出力・h2 は削除されず残る。修正方針(QA は直さない): cmd/hook.go:88 の case を cmd.Flags().Changed("remove") で判定し、空 ID は明示的にエラーにする
