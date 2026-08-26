// Package version carries the build information of sbnn.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is the sbnn version.
//
// It stays "dev" for a build from a source tree, and is filled in two ways
// for a build of a release. A binary from the releases page is built by
// GoReleaser, which passes the tag at link time with
// -X github.com/tenntenn/sbnn/version.Version=<version>. A binary from
// `go install github.com/tenntenn/sbnn@v1.2.3` carries no linker flags at
// all, so the module version out of the build info is the only place the
// release can be read from (see versionFrom).
//
// Nothing in the tree writes this. In particular .tagpr says
// "versionFile = -" on purpose: pointing tagpr at this file would have it
// rewrite the version-shaped strings in the comments above and then release
// whichever number it read back out of them. See
// TestTagprWritesNoVersionFile.
var Version = "dev"

// Revision is the commit sbnn was built from. It is read from the build info
// at startup (see revision.go), and may also be set at link time with
// -X github.com/tenntenn/sbnn/version.Revision=<commit>.
var Revision = "HEAD"

// pseudoVersion matches the tail every Go pseudo-version ends in: a UTC
// timestamp and the short commit it was built from. The separator before the
// timestamp is a dash when no tag precedes the commit
// (v0.0.0-20260826015029-0b87768de8e9) and a dot when one does
// (v1.2.4-0.20260826015029-0b87768de8e9), so both are accepted.
//
// A pseudo-version names a commit rather than a release - `go install
// ...@latest` produces one for a repository with no tag reachable - and
// Revision already says which commit this is, so it is not a version to
// report.
var pseudoVersion = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}$`)

// versionFrom picks the release out of the build info, and returns fallback
// when the build carries none.
//
// Only a real module version counts. Since Go 1.24 a `go build` of a checkout
// is stamped with a pseudo-version derived from the commit rather than left
// at "(devel)", so both of those - and a build that reports nothing at all -
// mean "not a release". The "v" a Go module version starts
// with is dropped so that a binary installed with `go install ...@v1.2.3` and
// one downloaded from the releases page, which GoReleaser stamps without it,
// answer `sbnn --version` with the same string.
func versionFrom(bi *debug.BuildInfo, ok bool, fallback string) string {
	if !ok || bi == nil {
		return fallback
	}
	v := bi.Main.Version
	// Build metadata is stripped before the shape is judged, not after: a
	// `go build` of a modified checkout is stamped
	// v0.0.0-20260826015029-0b87768de8e9+dirty, and the +dirty on the end
	// would otherwise stop that being recognised as the pseudo-version it
	// is. It is kept in what is reported, because a binary that says it was
	// built from a dirty tree should keep saying so.
	base, _, _ := strings.Cut(v, "+")
	if base == "" || base == "(devel)" || pseudoVersion.MatchString(base) {
		return fallback
	}
	return strings.TrimPrefix(v, "v")
}

func init() {
	bi, ok := debug.ReadBuildInfo()
	Version = versionFrom(bi, ok, Version)
}
