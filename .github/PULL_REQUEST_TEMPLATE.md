## What changed

<!-- What this does, and why. If it fixes an issue, add "Fixes #NNN". -->

## How you tested it

<!-- The commands you ran, and what they printed. -->

## Checklist

- [ ] I ran `task lint` and it passed.
      (`go vet ./...`, a `gofmt -l` check, and `go mod tidy` followed by
      `git diff --exit-code go.mod go.sum`. Run it before pushing rather than
      waiting to see what a checker says.)
- [ ] If I changed anything under `web/src`, I ran `task web` and committed
      the resulting `web/dist`.
      (`web/dist` is committed on purpose so `go install` works without Node,
      and `go:embed` needs it to build. Skip this if you did not touch
      `web/src`.)
