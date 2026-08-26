---
name: sbnn
description: Review a diff with sbnn - show it to a human in the browser and read their line comments back, or review a change yourself by leaving comments on lines and submitting. Use after producing or being handed changes that someone should look at, when the user asks to review a diff or open a diff/review UI, when review comments are waiting in sbnn, or when the reviewer cannot open a localhost URL and needs the review as a self-contained page or artifact (on a phone, in a chat, over mail).
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

### 3. Hand the URL to the human, or a page they can actually open

Decide which of the two it is before handing anything over. The sbnn server
listens on localhost, so its URL only works on the machine sbnn is running
on.

**The URL is the default.** Hand it over whenever you and the human are on
the same machine and they can open a browser there.

**When you cannot be sure of that, do not hand over the URL.** Inside a phone
app, a chat bot or a CI box, `http://localhost:6280/` reaches nobody, and the
human is left opening a link that does not exist for them. Write the review
out as a self-contained page and give them that instead:

```
git diff | sbnn export --target <topic> review.html   # a file they can open
git diff | sbnn export --fragment --target <topic>    # the body, on stdout
```

`--fragment` decides what you get; the filename only decides where it goes.
With `--fragment` you get the page body alone — written to the file you name,
or to stdout if you name none. Without it you get a whole self-contained page,
again to a file or to stdout. Reach for `--fragment` only when the review is
going inside something that brings its own `<html>` — an artifact, a message
that renders HTML, a mail. Hand a human a whole page.

The user can also just ask. If they want a file, an artifact, or something
they can read on their phone, export one whether or not the URL would have
worked.

**An exported page does not come back to you.** Comments written on it stay
in that browser and never reach a server, so steps 5–7 do not apply:
`sbnn comments` will return nothing however long you wait. The human presses
**Copy prompt** on the page and paste the text back to you. Say that when you
hand the page over, not after they have finished writing.

When the URL is the right answer, tell the user the URL sbnn printed and say
what you want reviewed. Then pick one of these — never poll `sbnn comments`
in a loop:

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

Make sure every comment has actually been addressed first: `--clear` empties
the whole group at once, comments you never got to along with the ones you
handled — there is no selective clear. Once nothing is left unaddressed,
clear and send the updated diff so the next round starts clean. This is the
one case where the diffs stay: the rounds of one review belong together.

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

The page carries the diff and the same UI and needs no server. Use
`--fragment` when the page is embedded into something that brings its own
`<html>` (for example an artifact); with no filename it writes to stdout, so
the markup can go straight into whatever you are building:

```
git diff | sbnn export --fragment --target <topic>
```

This is also the fallback step 3 sends you here for, and the same limit
applies: the comments written on such a page stay in that browser and never
reach a server, so `sbnn comments` has nothing to return. The human presses
**Copy prompt** on the page and pastes the text back to you, and that is the
only way the review reaches you.

## Reference material

The workflow above is the whole of what you need to run a review. The rest is
material to look something up in, kept in its own files so that reading the
skill does not mean reading all of it:

- [`references/commands.md`](references/commands.md) — every command and flag
  the workflow uses, when you need the exact syntax of a step you have decided
  to take.
- [`references/pipelines.md`](references/pipelines.md) — how sbnn joins onto
  the commands you already run, when you are scripting it rather than typing it.
- [`references/review-history.md`](references/review-history.md) — what
  `sbnn reviews` records, when you want to know what past reviews asked for.
- [`references/notes.md`](references/notes.md) — behaviour worth knowing but
  rarely decisive, when something surprises you.
