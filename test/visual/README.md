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

**There is no table below, because nothing is pinned today.** `geometry.spec.ts`
carries no such call, so every assertion in the suite is a guard, and any failure
is a regression rather than a defect coming good. `docs/doccheck` holds this
section and the specs in step in both directions, so an annotation added
without a row here is as loud as a row left behind after one is deleted.

The last pin out was the hover-versus-selected assertion, and it is worth
saying why, because it is not what its issue number says. That defect was a
colour one - both states painted with `--bg-inset` - and the colour half closed
first, the bundle settling on `rgb(255, 248, 197)` for `.file-item.active`
against `rgb(238, 241, 244)` for `.file-item:hover` on `desktop-light`. What
kept the assertion failing after that was a second defect underneath it: the
test clicks `.file-item` 0 and hovers 1, and the shipped bundle left the active
row on 1, so both selectors matched one element and its background equalled
itself. That closed when the rebuilt bundle landed, and the assertion moved
into "Tests that guard" below with the numbers it now holds at.

It is skipped on the phone projects rather than run there: hover is a pointer
affordance and the narrow layout has no pointer behind it.

## Tests that guard

The rest assert what holds today, and go red when it stops holding. These
are the ones that make the harness worth running: a pinned defect only
reports the day someone fixes it.

| test | where | measured |
|---|---|---|
| paths are painted in the order they are written (#73) | all four | `.file-path` was `direction: rtl`, so the bidi algorithm moved the leading dot of a dotfile to the end and `.github/workflows/ci.yml` painted as `github/workflows/ci.yml.` |
| no element is wider than the box that holds it (#119) | all four | `.disclosure` was a 9.6px box around a 14px icon: `clientWidth` 10, `scrollWidth` 14 |
| the page does not scroll sideways (#74) | all four | the preview pane laid out wider than its column on the desktop layout, `clientWidth` 517 against `scrollWidth` 576; the narrow layout shows one pane at a time and never had it |
| the selected file is painted differently from an unselected one | all four | selected `rgb(255, 248, 197)`, unselected `rgba(0, 0, 0, 0)` on `desktop-light` |
| the exported page contacts no network host (#55) | all four | 3 requests on the wide layout, 2 on the narrow, all of them `file:`; 617 DOM nodes / 55 diff rows wide, 90 / 8 narrow |
| hover and selected are different colours (#79) | desktop only | `.file-item.active` `rgb(255, 248, 197)` against `.file-item:hover` `rgb(238, 241, 244)` on `desktop-light`; the click underneath it lands where it is aimed now - clicking `.file-item` *i* leaves the active row on *i* for every *i*, measured 0..7 over the fixture's eight files |

The first three, and the last, were pinned defects until the bundle that fixes
them landed. Their annotations are gone and the numbers they were measured at
are kept here, because that is what a future failure has to be read against.

One number in the last row is viewport-dependent and the table above is not the
place for it: which file is active *before* anything is clicked depends on how
much of the diff fits on screen. It is file 0 at the 1440x900 this suite runs
at and file 1 at 1600x950, which is the size `docs/screenshot.png` is taken at.
Every click after that selects the file clicked at both sizes, which is the
part the assertion depends on.

## Proving a test can fail

An assertion nobody has watched fail is an assertion nobody knows the
meaning of. Each one above was checked by putting the defect into the
bundle by hand and running the suite again. The bundle is minified CSS, so
the edit is a `sed` on `web/dist/assets/index-*.css` and `git checkout --
web/dist` afterwards.

| edit | expected | got |
|---|---|---|
| `.file-item.active{background:none` | the selected-row guard fails | `Error: the selected row is painted like every other row / Expected: not "rgba(0, 0, 0, 0)"` in all four projects |
| append `@font-face{...url(https://fonts.gstatic.com/...)}` and point `.file-path` at it | the export guard fails | `requests the exported page made off the local file / + "https://fonts.gstatic.com/s/roboto/v30/injected.woff2"` on the two desktop projects; the narrow layout does not paint `.file-path`, so it does not fetch the font and stays green |
| `.file-item.active{background:#fff8c5` (i.e. fixing #79 in the bundle of the day) | the #79 pin fails | `Expected to fail, but passed` |
| the same, with `test.fail` removed | #79 passes | `1 passed` |
| put the defect back, `test.fail` still removed | #79 fails | `Error: hover background equals selected background / Expected: not "rgb(238, 241, 244)"` |
| the same, with the `settled()` call commented out | **#79 passes** | `1 passed` - the false green the next section is about |

Those four rows are a record of the day the pin was written, when the bundle
painted both states `rgb(238, 241, 244)`. They no longer describe the bundle,
and the pin they describe is gone: `.file-item.active` is `#fff8c5`, the click
underneath it lands where it is aimed, and the assertion is an ordinary guard.
The point they make is about the method, not the colour - each row is an
assertion watched failing on purpose, which is the only way to know it says
anything.

The last step of that record is the rebuild itself. Against the committed
bundle of the day the assertion failed for the click reason, so the annotation
was load-bearing; against the bundle rebuilt from the source that fixes it,
Playwright reported `Expected to fail, but passed` on `desktop-light` and
`desktop-dark` - two failures, on the two projects that run it - and deleting
the annotation turned those into `22 passed, 2 skipped`.

## Reading a colour: wait for the transition

`getComputedStyle` during a transition returns the value part way along it.
Two rows moving towards the *same* colour from different starting points
read as two different colours for as long as the transition runs, so a
comparison made too early passes no matter what the stylesheet says - which
is how the #79 assertion above spent its first version green over a bundle
that painted both states identically:

```
IMMEDIATE (4 animations running)  selected rgba(238, 241, 244, 0.345)  hover rgba(238, 241, 244, 0.36)
SETTLED   (0 animations running)  selected rgb(238, 241, 244)          hover rgb(238, 241, 244)
```

Read immediately the two differ - by how far apart the two transitions are,
nothing more - and `expect(hover).not.toBe(selected)` passes over a bundle
that paints both states the same colour.

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

The committed bundle is no longer behind the source. Verified by rebuilding it:

```console
$ cd web && pnpm install --frozen-lockfile --offline && pnpm run build
$ git diff --stat HEAD -- web/dist      # no output
```

so `web/dist` is exactly what `web/src` builds today, and a source fix and a
shipped fix are one rebuild apart rather than an unknown distance apart. The
two things that made this paragraph read the other way are both closed: #326
guarded the loop over `file.hunks`, which used to throw `i.hunks is not
iterable` on the fixture's binary `assets/logo.png` and leave a page of 9 DOM
nodes with no `.diff-table` at all, and #325 committed the rebuilt bundle.

## Viewports and colour schemes

Four projects: 1440x900 and 390x844, each in `light` and `dark`. A failure
names the combination, so `[phone-dark] > the page does not scroll
sideways` says where to look.
