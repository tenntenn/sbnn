---
name: sbnn
description: Review a diff with sbnn - show it to a human in the browser and read their line comments back, or review a change yourself by leaving comments on lines and submitting. Use after producing or being handed changes that someone should look at, when the user asks to review a diff or open a diff/review UI, or when review comments are waiting in sbnn.
license: MIT
---

# Reviewing a diff with a human using sbnn

`sbnn` serves a unified diff as a review page in the browser. The human reads
the diff, attaches comments to lines, and you read those comments back from
the command line. sbnn never runs git itself: it reads the diff from stdin, so
it works with `git diff`, `jj diff`, `diff -u`, a `.patch` file, or a diff
you produced yourself.

Two words recur below and mean different sizes: a **group** — what
`--target` names — is the whole review, with its own URL, its own comments
and its own history. A **round** is one diff sent into a group. Sending a
second diff into the same group starts its next round; it does not start a
new review.

## When to use this

- You changed code and want a human to look at it before continuing.
- The user asks to "review this diff", "show me the changes", "open the diff
  in a browser".
- The user says they left comments in sbnn (or you sent a diff earlier in this
  session and are picking the work back up).

Do not use it for changes the user asked you to apply without review, and do
not use it as a way to read a diff yourself: you can read the diff text
directly.

## Prerequisites

`sbnn` must be on PATH. Check with `sbnn --version`; if it is missing, tell the
user how to install it instead of guessing at a substitute:

```
go install github.com/tenntenn/sbnn@latest
```

