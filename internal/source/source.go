// Package source recovers the new side of a changed file.
//
// A unified diff only carries the changed hunks, so the working tree file is
// the better source whenever it is still there; otherwise the new side is
// rebuilt from the diff, which is complete only for added files.
package source

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

// Kind tells where the content came from.
type Kind string

const (
	// FromWorktree means the file was read from disk.
	FromWorktree Kind = "worktree"
	// FromDiff means the content was rebuilt out of the diff.
	FromDiff Kind = "reconstructed"
)

// Result is the recovered new side of a file.
type Result struct {
	Content string
	Kind    Kind
	// Path is the working tree file the content was read from, empty for
	// rebuilt content.
	Path string
	// Complete reports whether Content is the whole file.
	Complete bool
}

// NewSide returns the content of a file after the change. baseDir is the
// directory the diff paths are relative to.
func NewSide(baseDir string, f *model.File) Result {
	if path := AbsPath(baseDir, f.Path()); path != "" {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			if b, err := os.ReadFile(path); err == nil {
				return Result{Content: string(b), Kind: FromWorktree, Path: path, Complete: true}
			}
		}
	}
	content, complete := diff.Reconstruct(f)
	return Result{Content: content, Kind: FromDiff, Complete: complete}
}

// AbsPath resolves a diff path against the directory the diff was sent
// from, and returns "" for anything that lands outside it.
//
// The paths come out of the diff text, which sbnn did not write and does not
// vouch for: a patch someone mailed over can name ../../.ssh/id_rsa as
// happily as it names a file of the project. What is read from disk here is
// shown in the preview and baked into an exported page, so a path that
// leaves the directory the diff was sent from is refused and rebuilt from
// the diff instead.
//
// Climbing out with ".." is only the obvious way out. A path that stays
// inside lexically still leaves the directory when a symlink on the way
// points elsewhere, and a symlink is something a patch can add to the tree
// as easily as it names one that is already there. So both the base and the
// candidate are resolved before they are compared, and the comparison is
// what decides. The returned path is the unresolved one, so that the reader
// is shown the file where the diff says it is.
func AbsPath(baseDir, rel string) string {
	if rel == "" || baseDir == "" {
		return ""
	}
	base := filepath.Clean(baseDir)
	var abs string
	if filepath.IsAbs(rel) {
		abs = filepath.Clean(rel)
	} else {
		abs = filepath.Clean(filepath.Join(base, filepath.FromSlash(rel)))
	}
	if !within(base, abs) {
		return ""
	}
	realBase, err := evalSymlinks(base)
	if err != nil {
		return ""
	}
	realAbs, err := evalSymlinks(abs)
	if err != nil {
		return ""
	}
	if !within(realBase, realAbs) {
		return ""
	}
	return abs
}

// evalSymlinks is filepath.EvalSymlinks for a path that need not exist yet.
//
// A diff routinely names a file that is not on disk, and EvalSymlinks fails
// outright on those, so the path is resolved as far as it does exist and the
// missing tail is appended to the result. That still catches a symlinked
// directory on the way, which is what the check is after; only the names
// below the last existing directory are taken at face value, and those
// cannot be symlinks because they are not there at all.
func evalSymlinks(path string) (string, error) {
	var rest string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		// A dangling symlink is an entry that exists without resolving,
		// which is not the same as a name that is free to be created.
		// Refuse it rather than treat it as a file yet to be written.
		if _, lerr := os.Lstat(path); lerr == nil {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}

// within reports whether path is base itself or something under it.
func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}
