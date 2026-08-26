# The sbnn HTTP API

Everything sbnn does goes through this API. The `sbnn` command line is a client
of it (`internal/client`), the review page in the browser is another, and it is
the only way anything else can drive a review.

This file is the reference. It was written by starting a server and calling
every endpoint; the request and response bodies below are what came back, only
shortened where a diff repeats itself.

## Stability: unstable, and deliberately so

**`/_/api/` is not a stable interface.** sbnn is pre-1.0, there is no version
in the path, and no endpoint or field here is promised to survive a release.

That is worth knowing rather than guessing at. If you are writing a client:

- pin the sbnn version you built against, and read `version` out of
  [`GET /_/api/status`](#get-_apistatus) before assuming a shape;
- ignore fields you do not know, rather than failing on them. New ones are
  added without notice;
- expect fields marked *omitted when empty* below to be absent, not null.

Changes to what sbnn persists — the session file, the exported page and the
review log — are recorded under "Format changes" in
[CHANGELOG.md](../CHANGELOG.md). This API is not covered by that promise.

## Contents

- [Reaching the server](#reaching-the-server)
- [Conventions](#conventions)
- [The cross-origin rule](#the-cross-origin-rule)
- [Endpoints](#endpoints)
- [The event stream](#the-event-stream)
- [Payload shapes](#payload-shapes)

## Reaching the server

The server listens on `localhost:6280` by default (`--port`, `--bind`). Every
endpoint is under `http://localhost:6280`.

There is **no authentication**. sbnn binds loopback, and a page on another site
that tries to change something is refused by
[the cross-origin rule](#the-cross-origin-rule). Binding anything but loopback
needs `--dangerously-allow-remote-access`, and then anyone who can reach the
port can drive the review.

`GET /_/api/status` is also how a client finds out whether a server is running
at all: `internal/client` calls it before everything else and reports "no sbnn
server found on ..." when it does not answer.

## Conventions

**Media types.** Every JSON response is
`Content-Type: application/json; charset=utf-8`, indented with two spaces.
`GET .../prompt` answers `text/plain; charset=utf-8`, `GET .../image` and
`GET .../asset` answer the image's own type, and `GET /_/events` answers
`text/event-stream`.

**Errors are not JSON.** A refusal is `http.Error`: a plain-text line and a
newline, with `Content-Type: text/plain; charset=utf-8`.

```console
$ curl -i -X POST -d '{"verdict":"maybe"}' localhost:6280/_/api/groups/api/review
HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8

verdict must be approved, commented or changes-requested
```

The two preview endpoints are the exception: they answer their own failures
with `{"error": "..."}` as JSON, because the page draws that text.

**Group names.** `{group}` matches `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`. An
empty segment means the group `default`. Anything else is a 400:

```console
$ curl -i localhost:6280/_/api/groups/a%2Fb
HTTP/1.1 400 Bad Request

invalid group name "a/b": use letters, digits, '.', '-' or '_' (max 64)
```

**A group is created by writing to it.** There is no "create group" call:
`POST .../diffs` on an unknown name makes it. Reading an unknown group is not
an error either — `GET /_/api/groups/{group}` answers 200 with an empty one.

**Empty lists are `[]`, not `null`.** `diffs` and `comments` on a group are
always arrays, as are `GET .../comments`, `GET .../hooks` and `GET
/_/api/groups`. (They were `null` once; #29.)

**Body size limits.** 1 MB for a comment, a review or a hook; 32 MB for the
diff itself, inside a request bounded at 65 MB because JSON escaping can double
a string. Over the limit is a 413:

```console
$ curl -i -X POST --data @big.json localhost:6280/_/api/groups/api/comments
HTTP/1.1 413 Request Entity Too Large

the comment is too large (max 1MB)
```

**Malformed JSON** is a 400 naming the parse error:
`invalid request: invalid character 'o' looking for beginning of object key string`.

## The cross-origin rule

sbnn listens on loopback with no authentication, which means any website the
user visits can reach it. A `POST` from a page on `evil.example` would
otherwise register a hook, and a hook is a shell command sbnn runs.

So **every request that changes something must come from sbnn's own page or
from something that is not a browser at all.** Reads are exempt: without CORS
headers no other site gets to see the answer anyway.

| the request | outcome |
| --- | --- |
| `GET`, `HEAD`, `OPTIONS` | always allowed |
| `Sec-Fetch-Site: none` (the address bar) or `same-origin` | allowed |
| `Sec-Fetch-Site` anything else | **403** |
| no `Sec-Fetch-Site`, no `Origin` — curl, `sbnn`, a hook | allowed |
| `Origin: null` (a sandboxed page) | **403** |
| `Origin` naming this server | allowed |
| `Origin` naming anything else | **403** |

An `Origin` names this server when it matches the request's own `Host`, or when
it is `http`, carries sbnn's port, and its host is `localhost`, a loopback IP,
or whatever `--bind` was given — the page is reached by whichever loopback name
the user typed, and all of them count.

The refusal is one line:

```console
$ curl -i -X POST -H 'Origin: http://evil.example' \
    -d '{"command":"echo x"}' localhost:6280/_/api/groups/api/hooks
HTTP/1.1 403 Forbidden

sbnn only takes changes from its own page or from the command line
```

A client that is not a browser sends neither header and is never affected. A
browser-based client on another origin cannot be made to work: that is the
point of the rule, not an oversight.

## Endpoints

24 routes under `/_/api/`, plus `GET /_/events`. Everything else under `/` is
the review page itself.

### `GET /_/api/status`

What server this is, and what it is holding. → [`Status`](#status)

```console
$ curl localhost:6280/_/api/status
{
  "app": "sbnn",
  "version": "dev",
  "revision": "0b87768de8e9d453d3b60fd2a5c872768d230b8c",
  "pid": 18014,
  "url": "http://localhost:6501",
  "moUrl": "http://localhost:6275",
  "moProxyUrl": "http://127.0.0.1:37133",
  "moAvailable": false,
  "moError": "mo is not installed: install mo with `brew install k1LoW/tap/mo`, or download a binary from https://github.com/k1LoW/mo/releases",
  "groups": []
}
```

### `GET /_/api/reviews`

The submitted reviews from the review log, with a tally.
→ [`ReviewsResponse`](#reviewsresponse)

| query | meaning |
| --- | --- |
| `group` | only this group |
| `since` | `7d`, `2026-08-01`, or an RFC 3339 time |
| `limit` | at most this many, most recent first |

`limit` that is not a non-negative number is `400 limit must be a number`; an
unreadable `since` is a 400 naming what it could not read.

```console
$ curl 'localhost:6280/_/api/reviews?limit=1'
{
  "reviews": [
    {
      "group": "api",
      "reviewedAt": "2026-08-26T02:36:36.423229938Z",
      "firstDiffAt": "2026-08-26T02:35:40.176432192Z",
      "diffs": 1,
      "files": 2,
      "additions": 3,
      "deletions": 1,
      "note": "second look",
      "verdict": "changes-requested",
      "labels": { "pr": "42" },
      "comments": [
        {
          "path": "docs/guide.md",
          "author": "agent",
          "side": "new",
          "startLine": 3,
          "endLine": 3,
          "body": "Is this link relative?",
          "question": true,
          "resolved": true,
          "createdAt": "2026-08-26T02:35:57.203989113Z"
        }
      ]
    }
  ],
  "stats": { "reviews": 1, "comments": 1, "...": "see Stats below" }
}
```

The log is only kept when the server was started with a history file; without
one `reviews` is empty.

### `GET /_/api/groups`

One line per group. → array of [`GroupSummary`](#groupsummary)

```console
$ curl localhost:6280/_/api/groups
[
  {
    "name": "api",
    "url": "http://localhost:6501/api",
    "diffs": 1,
    "files": 2,
    "comments": 0,
    "unresolved": 0,
    "reviewed": false,
    "hooks": 0
  }
]
```

### `DELETE /_/api/groups`

Close every review. 200 with the count, always — removing nothing from an empty
server is what was asked for.

```console
$ curl -X DELETE localhost:6280/_/api/groups
{
  "removed": 0
}
```

### `GET /_/api/groups/{group}`

Everything the review page draws: the diffs, their files and hunks, and the
comments. → [`Group`](#group)

`raw` is blanked on this route — the review page never reads it and it is the
largest thing in the payload. It is present on
[`POST .../diffs`](#post-_apigroupsgroupdiffs) and on
[`POST .../review`](#post-_apigroupsgroupreview).

An unknown group is 200 with an empty one, not a 404.

```console
$ curl localhost:6280/_/api/groups/api
{
  "name": "api",
  "diffs": [
    {
      "id": "d1",
      "title": "Guide update",
      "baseDir": "/home/you/src/project",
      "createdAt": "2026-08-26T02:39:09.474410655Z",
      "labels": { "pr": "42" },
      "raw": "",
      "files": [
        {
          "id": "f1-07fdd026",
          "oldPath": "docs/guide.md",
          "newPath": "docs/guide.md",
          "status": "modified",
          "isBinary": false,
          "index": "1111111..2222222 100644",
          "additions": 3,
          "deletions": 1,
          "viewMode": "split",
          "isMarkdown": true,
          "isImage": false,
          "isNotebook": false,
          "hunks": [
            {
              "header": "@@ -1,3 +1,5 @@",
              "oldStart": 1, "oldLines": 3, "newStart": 1, "newLines": 5,
              "lines": [
                { "kind": "context", "oldNumber": 1, "newNumber": 1, "content": "# Guide" },
                { "kind": "delete", "oldNumber": 3, "newNumber": 0, "content": "See notes." },
                { "kind": "add", "oldNumber": 0, "newNumber": 3, "content": "See [notes](./notes.md)." }
              ]
            }
          ]
        }
      ]
    }
  ],
  "comments": []
}
```

### `DELETE /_/api/groups/{group}`

Close one review: its diffs, comments and hooks. `204` with no body, or `404 no
such group`.

### `POST /_/api/groups/{group}/diffs`

Add a unified diff. The group is created if it does not exist.
→ [`AddDiffRequest`](#adddiffrequest) → [`AddDiffResponse`](#adddiffresponse)

| status | when |
| --- | --- |
| 200 | stored |
| 400 | `empty diff`, or `no file diff found in the input` |
| 413 | past 32 MB |

```console
$ curl -X POST -H 'Content-Type: application/json' localhost:6280/_/api/groups/api/diffs -d '{
  "title": "Guide update",
  "baseDir": "/home/you/src/project",
  "content": "diff --git a/docs/guide.md b/docs/guide.md\n...",
  "labels": { "pr": "42" },
  "collapse": ["*.lock"]
}'
{
  "group": "api",
  "url": "http://localhost:6501/api",
  "diff": {
    "id": "d1",
    "title": "Guide update",
    "baseDir": "/home/you/src/project",
    "createdAt": "2026-08-26T02:35:40.176432192Z",
    "labels": { "pr": "42" },
    "raw": "diff --git a/docs/guide.md b/docs/guide.md\n...",
    "files": [ "..." ]
  }
}
```

`baseDir` is where sbnn looks for the working-tree file behind a preview, an
image and an asset. Without it those endpoints have only the diff to rebuild
from. `collapse` names files to fold away — sbnn matches the patterns and reads
nothing into them.

### `DELETE /_/api/groups/{group}/diffs/{diff}`

Drop one round. `204` with no body, or `404 no such diff`.

### `GET /_/api/groups/{group}/diffs/{diff}/files/{file}/preview`

The mo preview of a Markdown file, as URLs the page can frame.
→ [`PreviewResponse`](#previewresponse)

`404 no such file` for an unknown id. When mo cannot render it the body is
`{"error": "..."}` with mo's own status — `424` when mo is not installed:

```console
$ curl -i '.../files/f1-07fdd026/preview'
HTTP/1.1 424 Failed Dependency
Content-Type: application/json; charset=utf-8

{
  "error": "mo is not installed: install mo with `brew install k1LoW/tap/mo`, or download a binary from https://github.com/k1LoW/mo/releases"
}
```

### `GET /_/api/groups/{group}/diffs/{diff}/files/{file}/content`

The new side of a file as text, for a client that renders Markdown itself. This
is what the review page uses when the preview toggle is on sbnn's own renderer,
and what an exported page is built from.
→ [`FileContentResponse`](#filecontentresponse)

`assets` is the sibling images the Markdown points at, keyed by the reference
as the document wrote it, so a relative `src` can be pointed at something that
exists before the Markdown is drawn (#322).

```console
$ curl '.../files/f1-07fdd026/content'
{
  "path": "/home/you/src/project/docs/guide.md",
  "source": "worktree",
  "complete": true,
  "content": "# Guide\n\nSee [notes](./notes.md).\n\n![diagram](diagram.png)\n",
  "assets": {
    "diagram.png": {
      "url": "/_/api/groups/api/diffs/d1/files/f1-07fdd026/asset?path=docs%2Fdiagram.png",
      "path": "docs/diagram.png",
      "status": "ok",
      "size": 70
    }
  }
}
```

### `GET /_/api/groups/{group}/diffs/{diff}/files/{file}/image`

The bytes of a file that is itself an image, for a diff of a picture. The
response is the image with its own `Content-Type` and `Cache-Control:
no-store`; `404 no such file`, or `{"error": "..."}` when it cannot be read.

```console
$ curl -i '.../files/f2-47f2b048/image'
HTTP/1.1 200 OK
Cache-Control: no-store
Content-Type: image/png
Content-Length: 70
```

### `GET /_/api/groups/{group}/diffs/{diff}/files/{file}/asset`

One image a *Markdown* file points at, named by `?path=` with the repository
path out of the matching `assets` entry (#305). Same response shape as
`/image`. Only a path that entry named is served, so this is not a way to read
arbitrary files:

```console
$ curl -i '.../asset?path=docs%2Fnope.png'
HTTP/1.1 404 Not Found

{
  "error": "no preview for this file: docs/guide.md points at no image \"docs/nope.png\" that sbnn can show"
}
```

### `GET /_/api/groups/{group}/comments`

The comments of a group, oldest first. → array of [`Comment`](#comment)

### `POST /_/api/groups/{group}/comments`

Leave a comment. → [`AddCommentRequest`](#addcommentrequest) →
[`Comment`](#comment)

A client that only knows the path may leave `diffId` and `fileId` empty and let
the server resolve `path` against the newest diff. `snippet` is filled in from
the diff when it is not given.

| status | when |
| --- | --- |
| 200 | stored |
| 400 | an unusable `side`, a line range that cannot exist, a `fileId` naming no file of its diff, an empty body |
| 413 | past 1 MB |

```console
$ curl -X POST -H 'Content-Type: application/json' localhost:6280/_/api/groups/api/comments -d '{
  "path": "docs/guide.md", "side": "new", "startLine": 3, "endLine": 3,
  "author": "agent", "body": "Is this link relative?", "question": true
}'
{
  "id": "c1",
  "group": "api",
  "diffId": "d1",
  "fileId": "f1-07fdd026",
  "path": "docs/guide.md",
  "author": "agent",
  "side": "new",
  "startLine": 3,
  "endLine": 3,
  "body": "Is this link relative?",
  "question": true,
  "snippet": "+See [notes](./notes.md).",
  "resolved": false,
  "createdAt": "2026-08-26T02:35:57.203989113Z",
  "updatedAt": "2026-08-26T02:35:57.203989113Z"
}
```

### `PATCH /_/api/groups/{group}/comments/{id}`

Change a comment. → [`UpdateCommentRequest`](#updatecommentrequest) →
[`Comment`](#comment)

Every field is a pointer: omit one and it is left alone. `404 no such comment`
for an unknown id.

```console
$ curl -X PATCH -d '{"resolved":true}' localhost:6280/_/api/groups/api/comments/c1
{ "id": "c1", "...": "the whole comment, with resolved true and updatedAt moved" }
```

### `DELETE /_/api/groups/{group}/comments/{id}`

Drop one comment. `204` with no body, or `404 no such comment`.

### `DELETE /_/api/groups/{group}/comments`

Clear the comments of a group and say how many went. `?resolved=true` keeps
what is still open.

```console
$ curl -X DELETE 'localhost:6280/_/api/groups/api/comments?resolved=true'
{
  "removed": 1
}
```

### `GET /_/api/groups/{group}/prompt`

The comments as Markdown prose, ready to hand to an agent. `text/plain`, not
JSON — this is the body of `sbnn comments`.

| query | meaning |
| --- | --- |
| `resolved=true` | include the resolved comments too |
| `instruction=false` | leave off the closing instruction paragraph |

````console
$ curl 'localhost:6280/_/api/groups/api/prompt'
# Review comments (sbnn group "api")

1 comment(s) to address.

## 1. docs/guide.md:3

Diff: Guide update

From: agent

```
+See [notes](./notes.md).
```

Use a relative link?

```suggestion
See [notes](notes.md).
```

The suggestion block above replaces docs/guide.md:3.

---

Address every comment above. A suggestion block replaces the lines it names, verbatim. When a comment is not worth acting on, say why instead of changing the code.
````

`?instruction=false` stops after the last comment, leaving off the closing
paragraph.

### `POST /_/api/groups/{group}/review`

Submit the review. This is the moment the comments become worth reading and the
moment the hooks fire. → [`SubmitReviewRequest`](#submitreviewrequest) →
[`Group`](#group)

The body may be absent entirely; an empty one is a review that commented.
`verdict` is read leniently — case, spaces and `-`, `_`, `.` are folded away,
and `approve`, `accept`, `lgtm`, `ship` all mean approved, `reject` and
`changes` mean changes-requested. Anything else is
`400 verdict must be approved, commented or changes-requested`. `404 no such
group` for a group that was never written to.

```console
$ curl -X POST -d '{"note":"second look","verdict":"changes-requested"}' \
    localhost:6280/_/api/groups/api/review
{
  "name": "api",
  "diffs": [ "... with raw filled in ..." ],
  "comments": [ "..." ],
  "reviewedAt": "2026-08-26T02:36:36.423229938Z",
  "reviewNote": "second look",
  "reviewVerdict": "changes-requested"
}
```

### `GET /_/api/groups/{group}/hooks`

What runs when a review of this group is submitted.
→ array of [`Hook`](#hook)

Each hook carries how it last went, so a hook that fired while nobody was
watching can still be asked about. `lastCommandRun` and `lastPost` are absent
until that half has run once, and the two halves are recorded separately —
`h3` below ran its command and failed, then tried its URL and failed
differently.

```console
$ curl -s localhost:6280/_/api/groups/api/hooks
[
  {
    "id": "h1",
    "url": "http://localhost:6422/reviews",
    "createdAt": "2026-08-26T03:59:29.230539994Z",
    "lastPost": {
      "at": "2026-08-26T04:00:03.213754249Z",
      "ok": true,
      "detail": "200 OK"
    }
  },
  {
    "id": "h2",
    "command": "echo notified $SBNN_GROUP",
    "createdAt": "2026-08-26T03:59:59.038942595Z",
    "lastCommandRun": {
      "at": "2026-08-26T04:00:03.215200821Z",
      "ok": true,
      "detail": "notified api"
    }
  },
  {
    "id": "h3",
    "command": "exit 3",
    "url": "http://127.0.0.1:6499/dead",
    "createdAt": "2026-08-26T03:59:59.046635504Z",
    "lastCommandRun": {
      "at": "2026-08-26T04:00:03.214615029Z",
      "ok": false,
      "detail": "exit status 3"
    },
    "lastPost": {
      "at": "2026-08-26T04:00:03.215744799Z",
      "ok": false,
      "detail": "Post \"http://127.0.0.1:6499/dead\": dial tcp 127.0.0.1:6499: connect: connection refused"
    }
  }
]
```

### `POST /_/api/groups/{group}/hooks`

Register a hook. → [`Hook`](#hook) in and out.

A hook carries a `command` run through the shell, a `url` sent a JSON POST, or
both. `400 a hook needs a command or a url` for neither. Registering the same
pair twice returns the existing hook rather than a duplicate.

A `url` is checked here, against the same rule delivery applies, and a URL
that could never be delivered to is refused with a `400` while the person who
typed it is still there to read it — otherwise the typo is stored, persisted,
listed back looking healthy, and fails once per review into a log file. The
body is the plain-text reason, not JSON:

| `url` | status | body |
| --- | --- | --- |
| `example.com/reviews` | 400 | `a hook url has to start with http:// or https://: "example.com/reviews"` |
| `localhost:9000/reviews` | 400 | `a hook url has to be http or https, not "localhost": "localhost:9000/reviews"` |
| `ftp://example.com/reviews` | 400 | `a hook url has to be http or https, not "ftp": "ftp://example.com/reviews"` |
| `http:///reviews` | 400 | `a hook url needs a host: "http:///reviews"` |
| `ht tp://x` | 400 | `"ht tp://x" is not a url: parse "ht tp://x": first path segment in URL cannot contain colon` |

`localhost:9000/reviews` is the one worth knowing about: `url.Parse` reads
`localhost` as the scheme, so a host:port with no `http://` in front is not
a relative URL that could be guessed at — it is a URL with a scheme sbnn
cannot speak.

```console
$ curl -s -w '\nHTTP %{http_code}\n' -X POST -d '{"url":"localhost:9000/reviews"}' \
    localhost:6280/_/api/groups/api/hooks
a hook url has to be http or https, not "localhost": "localhost:9000/reviews"

HTTP 400
```

The same URL with `http://` in front is what it was meant to be:

```console
$ curl -X POST -d '{"url":"http://localhost:9000/reviews"}' \
    localhost:6280/_/api/groups/api/hooks
{
  "id": "h2",
  "url": "http://localhost:9000/reviews",
  "createdAt": "2026-08-26T02:36:11.377141242Z"
}
```

A `command` cannot be checked without running it, so its half is answered by
the recorded outcome in [`lastCommandRun`](#hookrun) instead.

**This is the most dangerous call in the API** — a command hook is a shell
command the server runs on the user's machine — and it is why
[the cross-origin rule](#the-cross-origin-rule) exists.

The JSON a URL hook is POSTed is not `Group`. It is
`{group, url, reviewedAt, note, verdict, comments, prompt}`, where `comments`
is the open comments and `prompt` is the same text `GET .../prompt` returns.
See `ReviewEvent` in `internal/server/hook.go`.

### `DELETE /_/api/groups/{group}/hooks/{id}`

Drop one hook. 200 with the count, or `404 no such hook` when the id matched
nothing — a typo must not look like a success.

### `DELETE /_/api/groups/{group}/hooks`

Drop them all. 200 with the count, whatever it is.

```console
$ curl -X DELETE localhost:6280/_/api/groups/api/hooks
{
  "removed": 1
}
```

### `POST /_/api/shutdown`

Stop the server. It answers first and exits after.

```console
$ curl -X POST localhost:6280/_/api/shutdown
{
  "status": "shutting down"
}
```

## The event stream

### `GET /_/events`

Server-sent events, for a client that wants to know when something happened
rather than polling. This is what `sbnn wait` blocks on.

```console
$ curl -N localhost:6280/_/events
HTTP/1.1 200 OK
Cache-Control: no-cache
Connection: keep-alive
Content-Type: text/event-stream

retry: 2000

data: {"group":"api","type":"change"}

id: 1
data: {"comments":0,"group":"api","reviewedAt":"2026-08-26T02:36:26.442194342Z","type":"review","verdict":"approved"}
```

Two kinds of event, told apart by `type`:

| `type` | payload | id |
| --- | --- | --- |
| `change` | `{"type":"change","group":"<name>"}` — a diff, comment or hook moved. `group` is empty when every group went at once. | none |
| `review` | `{"type":"review","group","reviewedAt","comments","verdict"}` — a review was submitted. `comments` is how many are still open. | a counter |

A `:` ping goes out every 25 seconds so an idle connection is not collected.

**Only `review` events carry an id, and only they are replayed.** Send
`Last-Event-ID: <n>` (a browser's `EventSource` does this by itself on
reconnect) to be handed the review notices after `n` that you missed. A client
opening the stream for the first time is given nothing: it has missed nothing
by definition, and replaying to `sbnn wait` would have it return a review
submitted before anyone asked it to wait.

**The replay is a snapshot, not a backlog.** The server keeps one stored
notice per group — the most recent — so a reconnect is answered with at most
one `review` event per group, whatever `n` you send. Review a group four times
while disconnected and you are handed the fourth, not all four:

```console
$ curl -N -H 'Last-Event-ID: 1' localhost:6280/_/events
retry: 2000

id: 4
data: {"type":"review","group":"gv", ...}

id: 5
data: {"type":"review","group":"api", ...}
```

Ids 2 and 3 were also reviews of `gv` and do not arrive; nothing in the id
sequence says so. This is deliberate rather than a gap to be closed. Both
clients want the current verdict: the browser refetches the group on any
notice, so repeats would be identical refetches, and `sbnn wait` asks whether
the group has been reviewed, not how often. Keeping the full history would
also make the store grow for the life of the process. So treat a resumed
stream as "here is where each group stands now", and read the round-by-round
history from [`GET /_/api/reviews`](#get-_apireviews) instead, which is where
it is kept.

A `change` notice may be dropped for a subscriber that has fallen behind; a
`review` notice is not.

At most 64 streams may be open at once. The 65th is
`503 too many event subscribers` — a plain refusal rather than a 200 that ends
immediately.

There is no `group` filter: every subscriber sees every group's events and
picks out its own by the `group` field.

## Payload shapes

Every field below is a JSON name of a Go type in this repository, and
`docs/doccheck/api_test.go` walks those types with reflection and fails when
one of them is missing from this file. A field cannot be renamed or added
without this document being updated in the same change — which is what #110
was: a hand-written description of a struct that drifted from it in both
directions.

*Omitted when empty* means the key is absent, not null.

#### `Status`

`GET /_/api/status`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `app` | string | always `"sbnn"` |
| `version` | string | `dev` for a build from source |
| `revision` | string | the commit. Omitted when empty |
| `pid` | number | the server process |
| `url` | string | where the review page is |
| `moUrl` | string | the mo server |
| `moProxyUrl` | string | mo through sbnn's frameable proxy. Omitted when there is none |
| `moAvailable` | bool | whether mo can render a preview |
| `moError` | string | why not, when it cannot. Omitted when it can |
| `groups` | array | [`GroupSummary`](#groupsummary) |
| `sessionError` | string | why the session is not reaching disk. Omitted while it is |

#### `GroupSummary`

`GET /_/api/groups`, and `groups` in `Status`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `name` | string | |
| `url` | string | the review page for this group |
| `diffs` | number | rounds |
| `files` | number | files across them |
| `comments` | number | |
| `unresolved` | number | |
| `reviewedAt` | time | omitted when never reviewed |
| `reviewed` | bool | false again once a diff arrives after the last review |
| `hooks` | number | |

#### `Group`

`GET /_/api/groups/{group}`, `POST .../review`. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `name` | string | |
| `diffs` | array | [`Diff`](#diff). `[]`, never null |
| `comments` | array | [`Comment`](#comment). `[]`, never null |
| `reviewedAt` | time | omitted when never reviewed |
| `reviewNote` | string | omitted when empty |
| `reviewVerdict` | string | `approved`, `commented`, `changes-requested`. Omitted when empty |
| `hooks` | array | [`Hook`](#hook). Omitted when there are none |

#### `Diff`

One round. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `id` | string | `d1`, `d2`, … per group |
| `title` | string | |
| `baseDir` | string | where the working-tree files are |
| `createdAt` | time | |
| `labels` | object | string to string, carried through untouched. Omitted when empty |
| `raw` | string | the diff text. Blanked on `GET /_/api/groups/{group}` |
| `files` | array | [`File`](#file) |

#### `File`

One file of a round. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `id` | string | index in the diff plus a hash of the path, `f1-07fdd026`. Unique only within its diff |
| `oldPath` | string | |
| `newPath` | string | |
| `status` | string | `added`, `modified`, `deleted`, `renamed`, `copied` |
| `isBinary` | bool | |
| `oldMode` | string | omitted when unchanged |
| `newMode` | string | omitted when unchanged |
| `index` | string | the diff's index line. Omitted when absent |
| `additions` | number | |
| `deletions` | number | |
| `viewMode` | string | `split` or `unified`, sbnn's suggestion |
| `folded` | bool | folded away on arrival. Omitted when false |
| `foldReason` | string | why. Omitted when empty |
| `isMarkdown` | bool | |
| `isImage` | bool | |
| `isNotebook` | bool | |
| `hunks` | array | [`Hunk`](#hunk). Empty for a binary file |

#### `Hunk`

`internal/model`.

| field | type | notes |
| --- | --- | --- |
| `header` | string | the `@@` line as written |
| `oldStart` | number | |
| `oldLines` | number | |
| `newStart` | number | |
| `newLines` | number | |
| `section` | string | the function or section after the `@@`. Omitted when absent |
| `lines` | array | [`Line`](#line) |

#### `Line`

`internal/model`.

| field | type | notes |
| --- | --- | --- |
| `kind` | string | `context`, `add` or `delete` |
| `oldNumber` | number | 0 on an added line |
| `newNumber` | number | 0 on a deleted line |
| `content` | string | without the leading marker |
| `noNewline` | bool | the file ends without one. Omitted when false |

#### `Comment`

`GET/POST .../comments`, `PATCH .../comments/{id}`. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `id` | string | `c1`, `c2`, … per server |
| `group` | string | |
| `diffId` | string | |
| `fileId` | string | |
| `path` | string | |
| `author` | string | empty for the reviewer in the browser. Omitted when empty |
| `side` | string | `old` or `new` |
| `startLine` | number | 0 for a comment about the file rather than a line |
| `endLine` | number | |
| `body` | string | Markdown |
| `question` | bool | wants an answer, not a change. Omitted when false |
| `snippet` | string | the lines commented on, as the diff wrote them |
| `resolved` | bool | |
| `createdAt` | time | |
| `updatedAt` | time | |
| `suggestions` | array | the suggestion blocks parsed out of `body`, added by `MarshalJSON`. Omitted when there are none |

`suggestions` is the one field with no struct field behind it: it is computed
on the way out so a client does not have to know how a suggestion is written
down. It is read-only — sending it back does nothing.

#### `Hook`

`GET/POST .../hooks`. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `id` | string | `h1`, `h2`, … per server |
| `command` | string | run through the shell. Omitted when empty |
| `url` | string | sent a JSON POST. Omitted when empty |
| `createdAt` | time | |
| `lastCommandRun` | [`HookRun`](#hookrun) | how `command` went the last time it ran. Omitted until it has run once |
| `lastPost` | [`HookRun`](#hookrun) | how the POST to `url` went the last time. Omitted until it has run once |

A hook fires when nobody is waiting, which is exactly when a silent failure
costs the most, so each half keeps its own outcome and `GET .../hooks` lists
it back. The two are separate because a hook with both a `command` and a
`url` can have one half succeed and the other fail — see `h3` in the example
under `GET .../hooks` above. Neither field is accepted on the way in:
`POST .../hooks` reads only `command` and `url`. `sbnn hook` prints the same
two outcomes on the command line.

#### `HookRun`

The outcome of one attempt to run one half of a hook. `internal/model`.

| field | type | notes |
| --- | --- | --- |
| `at` | time | when the attempt finished |
| `ok` | bool | the command exited 0, or the endpoint answered below 300 |
| `detail` | string | the one-line reason. Omitted when empty |

`detail` is folded onto one line and cut at 200 characters, because it is
written to the session file on every review and shown on one line by
`sbnn hook`. What goes in it:

| case | `ok` | `detail` |
| --- | --- | --- |
| command exited 0 | `true` | its combined output, e.g. `notified api`. Absent when it printed nothing, so a silent success is `{"at": ..., "ok": true}` |
| command exited non-zero | `false` | `exit status 3`, plus `: ` and the output when there was any |
| POST answered below 300 | `true` | the status, e.g. `200 OK` |
| POST answered 300 or above | `false` | `refused with ` and the status, e.g. `refused with 404 Not Found` |
| POST never got there | `false` | the transport error, e.g. `Post "http://127.0.0.1:6499/dead": dial tcp 127.0.0.1:6499: connect: connection refused` |

#### `AddDiffRequest`

`POST .../diffs`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `title` | string | a name is generated when empty |
| `baseDir` | string | where the working-tree files are |
| `content` | string | the unified diff |
| `labels` | object | carried through to the review record. Omitted when empty |
| `collapse` | array | patterns for files to fold away. Omitted when empty |

#### `AddDiffResponse`

`POST .../diffs`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `group` | string | the group it landed in |
| `url` | string | the review page to open |
| `diff` | object | [`Diff`](#diff), with `raw` filled in |

#### `AddCommentRequest`

`POST .../comments`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `diffId` | string | may be empty: `path` is resolved against the newest diff |
| `fileId` | string | may be empty |
| `author` | string | |
| `path` | string | |
| `side` | string | `old` or `new`, read leniently |
| `startLine` | number | |
| `endLine` | number | |
| `body` | string | |
| `snippet` | string | filled in from the diff when empty |
| `question` | bool | |
| `suggestion` | string | appended to `body` as a fenced suggestion block |

#### `UpdateCommentRequest`

`PATCH .../comments/{id}`. `internal/server`. Every field is optional; an
omitted one is left alone.

| field | type |
| --- | --- |
| `body` | string |
| `resolved` | bool |
| `question` | bool |

#### `SubmitReviewRequest`

`POST .../review`. `internal/server`. The whole body may be omitted.

| field | type | notes |
| --- | --- | --- |
| `note` | string | |
| `verdict` | string | empty means commented. Omitted when empty |

#### `PreviewResponse`

`GET .../preview`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `url` | string | the frameable mo page. Empty when the proxy could not start |
| `moUrl` | string | the same page on mo itself |
| `path` | string | the file mo was pointed at |
| `source` | string | `worktree` or `reconstructed` |
| `complete` | bool | whether the previewed Markdown is the whole file |

#### `FileContentResponse`

`GET .../content`. `internal/server`.

| field | type | notes |
| --- | --- | --- |
| `path` | string | the file on disk |
| `source` | string | `worktree` or `reconstructed` |
| `complete` | bool | a diff carries only its hunks, so a rebuilt file is complete only when it was added |
| `content` | string | the new side of the file |
| `assets` | object | the reference as the document wrote it → [`Entry`](#entry). Omitted when there are none |

#### `Entry`

One image a Markdown preview points at. `internal/asset`.

| field | type | notes |
| --- | --- | --- |
| `url` | string | where to fetch it. Omitted unless `status` is `ok` |
| `path` | string | the repository path, for the placeholder. Omitted when empty |
| `status` | string | `ok`, `too-large`, `over-budget`, `outside`, `missing`, `unsupported` |
| `size` | number | bytes. Omitted when unknown |

#### `ReviewsResponse`

`GET /_/api/reviews`. `internal/server`.

| field | type |
| --- | --- |
| `reviews` | array of [`Record`](#record) |
| `stats` | [`Stats`](#stats) |

#### `Record`

One submitted review. `internal/history`.

| field | type | notes |
| --- | --- | --- |
| `group` | string | |
| `reviewedAt` | time | |
| `firstDiffAt` | time | when the round opened. Omitted when unknown |
| `diffs` | number | |
| `files` | number | |
| `additions` | number | |
| `deletions` | number | |
| `note` | string | omitted when empty |
| `verdict` | string | omitted when empty |
| `labels` | object | omitted when empty |
| `comments` | array | the comments as they stood, below |

#### `history.Comment`

A comment inside a `Record`. Not `model.Comment`: a review record keeps what
was said, not the ids of a round that may be gone.

| field | type | notes |
| --- | --- | --- |
| `path` | string | |
| `author` | string | omitted when empty |
| `side` | string | |
| `startLine` | number | |
| `endLine` | number | |
| `body` | string | |
| `suggestions` | array | omitted when there are none |
| `question` | bool | omitted when false |
| `resolved` | bool | |
| `createdAt` | time | |

#### `Stats`

The tally over the reviews returned. `internal/history`.

| field | type | notes |
| --- | --- | --- |
| `reviews` | number | |
| `comments` | number | |
| `suggestions` | number | |
| `resolved` | number | |
| `files` | number | |
| `additions` | number | |
| `deletions` | number | |
| `silent` | number | reviews that said nothing |
| `approved` | number | |
| `commented` | number | |
| `changesRequested` | number | |
| `commentsPerReview` | number | |
| `medianWaitNanos` | number | first diff to review, in nanoseconds |
| `paths` | array | [`Count`](#count), most commented first |
| `extensions` | array | [`Count`](#count) |
| `authors` | array | [`Count`](#count) |
| `first` | time | oldest review counted. Omitted when there are none |
| `last` | time | newest. Omitted when there are none |

#### `Count`

| field | type |
| --- | --- |
| `key` | string |
| `count` | number |

## See also

- [README.md](../README.md) — the command line, which is a client of all this
- `internal/client` — the Go client the `sbnn` command uses
- `internal/server/hook.go` — `ReviewEvent`, the payload a URL hook is POSTed