sbnn renders the Markdown preview itself, so nothing else is needed. `mo`
renders a richer one for those who install it, and the reader picks which in
the preview header: `brew install k1LoW/tap/mo` (or a binary from
https://github.com/k1LoW/mo/releases).

## Workflow

### 1. Start from a clean page

A group keeps whatever was left in it: diffs from an earlier task, comments
nobody cleared, hooks from a session that ended. Mixing last week's change
into today's review costs the human the attention you asked for, so close the
old review before opening a new one:

```
sbnn --status --json                  # what is the group holding?
sbnn --clear --target <topic>         # close it: diffs, comments and hooks
```

Two exceptions, both of which mean "do not clear":

- You are sending the **next round** of a review you already started. Then
  the diffs belong together; clear the handled comments instead (step 7).
- The group holds **comments the human wrote that you have not addressed**.
  Say what is in there and ask before throwing it away.

### 2. Send the diff

Pipe the diff into `sbnn` and use `--target` to name the review, so several
reviews can be open at once without mixing their comments:

```
git diff | sbnn --target <topic>
```

Pick the name yourself and keep using it for every command of that review.
sbnn attaches no meaning to it, so make it stand for whatever separates this
review from your others — the task, the branch, the checkout you are working
in. If everything you do in this session belongs to one review, export
`SBNN_TARGET=<topic>` once instead of repeating the flag.

Other ways to produce the same diff text:

```
git diff HEAD~1 | sbnn --target <topic>     # a specific range
git diff --cached | sbnn --target <topic>   # staged changes
diff -u old.txt new.txt | sbnn --target <topic>
cat change.patch | sbnn --target <topic>
```

**Reviewing one branch of a stack of pull requests.** Give each branch its
own target, and diff it against the branch below it rather than against
main, so the group holds only that PR's own change:

```
git diff origin/main...feature-1 | sbnn --target feature-1 --label pr=101
git diff feature-1...feature-2   | sbnn --target feature-2 --label pr=102
```

`--label` carries the PR number or URL into `sbnn reviews` for later, and is
worth passing when you have it. None of this is required — sbnn does not
know what GitHub or a stack is — it is only the shape that keeps one group
lined up with one PR.

sbnn prints the review URL and returns immediately; the server keeps running in
the background. Running `sbnn` again adds another diff to the same page rather
than starting a second server.

**Fold away what nobody reads line by line.** A diff that arrives with a
lock file and a directory of build output in it costs the human attention
before they reach the change you want looked at. You know which files those
are — you produced the diff — so say so, with `--collapse`:

```
git diff | sbnn --target <topic> --collapse 'go.sum' --collapse 'web/dist/**'
```

Patterns work like .gitignore: a name without a slash matches at any depth,
`**` stands for any run of directories, and one `--collapse` can carry a
comma-separated list. In a git repository the list can come from the
repository itself, which beats you guessing at it:

```
git diff | sbnn --collapse "$(git ls-files ':(attr:linguist-generated)' | paste -sd,)"
```

sbnn folds one more kind of file on its own: one that declares itself
generated, in a `DO NOT EDIT` or `@generated` line its generator wrote. That
is the file speaking, not sbnn inferring, and the page says which line it
found. Nothing is folded on size, path or extension — a file folded for a
bad reason is a file nobody reads.

Folding hides nothing. A folded file keeps its place in the list and its
counts, opens with one click, and is never folded while it carries a
comment. So `--collapse` is for noise, never for a file you would rather
not have looked at.

Use `--json` when you want to parse the result:

```
git diff | sbnn --target <topic> --json
```

### 3. Hand the URL to the human, and decide how you come back

Tell the user the URL sbnn printed and say what you want reviewed. Then pick
one of these — never poll `sbnn comments` in a loop:

- **They are reviewing now and you can wait**: `sbnn wait --target <topic>`
  blocks until they press Submit review and then prints the comments. Give it
  a `--timeout` you can afford; status 2 means "not reviewed yet".
- **The review may land later** — they are in a meeting, it is late, your
  session will not live that long: register the follow-up before you go, so
  the sbnn server starts it when the review is submitted, and end your turn.

  ```
  sbnn hook --target <topic> --on-review '<command that resumes the work>'
  ```

  The command gets the review prompt on its stdin and the variables below in
  its environment:

  - `SBNN_GROUP` — the group that was reviewed: the name you passed to
    `--target`, or `default` when you passed none.
  - `SBNN_URL` — the review page of that group.
  - `SBNN_SERVER` — the base URL of the sbnn server that started the hook.
  - `SBNN_PORT` — the port of that same server. These two are how the hook
    talks back to the server that started it: a hook that wants the comments
    runs `sbnn comments --target "$SBNN_GROUP" --port "$SBNN_PORT"` rather
    than assuming the review is on the default port.
  - `SBNN_COMMENTS` — how many comments the review left, as a number. It is a
    count, not the comments themselves; read those with `sbnn comments`.
  - `SBNN_REVIEW_NOTE` — what the reviewer said about the change as a whole,
    which is empty when they said nothing.
  - `SBNN_VERDICT` — the verdict of the review as a whole, spelled the way the
    JSON event spells it: `approved`, `commented` or `changes-requested`. It is
    empty for a review that has none, so pick your own default rather than
    reading one into it.
  - `SBNN_BLOCKING` — `1` or `0`, the answer to "may the change go ahead?".
    This is the same rule as `wait --exit-code` and `submit --exit-code`, so a
    hook that branches on it agrees with a pipeline that branches on sbnn's
    exit status. It is not the verdict: a review that only commented still
    blocks while a comment of it is open, so branch on this rather than on
    `SBNN_VERDICT`.

  Ask the user what that command should be for their setup rather than
  guessing; if they do not want one, tell them to run `sbnn comments` and
  paste the result to you when they are back.
- **Neither**: say you will pick the review up next time, and stop. Nothing
  is lost — the comments stay in the sbnn server until they are cleared.

### 4. Leave your own comments, if you have any

Before handing over, you can mark the places you are unsure about, so the
human reviews them first. Always pass `--author` with your own name so the
human can tell your notes from theirs:

```
sbnn comment <path>:<line> -m "<question or note>" --author <you> --target <topic>
sbnn comment <path>:<line>-<line> -m "..." --suggest "<replacement>" --author <you>
sbnn comment <path>:<line> -m "<question>" --question --author <you>
```

`--question` marks a comment that wants an **answer, not a change**. The two
read alike in prose — "should this be a 404?" can be either — so say which
it is, and the reader is told plainly rather than guessing.

`--suggest` appends the replacement to the comment as a ` ```suggestion `
block, so the human sees it as a proposed change and can copy it:

```suggestion
if err != nil {
    return fmt.Errorf("read config: %w", err)
}
```

Use it for what is genuinely worth a human's attention — a decision you had
to guess at, a trade-off, something you could not verify. A comment on every
change is noise, not a review.

### 5. Read the comments

```
sbnn comments --target <topic>
```

This prints the open comments as Markdown, each with the file, the line
range, the reviewed code and the comment body. For programmatic handling:

```
sbnn comments --target <topic> --format json
```

Every JSON entry has `id`, `group`, `diffId`, `fileId`, `path`, `side` (`new`
or `old`), `startLine`, `endLine`, `body`, `snippet`, `resolved`, `createdAt`
and `updatedAt`. Line numbers refer to the side named by `side`, and `diffId`
says which round the comment came from, which is how you tell an old comment
from one left on the diff you just sent.

Three more keys appear only when they are set, so read them with a default
instead of by subscript — a missing key is the normal case, not an error:

- `author` — who left the comment. It is **missing** for the comments the
  human wrote in the browser, and present for the ones posted from the
  command line, including your own, so skip those when working through the
  list.
- `question` — present only as `true`; missing means the comment asks for a
  change rather than an answer.
- `suggestions` — present only when the body carries a suggestion block;
  missing means there is nothing to apply.

A comment may carry suggested replacements, written as fenced
` ```suggestion ` blocks inside the comment itself, the same convention
GitHub uses. The Markdown output prints the comment as it is and then names
the lines the block replaces; in JSON they are the `suggestions` array. Apply
each block verbatim to exactly those lines unless it is wrong, and say so if
you do not.

### 6. Act on every comment

Work through the comments one by one. Change the code where the comment asks
for a change, and replace the named lines exactly as written where a comment
carries a suggestion; when you disagree or a comment cannot be acted on, say
so explicitly in your reply to the user rather than silently skipping it.

A comment marked as a **question** (`"question": true` — the key is absent on
every other comment, and "This one is a question: answer it." in the Markdown
output) is asking for an answer.
Answer it in words in your reply, and change the code only if your own
answer says it should change. Rewriting code in place of answering is the
one response that leaves the reviewer having to ask again.

### 7. Send the next round

Clear the handled comments and send the updated diff so the next round starts
clean. This is the one case where the diffs stay: the rounds of one review
belong together.

```
sbnn comments --target <topic> --clear
git diff | sbnn --target <topic>
```

When the work is done and the review has served its purpose, close it, so the
next one starts on an empty page (the human can also press Close in the
browser):

```
sbnn --clear --target <topic>
```

### Reviewing instead of being reviewed

The same commands run the other way round, which is how you review a change
someone hands you. Leave the findings as comments and end the round
yourself, since there is no browser and no button:

```
sbnn comment <path>:<line> -m "<what is wrong and why>" --author <you> --target <topic>
sbnn submit --target <topic> --approve -m "<what the review says as a whole>"
```

`sbnn submit` is the Submit button: it wakes whoever ran `sbnn wait`, starts the
hooks, and writes the round into the log of past reviews. Submit even when
you found nothing — "nothing to address" is the answer the other side is
waiting for, and a round that is never submitted is one nobody is told
about.

Say what you decided about the change as a whole, the way a review on a
pull request does. It is a separate question from what any one comment
says, and the reader acts on it:

```
sbnn submit --approve                     # it can go ahead
sbnn submit                               # commented: said things, did not decide
sbnn submit --request-changes -m "..."    # not as it is
```

Approve when the change is right and your remarks are worth reading rather
than blocking — that is what an approval with comments means, and refusing
to give one because you found something to say is how a review stops being
useful. Ask for changes when it should not go ahead as it is, even if you
pointed at no single line. Leave it at "commented" when the decision is not
yours to make. `--exit-code` turns your answer into a status: 1 when the
review blocks the change, 0 when it does not.

Two things make such a review checkable afterwards, so do both: anchor every
comment to the exact `path:line` it is about, and always pass `--author` with
your own name. The line and the stored snippet let a reader see for
themselves whether the claim holds; the author is what separates your
findings from the human's when the log is read later.

```
sbnn reviews --comments | awk -F'\t' '{print $4}' | sort | uniq -c
```

Say what you could not check, rather than leaving it out. A review that
reports only what it verified is worth more than one that sounds complete.

### Sharing a review without sbnn

When the human cannot run sbnn — a review that travels by mail, a page for
someone else, an artifact — write the review out as a single HTML file:

```
git diff | sbnn export --target <topic> review.html
```

The page carries the diff and the same UI, needs no server, and the comments
written on it stay in that browser. Use `--fragment` when the page is
embedded into something that brings its own `<html>` (for example an
artifact).

## Fitting sbnn into what you were already doing

sbnn is one command among the ones you already run, so let the shell do the
joining rather than looking for a flag:

```
git diff | sbnn                          # anything that writes a diff feeds it
sbnn comments | pbcopy                   # anything that reads text takes it
sbnn reviews --format jsonl | jq ...     # a line per review, for whatever asks
```

`sbnn comments` and `sbnn wait` say what they found in their exit status too — 0
when there is nothing to address, 1 when there is, and 2 from `sbnn wait` when
the review has not happened yet — so a review can gate what comes next
without anyone reading the output:

```
git diff | sbnn --target <topic>
sbnn wait --target <topic> -q && git commit -m "<message>"
```

That is the recommended way round committing, and it is worth being plain
with the user about why: send what you are about to commit, wait for the
review, and commit only once it comes back with nothing to address. sbnn has
no idea what a commit is and stays out of the way of one — it writes nothing
into the working tree, so `git status` says exactly what it said before you
started. When a review does have comments, address them and send the next
round before committing rather than committing over them.

If the change is already committed, review it the same way: `git show | sbnn`,
or `git diff <base>..HEAD | sbnn`. Use `--title` to say which is which, since
sbnn only sees the text.

## Learning from past reviews

Every submitted review is kept, which makes the reviewer's habits readable
rather than guessed at:

```
sbnn reviews --stats                     # which files draw comments, how many per review
sbnn reviews --comments --since 30d      # one line per comment, to read properly
```

For any question `--stats` does not answer, `--comments` emits one record
per comment and ordinary tools do the counting. Parse the jsonl form — one
flat JSON object per line, and the only form that carries whole comment
bodies; the tab-separated text form (date, group, path:lines, author,
first body line) is for reading and quick pipes:

```
sbnn reviews --comments --format jsonl | jq -r 'select(.suggestions) | .path'
sbnn reviews --comments | cut -f3 | cut -d: -f1 | sort | uniq -c | sort -rn
```

Worth doing before you hand over a change of the same shape: if the last ten
reviews of this repository were mostly about error messages and test names,
check yours before asking. Say what you found and what you changed because of
it — a pattern you read out of the log is a claim about the human, so let
them correct it.

## Command reference: the commands and flags this workflow uses

This table is scoped on purpose: it carries every flag the steps above tell
you to pass, and nothing else. When you want a flag the workflow never asks
for, `sbnn <command> --help` is the complete list.

| Command | What it does |
| --- | --- |
| `sbnn --version` | Check sbnn is there before you build a plan on it |
| `<diff producer> \| sbnn` | Add a diff to the default group and print its URL |
| `... \| sbnn -t <name>` | Add it to a named group (its own URL and comments) |
| `... \| sbnn --title "..."` | Name the diff, so a stack of them can be told apart |
| `... \| sbnn --collapse '<glob>'` | Fold generated files away, repeatable |
| `... \| sbnn --label <key>=<value>` | Keep a PR number or URL with the diff, repeatable |
| `sbnn --status [--json]` | Show the running server, its groups and comment counts |
| `sbnn --clear [-t <name>]` | Close a review: its diffs, comments and hooks |
| `sbnn wait [-t <name>]` | Block until the review is submitted, then print it |
| `sbnn wait --timeout <duration>` | Give up after that long; status 2 means "not reviewed yet" |
| `sbnn wait -q` | Print nothing and answer in the exit status, for `&& git commit` |
| `sbnn hook --on-review '<cmd>'` | Have the server run something when the review lands |
| `sbnn hook [--clear]` | List or drop those hooks |
| `sbnn comment <path>:<line> -m "..."` | Leave a comment of your own |
| `sbnn comment ... --author <you>` | Say who is commenting — always pass it |
| `sbnn comment ... --question` | Mark it as wanting an answer, not a change |
| `sbnn comment ... --suggest "<text>"` | Propose a replacement for the commented lines |
| `sbnn comment --json` | Post many comments at once, read from stdin |
| `sbnn comments [-t <name>]` | Print open comments as Markdown |
| `sbnn comments --format json` | Print them as JSON |
| `sbnn comments --clear` | Remove the comments of the group, before the next round |
| `sbnn submit [-t <name>] [-m "..."]` | End the round yourself, as the Submit button does |
| `sbnn submit --approve` | Submit saying the change can go ahead |
| `sbnn submit --request-changes` | Submit saying it should not, as it is |
| `sbnn submit --exit-code` | Turn that verdict into a status: 1 blocks, 0 does not |
| `sbnn reviews [--since 7d]` | The reviews that were submitted |
| `sbnn reviews --stats` | What they say together: which files draw comments, how many per review |
| `sbnn reviews --comments [--format jsonl]` | One record per comment, for sort/uniq/awk/jq |
| `... \| sbnn export <file>` | Write the review as one self-contained HTML page |
| `... \| sbnn export --fragment <file>` | The same, body only, for embedding |

`--port` (default 6280) selects the server; use it only if the user runs sbnn
on a non-default port. Inside a review hook you do not have to ask which port
that is: the server passes its own in `SBNN_PORT`.

## Notes

- New files are shown as a unified diff, because there is no old side to put
  next to them.
- Markdown files get a preview pane next to the diff. The preview shows the
  working tree file when it exists; otherwise sbnn rebuilds what it can from
  the diff, and unified diffs only carry the changed hunks, so such a preview
  is partial by nature.
- A comment can also be made by selecting text in that preview, in which case
  its line range covers the whole Markdown blocks the selection touched -
  paragraphs, list, code fence - rather than only the words highlighted. The
  quoted snippet is what was selected.
- Comments are stored by the sbnn server, not in the browser, which is why they
  survive a reload and why you can read them from the command line.
- `sbnn --status --json` is the reliable way to check whether comments are
  waiting: it reports `comments`, `unresolved` and `reviewed` per group.
  `reviewed` is true once the human has submitted, and false again as soon as
  a newer diff arrives.
