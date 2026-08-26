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
$ task test      # go test ./... and the review UI tests in web/test
$ task lint      # go vet, a gofmt check, go fix -diff, and go mod tidy with no diff
$ task dev       # sbnn in the foreground plus the Vite dev server
```

**Run `task lint` and `task test` before you push, and treat that as your job
rather than a checker's.** `task lint` is `go vet ./...`, a `gofmt -l` check
that fails on any unformatted file, `go fix -diff ./...`, then `go mod tidy`
followed by `git diff --exit-code go.mod go.sum` — so it also catches a
dependency added without tidying. `go fix -diff` prints the rewrite the
standard library has since given a better spelling for rather than applying
it, and exits non-zero when there is one; run `go fix ./...` to take the
patch. Unformatted code that reaches review costs someone else a round trip.

`task test` runs two suites: `go test ./...`, and the review UI tests under
`web/test` — the word-level diff, the suggestion parser, the search matcher,
the shortcut table, the exported page's static client, none of which any Go
test reaches. CI runs them as two jobs, `go` and `web`, so the two halves are
`task test-go` and `task test-web` if you want one of them on its own. The web
half needs pnpm; if you have not got it, run `task test-go`.

There is a third suite that `task test` does **not** run: `test/visual`, a
Playwright harness that drives a real Chromium against the committed
`web/dist`. It is separate on purpose — it measures the bundle rather than
your source, and it pins defects that are still open with `test.fail()`, so it
goes red when one of them is fixed. See [test/visual/README.md](test/visual/README.md).

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
