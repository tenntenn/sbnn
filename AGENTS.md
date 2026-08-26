# Agent guide for this repository

## Reviewing changes with sbnn

This repository ships an agent skill for sbnn itself. Read
[`skills/sbnn/SKILL.md`](skills/sbnn/SKILL.md) before showing a diff to a human:
it describes the whole loop (send the diff, hand over the URL, read the
comments back, address them, start the next round).

To use the same skill in another repository, install it there with
`sbnn skill --install <dir>` (see the README for the directory each agent reads,
or link the installed file from that repository's AGENTS.md the way this
section does).

Short version:

```console
$ git diff | sbnn --target <topic>   # opens the review page, prints its URL
$ sbnn comments --target <topic>     # the comments the human left
$ sbnn comments --target <topic> --clear
$ git diff | sbnn export --target <topic> review.html   # a page that needs no server
```

## Working on sbnn

- Build: `task build` (runs `pnpm build` in `web/`, then `go build`).
- Test: `task test`.
- Tools are managed with [aqua](https://aquaproj.github.io/); run `aqua install` to get `task`.
- The built UI in `web/dist` is committed on purpose, so `go install` works
  without Node. Rebuild it whenever `web/src` changes.
- sbnn must not shell out to git: diffs only ever come from stdin. `task lint`
  enforces this with the `nogit` analyzer in `internal/analysis/nogit`, so an
  `exec.Command("git", ...)` fails the build rather than the review.
- Lint: `task lint`. It runs the repository's own vet tool
  (`internal/analysis/sbnnvet`) alongside `go vet`; add a machine-checkable
  rule as an analyzer there rather than as a paragraph here. See
  [`CONTRIBUTING.md`](CONTRIBUTING.md).
