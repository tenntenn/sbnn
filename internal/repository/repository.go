// Package repository finds the repository a directory belongs to, which is
// what decides the sbnn server an invocation talks to.
//
// It reads the files git leaves on disk and nothing else: sbnn never runs git
// (see AGENTS.md), and a repository is only ever needed here as a name to
// key a server on, not as something to ask questions of.
package repository

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// None is the key of the diffs sent from outside any repository. They get a
// server of their own rather than joining some repository's, and the empty
// string cannot be mistaken for a repository, whose key is always an
// absolute path.
const None = ""

// Find returns the root of the repository dir is in, or None when dir is in
// no repository.
//
// The root is the working tree of the main repository: a linked worktree
// leads back to the repository it was added to, so that every worktree of a
// repository shares one server and is told apart by --target. A submodule is
// a repository of its own, since its history and its reviews are not the
// superproject's. Symbolic links are resolved, so that /tmp/x and
// /private/tmp/x on macOS, or a checkout reached through a link, give the
// same answer.
func Find(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return None
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	for {
		dotgit := filepath.Join(dir, ".git")
		if fi, err := os.Stat(dotgit); err == nil {
			if fi.IsDir() {
				return dir
			}
			if fi.Mode().IsRegular() {
				return fromGitFile(dir, dotgit)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return None
		}
		dir = parent
	}
}

// fromGitFile follows the .git file of a linked worktree or a submodule,
// which names the directory git keeps the checkout's state in. dir is the
// directory holding the file, and is the answer whenever the file does not
// lead anywhere better: a .git file that cannot be read still marks the top
// of a checkout.
func fromGitFile(dir, dotgit string) string {
	gitdir := readGitdir(dotgit)
	if gitdir == "" {
		return dir
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	gitdir = canonical(gitdir)

	common := commonDir(gitdir)
	if common == "" {
		// No commondir and not under worktrees/: the state directory of a
		// submodule (.git/modules/<name>) or of a checkout made with
		// --separate-git-dir. Either way this checkout is its own repository.
		return dir
	}
	// A linked worktree. The main repository is the working tree whose .git
	// the common directory is.
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// The common directory is not anybody's .git: a worktree of a submodule
	// (.git/modules/<name>) or of a bare repository. The working tree it
	// belongs to, when it has one, is written down as core.worktree.
	if wt := coreWorktree(common); wt != "" {
		if !filepath.IsAbs(wt) {
			wt = filepath.Join(common, wt)
		}
		return canonical(wt)
	}
	// A bare repository has no main working tree at all, so the repository
	// is named by its common directory, which every worktree of it shares.
	return common
}

// readGitdir returns what a .git file points at.
func readGitdir(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := bytes.Cut(b, []byte("\n"))
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(line)), "gitdir:")
	if !ok {
		return ""
	}
	return strings.TrimSpace(rest)
}

// commonDir returns the directory the state of a linked worktree shares with
// its main repository, or "" when gitdir is not a linked worktree's.
//
// git writes the answer into the commondir file. The layout it always uses,
// <common>/worktrees/<name>, is the fallback for a commondir that is missing
// or unreadable.
func commonDir(gitdir string) string {
	if b, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		if c := strings.TrimSpace(string(b)); c != "" {
			if !filepath.IsAbs(c) {
				c = filepath.Join(gitdir, c)
			}
			return canonical(c)
		}
	}
	if filepath.Base(filepath.Dir(gitdir)) == "worktrees" {
		return filepath.Dir(filepath.Dir(gitdir))
	}
	return ""
}

// coreWorktree reads core.worktree out of the config file of a git
// directory. It understands just enough of the format to find that one key,
// which is all a submodule's config needs: git writes it there itself.
func coreWorktree(gitdir string) string {
	f, err := os.Open(filepath.Join(gitdir, "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	inCore := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inCore = strings.EqualFold(strings.Trim(line, "[] \t"), "core")
			continue
		}
		if !inCore {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "worktree") {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

// canonical cleans a path and resolves its links, keeping the cleaned path
// when it cannot be resolved: a key that is merely not canonical still beats
// none.
func canonical(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// Describe names a repository key for a person.
func Describe(key string) string {
	if key == None {
		return "the diffs from outside any repository"
	}
	return key
}
