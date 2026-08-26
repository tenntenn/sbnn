# `docs/screenshot.png`

The picture at the top of the README is the first thing anyone sees of sbnn,
and for a while it showed a UI the released binary did not have. It had been
taken against a bundle built locally from `web/src`, while the binary serves
the committed `web/dist`; the two had drifted, and the README advertised a
search box, a file status and a preview pane that nobody who downloaded sbnn
would find. Measured at the time: 468,411 differing pixels on 894 rows between
the committed picture and a shot from a binary built at the same commit.

## The rule

> `docs/screenshot.png` is taken from a binary built at the commit under
> review, serving that commit's committed `web/dist`. Never from `pnpm dev`,
> never from a bundle built locally and not committed.

If `web/dist` moves, the picture is retaken **after** the rebuilt bundle is
committed, in the same change.

## Conditions

Fixed by #313, and not to be changed casually - a shot taken under different
conditions cannot be compared against the one it replaces.

| | |
|---|---|
| viewport | 1600 x 950 |
| `deviceScaleFactor` | 1 |
| colour scheme | `light` |
| capture | the page only: no browser chrome, no `fullPage` |
| settle | 2.5s after `domcontentloaded`, so every transition has finished |
| fixture | three files - `internal/server/server.go` (modified), `internal/server/server_test.go` (added), `README.md` (modified) - and two comments: a suggestion from `reviewer` on `server.go:11`, and a question from `claude` on `README.md:16` |

## Taking it

```console
$ go build -o /tmp/sbnn .                     # the committed web/dist, nothing else
$ cd <fixture-dir>
$ XDG_STATE_HOME=/tmp/shot/state XDG_CACHE_HOME=/tmp/shot/cache \
    /tmp/sbnn --foreground --port <free-port> --history-file off &
$ /tmp/sbnn --port <free-port> --no-open < fixture.diff
$ /tmp/sbnn comment internal/server/server.go:11 --port <free-port> --author reviewer \
    -m '...' --suggest '...'
$ /tmp/sbnn comment README.md:16 --port <free-port> --author claude --question -m '...'
```

then a headless Chromium at the conditions above, `page.screenshot()` into
`docs/screenshot.png`.

**Compare before replacing.** The noise floor is not zero: repeated shots from
one binary come back either byte-identical or differing by around 1,200 pixels
of glyph antialiasing, in rows 8-10 and 620-632. A real UI change is two orders
of magnitude larger. If the difference is zero, or is only those bands, the
committed picture is still correct and should be left alone rather than
replaced with an equivalent one - a PNG is a binary file and rewriting it for
nothing costs every future reader of the history.

## What the picture shows

These strings are painted by the bundle, not by the fixture, and they are on
screen in the committed picture. `TestScreenshotShowsWhatTheBundleCanPaint` in
`docs/doccheck` asserts that the committed `web/dist` can still paint every one
of them, which is what ties the picture to the bundle.

| string | where it is on the page |
|---|---|
| `Search paths and lines ( / )` | the search box, above the file list |
| `Copy prompt` | the header |
| `Submit review` | the header |
| `Every file` | the diff pane toolbar |
| `comment on file` | the file header |
| `Suggested change` | the comment carrying a suggestion |
| `Resolve` | the comment's buttons |
| `round(s)` | the group summary line |
| `comment(s)` | the group summary line |

Add to this table when the picture is retaken and the new one shows a label the
old one did not. Removing a row is the same admission the other way round.

## Why this and not a pixel comparison

A `test/visual` case that retakes the shot and diffs it against the committed
picture is the obvious guard, and it was not chosen:

- `test/visual` does not run in CI (#317). A guard there catches nothing until
  that is fixed.
- It needs a browser. `test/visual` downloads Chromium; `go test ./...` needs a
  toolchain and nothing else, and is what CI already runs on every change.
- The noise floor is not zero, so the comparison needs a tolerance, and a
  tolerance is a number somebody has to keep honest as fonts and rendering
  move under it.

The string check has none of that. It runs in the same `go test ./...` the
repository already has, needs no browser, and catches the failure this file
exists for: checked against the bundle at `c17d3a3`, the commit whose picture
was wrong, `web/dist` held `Filter paths` and no `Search paths and lines`, so
the check goes red there. What it does not catch is a purely visual drift -
a colour, a spacing, a layout - with no label attached. That is a real gap, and
the picture at 1600x950 next to a rebuilt bundle is still the thing to look at
by eye when the UI moves.
