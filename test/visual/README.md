# Visual tests

Browser rendering tests for the review UI. They start a real sbnn server,
load a fixture diff into it, drive Chromium, and assert on geometry and
computed style.

Nothing here is a golden image. Every assertion is a number that is stable
across machines - a width, a background colour, the left edge of a glyph -
so there is no screenshot to re-bless and no font rendering to be flaky
about, and a failure names the number that is wrong.

## Running them

```console
$ cd test/visual
$ pnpm install
$ pnpm test
```

Chromium is required. Playwright downloads it on demand:

```console
$ pnpm exec playwright install chromium
```

To check that the configuration and the specs parse without a browser:

```console
$ pnpm exec playwright test --list
```

## Why this is its own project

The dependencies live here rather than in `web/package.json` for two
reasons. They are development tooling, and `web/` builds the bundle that
ships inside the binary - a test runner has no business in it. And the
`web/` lockfile is installed with `--frozen-lockfile`, so adding to it
would force every other checkout to reinstall.

Nothing in this directory is embedded into the binary or served to anyone.

## How the harness starts the server

`global-setup.ts` builds the binary and starts it; `global-teardown.ts`
shuts it down. The choices, made by reading `cmd/root.go` and
`cmd/server.go`:

- **The port is chosen by the test, not by sbnn.** The test takes a free
  port from the operating system and passes it as `--port`. `cmd/root.go`
  accepts the flag and `runServer` binds it, so the URL is known before the
  process starts and two runs never collide. Parsing the URL out of stdout
  would work too, but only after the process is already up.
- **Readiness is `GET /_/api/status`**, polled - the same probe
  `waitForReady` in `cmd/server.go` uses.
- **The fixture arrives on stdin**, which is the `git diff | sbnn` path,
  rather than through the HTTP API. That way the parser under the page is
  the one a real diff goes through.
- **The binary is built first** rather than run with `go run`. `go run`
  stays alive as the parent of the real server, so killing it can leave the
  server holding the port.
- **The server stays on loopback.** `--dangerously-allow-remote-access` is
  never passed.
- Session, cache and review log are redirected into `.tmp/` with
  `XDG_STATE_HOME` and `XDG_CACHE_HOME`, so a test run never touches the
  state of the sbnn you use yourself.

The page holds an `EventSource` open on `/_/events`, so Playwright's
`networkidle` never fires. The specs wait for the file list instead.

## The fixture

`fixtures/visual.diff` carries the cases that keep breaking: a dotfile
path, a path with parentheses, a very long path, a rename, an added file, a
deleted file, a binary file, and a Markdown file with a relative image.

## Tests that are expected to fail

Some assertions describe defects that are open right now. Those carry
`test.fail()`, so Playwright expects them to fail: the suite is green while
the defect exists and goes **red when the defect is fixed**, which is the
signal to delete the annotation.

| test | issue | where | measured when written |
|---|---|---|---|
| paths are painted in the order they are written | #73 | all four | `.file-path` is `direction: rtl`, so `.github/workflows/ci.yml` paints as `github/workflows/ci.yml.` |
| no element is wider than the box that holds it | #119 | all four | `.disclosure` is `clientWidth` 10 around a 14px icon |
| the page does not scroll sideways | #74 | desktop only | the preview pane is `clientWidth` 517, `scrollWidth` 576; the narrow layout shows one pane at a time and holds |
| hover and selected are different colours | #79 | desktop only | both settle on `rgb(238, 241, 244)`: the bundle paints `.file-item:hover` and `.file-item.active` with the same `--bg-inset` |

**#74** holds on the phone layout, so only the desktop projects carry that
annotation. **#79** is skipped there instead: hover is a pointer affordance
and the narrow layout has no pointer behind it.

## Reading a colour: wait for the transition

`getComputedStyle` during a transition returns the value part way along it.
Two rows moving towards the *same* colour from different starting points
read as two different colours for as long as the transition runs, so a
comparison made too early passes no matter what the stylesheet says - which
is how the #79 assertion above spent its first version green over a bundle
that painted both states identically:

```
IMMEDIATE  selected rgba(238, 241, 244, 0.85)   hover rgba(238, 241, 244, 0.255)
SETTLED    selected rgb(238, 241, 244)          hover rgb(238, 241, 244)
```

`settled(page, selector)` in `geometry.spec.ts` waits for every animation on
the matched elements to finish. Anything that compares computed colour has
to go through it. Emulating `prefers-reduced-motion` is not a substitute:
the guard that honours the query lives in the stylesheet under test, so a
bundle without it would go unwaited.

## What is measured is `web/dist`, not `web/src`

The binary serves the committed bundle, so that is what the browser sees. A
fix that is in `web/src` but not yet rebuilt into `web/dist` does not move
these tests, and an annotation flips to "Expected to fail, but passed" when
the rebuilt bundle lands rather than when the source change does.

## Viewports and colour schemes

Four projects: 1440x900 and 390x844, each in `light` and `dark`. A failure
names the combination, so `[phone-dark] > the page does not scroll
sideways` says where to look.
