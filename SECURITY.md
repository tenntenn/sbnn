# Security Policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub, at
<https://github.com/tenntenn/sbnn/security/advisories/new>.

Please do not open a public issue for a suspected vulnerability. A public
issue is the right place once a fix is out, or when the report turns out to
be one of the expected behaviours listed under [Out of scope](#out-of-scope).

Include enough to reproduce it. For sbnn that usually means the diff text
(redacted as needed), the command that produced it, and what sbnn then did
with it.

There is no bounty programme and no guaranteed response time. This is a
single-maintainer project; reports are looked at as time allows.

## Threat model

sbnn trusts the person running the CLI, and nothing else that reaches it.

What it does **not** trust:

- **The diff text.** sbnn did not write it and does not vouch for it. It is
  parsed, rendered in a browser, and baked into exported pages.
- **The working-tree paths a diff names.** A diff can name any path at all;
  `source.AbsPath` (`internal/source/source.go`) exists to refuse the ones
  that leave the directory the diff was sent from.
- **Any web page the user happens to have open.** The server listens on
  loopback with no authentication, so any site the user visits can reach it.
  `Server.crossOrigin` (`internal/server/server.go`) is what keeps such a
  page from POSTing to it.

That last one matters because hooks are shell commands: `runHookCommand`
(`internal/server/hook.go`) runs the registered command through `/bin/sh -c`
with the review prompt on stdin.

## In scope

- **Escaping the containment check.** Any way to make sbnn read a file
  outside the directory a diff was sent from — `AbsPath` returning a path it
  should have refused, or a code path that reads a diff-named file without
  going through `AbsPath` at all. Symlink resolution is a known gap tracked
  in [#41](https://github.com/tenntenn/sbnn/issues/41); new findings there
  are welcome, but that one is already public.
- **XSS in the preview.** The Markdown and notebook previews render
  untrusted content and sanitise it with DOMPurify (`sanitize` in
  `web/src/markdown.ts`). A payload that survives that sanitiser, or a
  rendering path that reaches the DOM without it, is in scope — in the live
  review page and in an exported page alike.
- **Bypassing the cross-origin check.** Anything that lets a page on another
  site drive a state-changing request to sbnn: registering a hook,
  submitting a review, clearing a group. Registering a hook is the worst
  case, because it hands that page a shell command.
- **Leaking working-tree content into an export.** `sbnn export` bakes file
  contents into a single HTML page meant to be handed to someone else; a way
  to get content in there that the exporter did not intend to share.
- Anything else that lets untrusted input — the diff, or a remote page —
  execute code, read files, or write files outside what the CLI was pointed
  at.

## Out of scope

These are the tool working as designed. They are not vulnerabilities:

- **`--dangerously-allow-remote-access` doing what it says.** The flag binds
  the server to a non-loopback address with no authentication, and its help
  text says so: *"Allow binding to a non-loopback address (no
  authentication!)"*. Reaching an sbnn started with that flag from another
  machine is the flag working.
- **Hooks running commands the user registered.** A hook is a shell command,
  by design. The concern is a hook getting registered by someone other than
  the user (in scope, above), not a registered hook running.
- **A diff naming an arbitrary path.** Diffs can say anything. That is
  expected input; the containment check is the boundary. "The diff contains
  `../../.ssh/id_rsa`" is not a report. "sbnn read it" is.
- Findings that require an attacker who can already run commands as the user
  running sbnn.
- Vulnerabilities in dependencies, unless sbnn's own use of them is what
  makes them exploitable. Report those upstream.

## How the repository is guarded

What runs on every change, and what the repository settings turn on, so that
a weakness in sbnn's own supply chain is caught by something other than a
reader:

- **Dependencies.** `govulncheck` and `pnpm audit` run on every pull request,
  on `main` and weekly (`.github/workflows/security.yml`). Dependabot alerts
  are on, and Dependabot opens a pull request for a dependency with a known
  vulnerability; routine version updates are not its job.
- **Code.** CodeQL's default setup scans the Go, the TypeScript and the
  workflows. Its alerts are visible to maintainers only, like a private
  report.
- **Secrets.** Secret scanning is on, with push protection, so a push that
  carries a recognised token is refused rather than published.
- **Workflows.** Every action is pinned to a commit SHA and every tool run
  with `go run` to a version, and the `pinact` job fails on one that is not.
  Workflows hold `contents: read` unless a job needs more, and checkouts do
  not keep the token.

These are repository settings as much as files: if one of them is turned off,
this section is the record of what it was for.

## Updates

`sbnn update` replaces the sbnn binary, and a release binary checks for a newer
release on its own, so what they fetch and what they accept is spelled out
here. The code is `internal/selfupdate`.

- **What is contacted.** `https://api.github.com/repos/tenntenn/sbnn/releases/latest`
  (or `.../releases/tags/<tag>` for `sbnn update --version`) to learn the
  release, then `checksums.txt` and the archive for the running OS and
  architecture from `https://github.com/tenntenn/sbnn/releases/download/<tag>/`.
  Only HTTPS is used, and only to `api.github.com` and `github.com`; the
  redirect GitHub answers a download with is followed only to its asset hosts
  (`objects.githubusercontent.com`, `release-assets.githubusercontent.com`),
  over HTTPS. Every request has a timeout and every response a size cap.
- **What is sent.** A `User-Agent` naming the sbnn version. `GITHUB_TOKEN`, when
  it is set, is sent as a bearer token to `api.github.com` only, to raise the
  rate limit; it is removed from any redirect and never sent to a download
  host.
- **How the archive is verified.** Its SHA-256 has to match the entry for its
  name in the `checksums.txt` of the same release before anything is unpacked.
  A mismatch, a missing entry or a missing `checksums.txt` stops the update
  with nothing replaced. The checksum and the archive come from the same
  release, so this protects against a corrupted or swapped download, not
  against a compromised release; it is the same check a person downloading by
  hand would make.
- **What is unpacked.** Only the `sbnn` (`sbnn.exe`) entry at the top of the
  archive, which has to be a regular file under a size cap. An archive with an
  absolute path or a `..` in any entry is refused as a whole.
- **How the binary is replaced.** The new binary is written next to the real
  executable (symlinks resolved), given its permissions, and renamed over it,
  so it is either the old binary or the whole new one. On Windows the running
  executable is renamed aside first and removed by the next run. sbnn never
  elevates itself: when the directory is not writable, it says so and stops.
- **What is never done.** An older release is installed only when
  `--version` names it. A binary built with `go install` is not replaced; it is
  told to run `go install github.com/tenntenn/sbnn@latest`.
- **The start-up check.** At most once every 24 hours a release binary asks
  the API for the latest release, concurrently with the command and with a
  short timeout, and remembers the answer in `<state>/update-check.json`
  (mode 0600). It downloads nothing. It is skipped for source builds, when
  stderr is not a terminal, with `--json`, in the background server and in
  hook commands, and whenever `SBNN_NO_UPDATE_CHECK` is set to anything —
  set that to keep sbnn from contacting GitHub at all.

## Supported versions

sbnn has no releases yet: `version.Version` is `dev`
(`version/version.go`), and there are no tags. Fixes land on `main`, and
that is the only place to get them today.

Once releases exist ([#101](https://github.com/tenntenn/sbnn/issues/101)),
only the latest release will receive fixes, and this section needs updating
to say so with the version numbers of the day.
