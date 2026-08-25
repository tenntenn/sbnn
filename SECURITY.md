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

## Supported versions

sbnn has no releases yet: `version.Version` is `dev`
(`version/version.go`), and there are no tags. Fixes land on `main`, and
that is the only place to get them today.

Once releases exist ([#101](https://github.com/tenntenn/sbnn/issues/101)),
only the latest release will receive fixes, and this section needs updating
to say so with the version numbers of the day.
