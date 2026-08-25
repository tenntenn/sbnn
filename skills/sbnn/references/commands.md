# Command reference: the commands and flags this workflow uses

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
