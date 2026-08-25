package server

import (
	"path"
	"strings"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// foldFiles decides which files of a diff open shut, and says why for each
// one.
//
// There are exactly two reasons, and neither of them is sbnn's opinion:
//
//   - the sender asked for it, by naming the file in --collapse. Whoever
//     produced the diff is the one who knows what their generated files are
//     called, so that knowledge comes in with the diff instead of being
//     guessed at here;
//   - the file says so itself, in a "DO NOT EDIT" or "@generated" line put
//     there by whatever wrote it.
//
// Nothing is folded on a hunch - not by size, not by directory, not by
// extension. A review is the wrong place to be clever: a file folded for a
// bad reason is a file nobody read.
func foldFiles(d *model.Diff, patterns []string) {
	for _, f := range d.Files {
		if pattern, ok := matchAny(patterns, f.Path()); ok {
			f.Folded = true
			f.FoldReason = "the sender asked for it (--collapse " + pattern + ")"
			continue
		}
		if marker := declaredGenerated(d, f); marker != "" {
			f.Folded = true
			f.FoldReason = "the file says so: " + shorten(marker, 60)
			continue
		}
		// A previous round may have folded it; nothing says so now.
		f.Folded, f.FoldReason = false, ""
	}
}

// declaredGenerated returns the line by which a file declares itself
// generated.
//
// The working tree file is the whole file, so it answers properly. Failing
// that - a patch from elsewhere, a file already deleted - only what the diff
// shows can be read, and a unified diff reaches the top of a file just when
// its first hunk starts there. When neither can answer, the file is left
// alone: a declaration that cannot be found is not a declaration that is
// absent, and folding on that difference would be guessing.
func declaredGenerated(d *model.Diff, f *model.File) string {
	if f.IsBinary {
		return ""
	}
	if got := source.NewSide(d.BaseDir, f); got.Kind == source.FromWorktree {
		return diff.GeneratedMarker(got.Content)
	}
	return diff.GeneratedMarker(diff.VisibleTop(f))
}

// matchAny reports which pattern a path matches, if any.
//
// A pattern with a slash in it is matched against the whole path, one
// without against the file name at any depth - the shape people already know
// from .gitignore. "**" stands for any run of directories.
func matchAny(patterns []string, p string) (string, bool) {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchPath(pattern, p) {
			return pattern, true
		}
	}
	return "", false
}

func matchPath(pattern, p string) bool {
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, p)
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(p)); ok {
			return true
		}
		return false
	}
	ok, _ := path.Match(pattern, p)
	return ok
}

// matchDoubleStar handles the one thing path.Match cannot: "**", standing
// for any run of directories - anywhere in the pattern, as many times as the
// sender cares to write it. The pattern and the path are cut into segments
// and walked together, because "**" is about whole directory steps and
// path.Match cannot see a "/" at all.
func matchDoubleStar(pattern, p string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

// matchSegments matches path segments against pattern segments. A pattern
// segment of exactly "**" stands for any number of path segments, none
// included - except as the last segment of the pattern, where "dir/**" means
// what is inside dir and so wants at least one. Every other segment is
// path.Match against a single segment, so "*" stops at a "/" the way it
// always did.
func matchSegments(pattern, p []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return len(p) > 0
			}
			for i := 0; i <= len(p); i++ {
				if matchSegments(pattern[1:], p[i:]) {
					return true
				}
			}
			return false
		}
		if len(p) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], p[0]); !ok {
			return false
		}
		pattern, p = pattern[1:], p[1:]
	}
	return len(p) == 0
}

func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
