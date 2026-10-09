package skills

import (
	"bytes"
	"strings"

	"golang.org/x/mod/semver"
)

// The skill carries the version of the sbnn that wrote it, in the front matter:
//
//	metadata:
//	  version: "1.2.3"
//
// The source file never holds one. The git tag is the only authority on a
// release (see package version), so the number is stamped when the skill is
// printed or installed, and a source build ("dev") stamps nothing. What an
// installed copy says is therefore the release of the binary that wrote it,
// which is what lets a binary tell a skill it should refresh from one that is
// newer than itself.

const versionKey = "version"

// splitFront cuts md into the front matter (without its delimiters) and what
// follows it, starting at the closing "---" line.
func splitFront(md []byte) (front, rest string, ok bool) {
	body, ok := strings.CutPrefix(string(md), "---\n")
	if !ok {
		return "", "", false
	}
	end := strings.Index(body, "\n---\n")
	if end < 0 {
		return "", "", false
	}
	return body[:end], body[end:], true
}

// Stamp returns md with version recorded in its front matter. An empty
// version or "dev" is no release, so md is returned unchanged; so is a file
// that has no front matter to put it in.
func Stamp(md []byte, version string) []byte {
	if version == "" || version == "dev" {
		return md
	}
	front, rest, ok := splitFront(md)
	if !ok {
		return md
	}
	var lines []string
	all := strings.Split(front, "\n")
	for i := 0; i < len(all); i++ {
		// Drop a stamp that is already there, so stamping twice is the
		// same as stamping once.
		if all[i] == "metadata:" && i+1 < len(all) && strings.HasPrefix(all[i+1], "  "+versionKey+":") {
			i++
			continue
		}
		lines = append(lines, all[i])
	}
	lines = append(lines, "metadata:", "  "+versionKey+": \""+version+"\"")
	return []byte("---\n" + strings.Join(lines, "\n") + rest)
}

// Version returns the release a SKILL.md was stamped with, or "" when it
// carries none (a copy installed by a source build, or one that predates the
// stamp).
func Version(md []byte) string {
	front, _, ok := splitFront(md)
	if !ok {
		return ""
	}
	lines := strings.Split(front, "\n")
	for i, l := range lines {
		if l != "metadata:" || i+1 >= len(lines) {
			continue
		}
		if v, ok := strings.CutPrefix(lines[i+1], "  "+versionKey+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// Action is what to do about an installed skill.
type Action int

const (
	// Current: the installed skill is what this binary carries.
	Current Action = iota
	// Refresh: the binary is newer, or the installed copy has no release
	// to defend.
	Refresh
	// Newer: the installed skill comes from a newer sbnn than this one, and
	// overwriting it would take it back to older text.
	Newer
)

// Decide compares the installed SKILL.md with the one the running binary
// would write (own, already stamped with binaryVersion).
//
// Content alone cannot say which side is newer, so releases decide when both
// have one: the binary refreshes a skill only when its own release is greater,
// and leaves a newer one alone. A side without a release - a source build, or
// a copy from before the stamp existed - carries no information to compare, so:
//
//   - a binary with no release never overwrites a skill that has one;
//   - a skill with no release is refreshed, by a released binary and by a
//     source build alike (which is how it behaved before the stamp).
//
// Equal content is Current whatever the versions say.
func Decide(installed, own []byte, binaryVersion string) Action {
	if bytes.Equal(installed, own) {
		return Current
	}
	sv := semverOf(Version(installed))
	bv := semverOf(binaryVersion)
	switch {
	case sv == "":
		return Refresh
	case bv == "":
		return Newer
	}
	if semver.Compare(bv, sv) < 0 {
		return Newer
	}
	// Greater, or the same release with different text (a modified copy):
	// the binary is the authority on what the skill says.
	return Refresh
}

// semverOf returns v in the form package semver wants, or "" when v is not a
// release.
func semverOf(v string) string {
	if v == "" || v == "dev" {
		return ""
	}
	s := "v" + strings.TrimPrefix(v, "v")
	if !semver.IsValid(s) {
		return ""
	}
	return s
}
