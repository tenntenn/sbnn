# Contributing to sbnn

Thanks for taking an interest. This file covers what you need installed, the
commands to run, and the two rules of this repository that you cannot guess
from reading the code.

## What you need

- **The Go toolchain `go.mod` asks for**, which is `go 1.27.0`. Your installed
  Go does not have to be that new: since Go 1.21 the default `GOTOOLCHAIN=auto`
  downloads the toolchain a module requires. It stops only under
  `GOTOOLCHAIN=local`, or on Go 1.20 and earlier, and the error names both
  versions — `go: go.mod requires go >= 1.27.0 (running go 1.24.7;
  GOTOOLCHAIN=local)` — so upgrade to the version it prints.
- **[aqua](https://aquaproj.github.io/).** Run `aqua install` in the repository
  root to get the pinned tools — `task` and `tagpr`, at the versions in
  `aqua.yaml`. Everything below is a `task` command.
- **[pnpm](https://pnpm.io/) and Node.** The review UI is React + Vite under
  `web/`, and its build runs through pnpm. You only need it if you touch
  `web/src` or run a full `task build`.

## The commands

```console
$ task build     # pnpm build in web/, then go build ./...
$ task test      # go test ./...
$ task lint      # go vet, sbnnvet, a gofmt check, go fix -diff, go mod tidy with no diff
$ task dev       # sbnn in the foreground plus the Vite dev server
```

**Run `task lint` and `task test` before you push, and treat that as your job
rather than a checker's.** `task lint` is `go vet ./...`, then `sbnnvet` (below),
a `gofmt -l` check that fails on any unformatted file, `go fix -diff ./...`,
then `go mod tidy` followed by `git diff --exit-code go.mod go.sum` — so it also
catches a dependency added without tidying. `go fix -diff` prints the rewrite the
standard library has since given a better spelling for rather than applying
it, and exits non-zero when there is one; run `go fix ./...` to take the
patch. Unformatted code that reaches review costs someone else a round trip.

## The repository's own vet tool

`go vet`'s default set is deliberately small, so the rules this repository
cares about live in a vet tool of its own, `internal/analysis/sbnnvet`. It is a
`tool` directive in `go.mod`, which means there is nothing to install: `go`
builds it from this module, with the toolchain `go.mod` asks for. `task lint`
runs it as

```console
$ go vet -vettool=$(go tool -n sbnnvet) ./...
```

and you can point it at one package, or run a single analyzer, while working:

```console
$ go vet -vettool=$(go tool -n sbnnvet) -nogit ./internal/...
```

It carries three analyzers.

- **`nogit`** — this is sbnn's own rule, the one stated in `AGENTS.md`: sbnn
  never runs git, because a diff only ever arrives on stdin. That is what lets
  it review a diff no working tree can produce any more. The analyzer reports
  `exec.Command`, `exec.CommandContext` or `exec.LookPath` given a constant
  that names the git binary — `"git"`, `"/usr/bin/git"`, `"git.exe"` — so the
  one line that would quietly undo the rule fails the build instead of the
  review. Its source is `internal/analysis/nogit`, with the cases it does and
  does not report spelled out in `testdata/src/a/a.go`.
- **`nilness`** and **`unusedwrite`** — two analyzers `golang.org/x/tools`
  ships that `go vet` does not run by default. `nilness` reports a dereference
  or a comparison that is nil on every path; `unusedwrite` reports a write to
  a struct field or an array element that nothing reads back, which is what a
  fix applied to a copy instead of the original looks like.

`shadow` is deliberately **not** in the set. It reports 36 places in this tree,
and every one of them is the ordinary `if err := f(); err != nil` written
inside a function that already has an `err` — the idiom, not a bug. A check
that is wrong 36 times out of 36 teaches people to ignore it.

To add a rule, put the analyzer in `internal/analysis/<name>/` with an
`analysistest` case under `testdata/src/a/`, and register it in
`internal/analysis/sbnnvet/main.go`.

## If you touch `web/src`, rebuild `web/dist`

This is the most surprising rule here, so it gets its own section.

**`web/dist` is a build output, and it is committed on purpose.** That is what
lets `go install github.com/tenntenn/sbnn@latest` work for someone who has no
Node at all: the assets are already in the tree, and `go:embed` needs them
present to build the binary in the first place.

The corollary is that the tree does not rebuild itself:

```console
$ task web       # pnpm install, then pnpm run build, in web/
```

Run that whenever you change anything under `web/src`, and commit the resulting
`web/dist` along with your source change.

Nothing catches this for you. A UI change submitted without the rebuild reviews
perfectly — the diff of `web/src` is right there and reads correctly — and then
does nothing at all when someone installs it, because the binary embeds the old
assets. Automated checks build `web/` to see that it compiles, but they do not
compare `web/dist` against `web/src`, since `dist` is regenerated in batches
rather than on every change. So this one is on you.

## Running what you changed

```console
$ task dev
```

That runs two things side by side: `go run . --foreground`, which is the sbnn
server, and Vite's dev server for the UI. Vite proxies everything under `/_` to
the sbnn server on `localhost:6280`, so the page you open from Vite talks to
your local build with hot reloading in front of it.

To exercise it the way a user would, pipe a diff in from another terminal:

```console
$ git diff | sbnn --target scratch
```

## What a good pull request looks like

**One change per pull request**, with the reasoning in the description: what
was wrong, and why this is the fix. The repository's own history is a fair
sample of the tone.

**Commit subjects are sentence case, written as what the change does, with no
`feat:`/`fix:` prefix and no trailing full stop.** From `git log`:

```
Tidy go.mod so cobra and pkg/browser are not marked indirect
Preview images and Jupyter notebooks, not only Markdown
Fix the settings menu rendering under the preview toolbar
Stop calling a round a group, and document reviewing a PR stack
```

They say what the change does, not which files it touched. Where it takes a
clause of explanation to make the subject honest, the subjects here spend it.

**Add a test where one can exist.** A bug fix is worth a test that fails before
it and passes after. The tests under `internal/` are table-driven; follow the
ones next to the code you are changing.

**Say how you checked it.** The commands you ran and what they printed.

## How a release happens

Nothing is released by hand.

1. Merging to `main` runs [tagpr](https://github.com/Songmu/tagpr), which keeps
   a release pull request open with the merged pull requests collected into
   `CHANGELOG.md`.
2. Merging *that* pull request pushes the tag and opens the GitHub release.
3. The tag push runs `.github/workflows/release.yml`, which runs GoReleaser
   over `.goreleaser.yml`: six archives — macOS, Linux and Windows on x86_64
   and arm64 — plus `checksums.txt`, appended to the release tagpr made.

The version a binary reports comes from the tag, not from a file anyone edits.
GoReleaser passes it at link time; a `go install github.com/tenntenn/sbnn@v1.2.3`
build has no linker flags and reads the module version out of its own build
info instead. Both paths are in `version/`, and `version/release_test.go`
checks that `.goreleaser.yml` still names the variables it stamps.

To try the build without releasing anything:

```console
$ go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
```

It writes the archives into `dist/`, which is git-ignored. `web/dist` is a
different directory and is committed on purpose.

## Reporting things

- **Bugs and features:** open an issue, and include the diff sbnn was given.
  sbnn never runs `git` itself, so that text is its entire input and most bugs
  cannot be reproduced without it. Redact it as much as you need to — as long
  as the structure that triggers the bug survives.
- **Security:** please do not open a public issue. Report it privately at
  <https://github.com/tenntenn/sbnn/security/advisories/new>.
