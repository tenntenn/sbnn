# Changelog

From the first tagged release onwards, the sections below are maintained by
[tagpr](https://github.com/Songmu/tagpr): it collects the merged pull requests
into the release pull request it opens, and that is what lands here. Entries
under `## Unreleased` are written by hand, and anything about a persisted
format belongs under [Format changes](#format-changes) as well.

## [v0.0.1](https://github.com/tenntenn/sbnn/commits/v0.0.1) - 2026-08-26

- Add sa: a stdin-driven local diff review server by @tenntenn in https://github.com/tenntenn/sbnn/pull/1
- Add tagpr for automated release PRs by @tenntenn in https://github.com/tenntenn/sbnn/pull/2
- README says mo is optional, not required by @tenntenn in https://github.com/tenntenn/sbnn/pull/4
- MakefileからTaskfileへ移行し、aquaでツール管理 by @tenntenn in https://github.com/tenntenn/sbnn/pull/3
- Rename sa to sbnn by @tenntenn in https://github.com/tenntenn/sbnn/pull/6
- Show other groups on the welcome page by @tenntenn in https://github.com/tenntenn/sbnn/pull/7
- Stop calling a round a group, and document reviewing a PR stack by @tenntenn in https://github.com/tenntenn/sbnn/pull/8
- Redesign the review toolbar: icons, a settings menu, and divider handles by @tenntenn in https://github.com/tenntenn/sbnn/pull/9
- Show every file in one continuous scroll, diff and preview both by @tenntenn in https://github.com/tenntenn/sbnn/pull/10
- Comment on the preview by selecting text in it by @tenntenn in https://github.com/tenntenn/sbnn/pull/11
- Add a screenshot of the review page to the README by @tenntenn in https://github.com/tenntenn/sbnn/pull/12
- Fix the settings menu rendering under the preview toolbar by @tenntenn in https://github.com/tenntenn/sbnn/pull/13
- Preview images and Jupyter notebooks, not only Markdown by @tenntenn in https://github.com/tenntenn/sbnn/pull/14
- Tidy go.mod so cobra and pkg/browser are not marked indirect by @tenntenn in https://github.com/tenntenn/sbnn/pull/163
- Add a security policy with sbnn's threat model written down by @tenntenn in https://github.com/tenntenn/sbnn/pull/214
- Give the stylesheet a token layer and spend every value from it by @tenntenn in https://github.com/tenntenn/sbnn/pull/211
- Read the build revision from the build info by @tenntenn in https://github.com/tenntenn/sbnn/pull/191
- Match --collapse patterns with any number of "**" by @tenntenn in https://github.com/tenntenn/sbnn/pull/188
- Read every marker column of a combined diff, not just the first by @tenntenn in https://github.com/tenntenn/sbnn/pull/204
- Trim --label pairs and refuse a key given twice by @tenntenn in https://github.com/tenntenn/sbnn/pull/216
- Refuse sbnn hook --clear together with a registration by @tenntenn in https://github.com/tenntenn/sbnn/pull/206
- Refuse to export over a file sbnn did not write by @tenntenn in https://github.com/tenntenn/sbnn/pull/200
- Let the verdict decide the exit status of sbnn comments by @tenntenn in https://github.com/tenntenn/sbnn/pull/193
- Reject an unknown wait --format before the wait, not after it by @tenntenn in https://github.com/tenntenn/sbnn/pull/178
- Accept --side spellings that differ only in case or padding by @tenntenn in https://github.com/tenntenn/sbnn/pull/164
- Accept a whole line number written as 12.0 in --json by @tenntenn in https://github.com/tenntenn/sbnn/pull/170
- Report why the server did not start instead of timing out by @tenntenn in https://github.com/tenntenn/sbnn/pull/234
- Count runes, not bytes, when shortening a fold reason by @tenntenn in https://github.com/tenntenn/sbnn/pull/198
- Refuse a line number too large to hold instead of clamping it by @tenntenn in https://github.com/tenntenn/sbnn/pull/175
- Spend the interactive colour on one meaning and give the rest their own by @tenntenn in https://github.com/tenntenn/sbnn/pull/218
- Say which comments were stored when a batch fails part-way by @tenntenn in https://github.com/tenntenn/sbnn/pull/196
- Keep a broken session file instead of overwriting it by @tenntenn in https://github.com/tenntenn/sbnn/pull/194
- Report a session file that cannot be written instead of failing silently by @tenntenn in https://github.com/tenntenn/sbnn/pull/171
- Stop papering over --ok-bg and --bg-elevated with var() fallbacks by @tenntenn in https://github.com/tenntenn/sbnn/pull/228
- Leave the sender's round title in the case they typed it by @tenntenn in https://github.com/tenntenn/sbnn/pull/235
- Split the badge pill by what it is saying by @tenntenn in https://github.com/tenntenn/sbnn/pull/238
- Number rounds from a counter that only goes up, per group by @tenntenn in https://github.com/tenntenn/sbnn/pull/205
- Add comments --clear --resolved-only to keep what is still open by @tenntenn in https://github.com/tenntenn/sbnn/pull/202
- Keep a suggestion that proposes a code block whole by @tenntenn in https://github.com/tenntenn/sbnn/pull/186
- Let the remote review page write, not just read by @tenntenn in https://github.com/tenntenn/sbnn/pull/229
- Refuse a comment whose fileId names no file of its diff by @tenntenn in https://github.com/tenntenn/sbnn/pull/269
- Skip an over-long line instead of losing the whole log by @tenntenn in https://github.com/tenntenn/sbnn/pull/192
- Reject hunk header numbers that carry a sign of their own by @tenntenn in https://github.com/tenntenn/sbnn/pull/180
- Name a file the diff never identified instead of leaving its path empty by @tenntenn in https://github.com/tenntenn/sbnn/pull/169
- Keep the drop shadow for things that actually float by @tenntenn in https://github.com/tenntenn/sbnn/pull/243
- Give the corner radii a rule to be consistent with by @tenntenn in https://github.com/tenntenn/sbnn/pull/248
- Refuse --json next to the flags that hold a single comment's text by @tenntenn in https://github.com/tenntenn/sbnn/pull/185
- Reject comments whose line range cannot exist by @tenntenn in https://github.com/tenntenn/sbnn/pull/263
- Let the reader fold a file that carries comments by @tenntenn in https://github.com/tenntenn/sbnn/pull/250
- Keep a text selection in the diff from opening a comment draft by @tenntenn in https://github.com/tenntenn/sbnn/pull/241
- Read a bare --since date in the local zone by @tenntenn in https://github.com/tenntenn/sbnn/pull/173
- Let a plain click start a new line range instead of only growing one by @tenntenn in https://github.com/tenntenn/sbnn/pull/246
- Read only a whole "7d" as days in ParseSince by @tenntenn in https://github.com/tenntenn/sbnn/pull/167
- Document the real state and cache directories per platform by @tenntenn in https://github.com/tenntenn/sbnn/pull/182
- Accept every separator spelling of a verdict, not four of five by @tenntenn in https://github.com/tenntenn/sbnn/pull/166
- Stop transitioning a property the divider handle never sets by @tenntenn in https://github.com/tenntenn/sbnn/pull/251
- Validate the comment side instead of folding it into "new" by @tenntenn in https://github.com/tenntenn/sbnn/pull/267
- Confirm before --clear drops reviews that still hold work by @tenntenn in https://github.com/tenntenn/sbnn/pull/212
- Make the round tab's remove control a sibling button by @tenntenn in https://github.com/tenntenn/sbnn/pull/290
- Reject --all when --clear is not given by @tenntenn in https://github.com/tenntenn/sbnn/pull/222
- Stop dropping hunk body lines the header did not count by @tenntenn in https://github.com/tenntenn/sbnn/pull/190
- Stop sending the raw diff text nobody reads by @tenntenn in https://github.com/tenntenn/sbnn/pull/244
- Count diff, comment and hook ids separately by @tenntenn in https://github.com/tenntenn/sbnn/pull/181
- Honour --stats in every output format, not only text by @tenntenn in https://github.com/tenntenn/sbnn/pull/187
- Inline the icon font so exported pages keep their glyphs by @tenntenn in https://github.com/tenntenn/sbnn/pull/298
- Append to the review log under an exclusive file lock by @tenntenn in https://github.com/tenntenn/sbnn/pull/213
- Inline the entry module index.html names, not every .js in assets by @tenntenn in https://github.com/tenntenn/sbnn/pull/179
- Stop injecting an HTML comment into reconstructed previews by @tenntenn in https://github.com/tenntenn/sbnn/pull/199
- Default the port from the scheme when matching the mo endpoint by @tenntenn in https://github.com/tenntenn/sbnn/pull/172
- Read the review body when the Content-Length is unknown by @tenntenn in https://github.com/tenntenn/sbnn/pull/279
- Let the preview pane shrink to the screen on a phone by @tenntenn in https://github.com/tenntenn/sbnn/pull/273
- Indent a comment card by the gutter its own view mode has by @tenntenn in https://github.com/tenntenn/sbnn/pull/275
- Give the agent-comment marker a border it can actually paint by @tenntenn in https://github.com/tenntenn/sbnn/pull/278
- Dim the prose of a resolved comment, not its controls by @tenntenn in https://github.com/tenntenn/sbnn/pull/280
- Size the disclosure box from the glyph it holds by @tenntenn in https://github.com/tenntenn/sbnn/pull/282
- Floor the comment count at the pill's own height so one digit is a circle by @tenntenn in https://github.com/tenntenn/sbnn/pull/289
- Always answer with arrays for a group's diffs and comments by @tenntenn in https://github.com/tenntenn/sbnn/pull/281
- Resolve symlinks before deciding a diff path stays in the base dir by @tenntenn in https://github.com/tenntenn/sbnn/pull/177
- Tell the reader to run task build, not make build by @tenntenn in https://github.com/tenntenn/sbnn/pull/221
- Point the web package doc at task web instead of the Makefile by @tenntenn in https://github.com/tenntenn/sbnn/pull/165
- Give the README an index and a place to look flags up by @tenntenn in https://github.com/tenntenn/sbnn/pull/230
- Check the README's command line claims against the binary by @tenntenn in https://github.com/tenntenn/sbnn/pull/237
- Turn on tagpr changelog generation and start a CHANGELOG by @tenntenn in https://github.com/tenntenn/sbnn/pull/247
- Tell review hooks the verdict by @tenntenn in https://github.com/tenntenn/sbnn/pull/208
- Add issue and PR templates, .editorconfig and a code of conduct by @tenntenn in https://github.com/tenntenn/sbnn/pull/245
- Show a filled suggestion block in the skill's --suggest example by @tenntenn in https://github.com/tenntenn/sbnn/pull/217
- Run lint, tests and both builds on every pull request by @tenntenn in https://github.com/tenntenn/sbnn/pull/227
- Pin the release workflow's actions to commit SHAs by @tenntenn in https://github.com/tenntenn/sbnn/pull/233
- Scan both ecosystems for known vulnerabilities on every push and weekly by @tenntenn in https://github.com/tenntenn/sbnn/pull/225
- Configure Renovate so the annotation in aqua.yaml means something by @tenntenn in https://github.com/tenntenn/sbnn/pull/231
- Check the working tree against the diff before trusting it by @tenntenn in https://github.com/tenntenn/sbnn/pull/203
- Say which comment JSON keys are always there and which are not by @tenntenn in https://github.com/tenntenn/sbnn/pull/232
- Fail the build when a design token is used but never defined by @tenntenn in https://github.com/tenntenn/sbnn/pull/260
- Scope the command table to what the workflow tells you to pass by @tenntenn in https://github.com/tenntenn/sbnn/pull/242
- Render comment prose as Markdown in the browser by @tenntenn in https://github.com/tenntenn/sbnn/pull/219
- Document the --collapse separators and what the comma costs by @tenntenn in https://github.com/tenntenn/sbnn/pull/272
- Pin the exported surface of internal/diff and propose waiting on v1 by @tenntenn in https://github.com/tenntenn/sbnn/pull/307
- Propose an MCP server that adds no dependency by @tenntenn in https://github.com/tenntenn/sbnn/pull/304
- Propose how to expand the context around a hunk by @tenntenn in https://github.com/tenntenn/sbnn/pull/295
- Propose how a comment gets a reply by @tenntenn in https://github.com/tenntenn/sbnn/pull/297
- Propose sbnn ls before any desktop wrapper by @tenntenn in https://github.com/tenntenn/sbnn/pull/301
- Build notebook SVG outputs as a standard base64 data URL by @tenntenn in https://github.com/tenntenn/sbnn/pull/259
- Refetch on every SSE connect so a reconnected page is not stale by @tenntenn in https://github.com/tenntenn/sbnn/pull/254
- Put the file being read into the URL, and honour it on arrival by @tenntenn in https://github.com/tenntenn/sbnn/pull/288
- Key the preview loader on the file's content, not its object identity by @tenntenn in https://github.com/tenntenn/sbnn/pull/286
- Size a framed preview to what is in it, not to 80vh by @tenntenn in https://github.com/tenntenn/sbnn/pull/293
- Stop mounting empty preview sections and skip offscreen diff sections by @tenntenn in https://github.com/tenntenn/sbnn/pull/287
- Give reload persistence one rule and follow it by @tenntenn in https://github.com/tenntenn/sbnn/pull/262
- Recognise Japanese self-declarations of generated files by @tenntenn in https://github.com/tenntenn/sbnn/pull/174
- Colour the diff by syntax, without taking on a highlighter by @tenntenn in https://github.com/tenntenn/sbnn/pull/300
- Treat false, 0 and disabled as "keep no history", and refuse "-" by @tenntenn in https://github.com/tenntenn/sbnn/pull/168
- Add sbnn hook --remove to drop a single hook by ID by @tenntenn in https://github.com/tenntenn/sbnn/pull/195
- Report a missing mo deep link instead of the group URL by @tenntenn in https://github.com/tenntenn/sbnn/pull/183
- Build with Go 1.27, so govulncheck can run again by @tenntenn in https://github.com/tenntenn/sbnn/pull/309
- Refuse a session file written by a newer sbnn by @tenntenn in https://github.com/tenntenn/sbnn/pull/184
- Show the verdict in the reviews listing and tally it in --stats by @tenntenn in https://github.com/tenntenn/sbnn/pull/176
- Step n and p through comments, not through the files holding them by @tenntenn in https://github.com/tenntenn/sbnn/pull/256
- Stop crediting the sender for a fold the reader performed by @tenntenn in https://github.com/tenntenn/sbnn/pull/252
- Carry the verdict, the note and reviewedAt into the exported payload by @tenntenn in https://github.com/tenntenn/sbnn/pull/189
- Pin the prompt wording with a corpus both renderers can read by @tenntenn in https://github.com/tenntenn/sbnn/pull/209
- Tell the reviewer when an exported page cannot save their comments by @tenntenn in https://github.com/tenntenn/sbnn/pull/303
- Take the rewrites go fix suggests, and keep them taken by @tenntenn in https://github.com/tenntenn/sbnn/pull/310
- Search hunk content as well as paths in the sidebar filter by @tenntenn in https://github.com/tenntenn/sbnn/pull/291
- Keep sidebar paths in logical order when truncating from the left by @tenntenn in https://github.com/tenntenn/sbnn/pull/270
- Split word-level diff on grapheme clusters, not code units by @tenntenn in https://github.com/tenntenn/sbnn/pull/253
- Cap event subscribers and stop losing the review notice by @tenntenn in https://github.com/tenntenn/sbnn/pull/236
- Say a body is too large instead of blaming its JSON by @tenntenn in https://github.com/tenntenn/sbnn/pull/240
- Add a browser rendering test harness for the review UI by @tenntenn in https://github.com/tenntenn/sbnn/pull/257
- Check every colour token against every surface it is painted on by @tenntenn in https://github.com/tenntenn/sbnn/pull/264
- Make the Install section about installing sbnn by @tenntenn in https://github.com/tenntenn/sbnn/pull/224
- Add CONTRIBUTING.md and point the README's Development section at it by @tenntenn in https://github.com/tenntenn/sbnn/pull/258
- Give the reader a page when they cannot open a localhost URL by @tenntenn in https://github.com/tenntenn/sbnn/pull/239
- List every hook variable the server sets, without pinning a count by @tenntenn in https://github.com/tenntenn/sbnn/pull/226
- Add table tests for the pure helpers of the cmd package by @tenntenn in https://github.com/tenntenn/sbnn/pull/276
- Render the SPA only for the root and a group name by @tenntenn in https://github.com/tenntenn/sbnn/pull/215
- Propose the order the non-terminal entry points come in by @tenntenn in https://github.com/tenntenn/sbnn/pull/299
- Let a verdict decide only the round it was given for by @tenntenn in https://github.com/tenntenn/sbnn/pull/308
- Fix the addition and deletion counts a combined diff reports by @tenntenn in https://github.com/tenntenn/sbnn/pull/292
- Split the reference material out of SKILL.md into references/ by @tenntenn in https://github.com/tenntenn/sbnn/pull/284
- Check the skill against the command line it documents by @tenntenn in https://github.com/tenntenn/sbnn/pull/277
- Drop group names the session file should not contain by @tenntenn in https://github.com/tenntenn/sbnn/pull/207
- Answer 404 when deleting a hook that is not there by @tenntenn in https://github.com/tenntenn/sbnn/pull/285
- Clamp a comment range to the last line the diff really shows by @tenntenn in https://github.com/tenntenn/sbnn/pull/274
- Log a line per request, behind SBNN_LOG by @tenntenn in https://github.com/tenntenn/sbnn/pull/249
- Let the server end itself once it holds nothing to review by @tenntenn in https://github.com/tenntenn/sbnn/pull/255
- Report a failed stdin stat instead of reading it as no diff by @tenntenn in https://github.com/tenntenn/sbnn/pull/201
- Read the export payload's version under its current name by @tenntenn in https://github.com/tenntenn/sbnn/pull/306
- Do not read a quoted suggestion block as a proposed change by @tenntenn in https://github.com/tenntenn/sbnn/pull/197
- Stop a jump from parking a file header under the diff toolbar by @tenntenn in https://github.com/tenntenn/sbnn/pull/261
- Ask before removing a round, and say so when it fails by @tenntenn in https://github.com/tenntenn/sbnn/pull/266
- Let the reader mark a file read and keep their place by @tenntenn in https://github.com/tenntenn/sbnn/pull/268
- Name the browser tab after the review it is showing by @tenntenn in https://github.com/tenntenn/sbnn/pull/283
- Write the sample review note in the language the README is written in by @tenntenn in https://github.com/tenntenn/sbnn/pull/314
- Reshoot the README screenshot and put it beside the sentence it illustrates by @tenntenn in https://github.com/tenntenn/sbnn/pull/313
- Let a comment start from the keyboard on the line gutter by @tenntenn in https://github.com/tenntenn/sbnn/pull/294
- Describe the file-list search that #291 actually shipped by @tenntenn in https://github.com/tenntenn/sbnn/pull/316
- Subscribe before asking whether the review already happened by @tenntenn in https://github.com/tenntenn/sbnn/pull/223
- Give the phone diff pane a way on to the next file by @tenntenn in https://github.com/tenntenn/sbnn/pull/271
- Let a comment be about a file rather than about a line of it by @tenntenn in https://github.com/tenntenn/sbnn/pull/320
- Preview a source file as its own syntax coloured lines by @tenntenn in https://github.com/tenntenn/sbnn/pull/321
- Carry the images a Markdown preview points at into the page by @tenntenn in https://github.com/tenntenn/sbnn/pull/322
- Draw a file that has no hunks instead of blanking the page by @tenntenn in https://github.com/tenntenn/sbnn/pull/326
- Rebuild web/dist from the source that guards an empty hunks array by @tenntenn in https://github.com/tenntenn/sbnn/pull/325
- Refuse a hook url that cannot be delivered to, and list how each hook went by @tenntenn in https://github.com/tenntenn/sbnn/pull/220
- Attach cross compiled binaries to every tag, and stamp the version from it by @tenntenn in https://github.com/tenntenn/sbnn/pull/332
- Follow a relative preview link to the file it means, or say it is not here by @tenntenn in https://github.com/tenntenn/sbnn/pull/265
- Name the selective clear in step 7 of the skill by @tenntenn in https://github.com/tenntenn/sbnn/pull/15
- Describe the HTTP API, and check the description against the types by @tenntenn in https://github.com/tenntenn/sbnn/pull/333
- Rebuild web/dist so the shipped page follows relative preview links by @tenntenn in https://github.com/tenntenn/sbnn/pull/338
- Stop tracking tsc's incremental build cache by @tenntenn in https://github.com/tenntenn/sbnn/pull/348
- Run the review UI tests in CI, and make task test mean the same thing by @tenntenn in https://github.com/tenntenn/sbnn/pull/349
- Put the line gutter on the keyboard help sheet by @tenntenn in https://github.com/tenntenn/sbnn/pull/350
- Hold an image of the diff to the same size cap as a sibling image by @tenntenn in https://github.com/tenntenn/sbnn/pull/353
- Read the API's error body as JSON instead of showing it by @tenntenn in https://github.com/tenntenn/sbnn/pull/351
- Resolve preview links against the path the review is keyed by by @tenntenn in https://github.com/tenntenn/sbnn/pull/352
- Write down where docs/screenshot.png comes from, and check it by @tenntenn in https://github.com/tenntenn/sbnn/pull/347
- Describe the visual suite that exists, and hold the description to it by @tenntenn in https://github.com/tenntenn/sbnn/pull/346
- Let a jump to a file outrank the scroll rule that picks the active one by @tenntenn in https://github.com/tenntenn/sbnn/pull/345
- Name --resolved-only in both of the README's --clear places by @tenntenn in https://github.com/tenntenn/sbnn/pull/340
- Say in docs/api.md that the event replay is per group, and hold it by @tenntenn in https://github.com/tenntenn/sbnn/pull/343
- Tell tagpr to write no version file, rather than the wrong one by @tenntenn in https://github.com/tenntenn/sbnn/pull/341
- Give task lint a vet tool of its own, starting with the no-git rule by @tenntenn in https://github.com/tenntenn/sbnn/pull/344
- Rebuild web/dist, and unpin the defect the rebuilt bundle closes by @tenntenn in https://github.com/tenntenn/sbnn/pull/359

## Unreleased

Nothing released yet. `version.Version` is still `dev` and the repository has
no tags, so every build is from source and there is no version number to
compare against.

## Format changes

sbnn writes three things that outlive the process, and each makes a promise
about how it may change. When one of them does change, record it here — which
version, which field, what a reader of the old shape should do. Only changes
from a tagged release onwards can be recorded; nothing below is filled in
retroactively, because there is no release history to reconstruct it from.

### Session file

The session sbnn keeps between runs, written by `internal/server/store.go`. Its
JSON carries an explicit schema version:

```go
type persisted struct {
	Version int            `json:"version"`
	Seq     int            `json:"seq"`
	Groups  []*model.Group `json:"groups"`
}

const persistVersion = 1
```

Current schema version: **1**. A change that older sbnn cannot read is a bump
of `persistVersion`, and belongs here with what happens to an existing session
file when it meets the newer binary.

### `reviews.jsonl`

The review log, one flat JSON object per line, kept outside the working tree
(`$XDG_STATE_HOME/sbnn/reviews.jsonl` by default). It is meant to be kept for a
long time, shared, and read with `jq`, and `sbnn reviews --help` makes the
compatibility promise in as many words:

> Parse the jsonl form - one flat JSON object per line, fields get added but
> not renamed.

So: **fields may be added, and are not renamed or removed.** Every added field
goes here with the version that started emitting it — that is the question a
year-old log raises and nothing else in the repository answers.

### `sbnn export` payload

The data embedded in an exported HTML page, from `internal/export/export.go`,
versioned by `PayloadVersion` (`"version"` in the payload).

Current payload version: **1**. An exported page is a self-contained artefact
that someone else opens, possibly long after it was made, so a bump belongs
here together with whether older pages keep rendering.
