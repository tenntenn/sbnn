# Changelog

From the first tagged release onwards, the sections below are maintained by
[tagpr](https://github.com/Songmu/tagpr): it collects the merged pull requests
into the release pull request it opens, and that is what lands here. Entries
under `## Unreleased` are written by hand, and anything about a persisted
format belongs under [Format changes](#format-changes) as well.

## Unreleased

Nothing released yet. `version.Version` is still `dev` and the repository has
no tags, so every build is from source and there is no version number to
compare against.

## Format changes

sbnn writes three things that outlive the process, and each makes a promise
about how it may change. When one of them does change, record it here — which
version, which field, what a reader of the old shape should do. Only changes
from a tagged release onwards can be recorded; nothing below is filled in
retroactively, because there is no release history to reconstruct it from.

### Session file

The session sbnn keeps between runs, written by `internal/server/store.go`. Its
JSON carries an explicit schema version:

```go
type persisted struct {
	Version int            `json:"version"`
	Seq     int            `json:"seq"`
	Groups  []*model.Group `json:"groups"`
}

const persistVersion = 1
```

Current schema version: **1**. A change that older sbnn cannot read is a bump
of `persistVersion`, and belongs here with what happens to an existing session
file when it meets the newer binary.

### `reviews.jsonl`

The review log, one flat JSON object per line, kept outside the working tree
(`$XDG_STATE_HOME/sbnn/reviews.jsonl` by default). It is meant to be kept for a
long time, shared, and read with `jq`, and `sbnn reviews --help` makes the
compatibility promise in as many words:

> Parse the jsonl form - one flat JSON object per line, fields get added but
> not renamed.

So: **fields may be added, and are not renamed or removed.** Every added field
goes here with the version that started emitting it — that is the question a
year-old log raises and nothing else in the repository answers.

### `sbnn export` payload

The data embedded in an exported HTML page, from `internal/export/export.go`,
versioned by `PayloadVersion` (`"version"` in the payload).

Current payload version: **1**. An exported page is a self-contained artefact
that someone else opens, possibly long after it was made, so a bump belongs
here together with whether older pages keep rendering.
