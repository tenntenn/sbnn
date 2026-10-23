package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// layout writes files under root: a path ending in "/" is a directory, and
// anything else a file with the given content.
func layout(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if name[len(name)-1] == '/' {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		// dir is where Find is asked from, and want the answer, both
		// relative to the test's root; want "" is None.
		dir, want string
		// link, when set, is a symbolic link made at link -> dir, and
		// Find is asked from the link instead.
		link string
	}{
		{
			name:  "the top of a repository",
			files: map[string]string{"repo/.git/": ""},
			dir:   "repo",
			want:  "repo",
		},
		{
			name:  "a nested directory",
			files: map[string]string{"repo/.git/": "", "repo/a/b/c/": ""},
			dir:   "repo/a/b/c",
			want:  "repo",
		},
		{
			name:  "outside any repository",
			files: map[string]string{"plain/a/": ""},
			dir:   "plain/a",
			want:  "",
		},
		{
			name: "a linked worktree with commondir",
			files: map[string]string{
				"repo/.git/worktrees/wt/commondir": "../..\n",
				"wt/.git":                          "gitdir: ../repo/.git/worktrees/wt\n",
				"wt/sub/":                          "",
			},
			dir:  "wt/sub",
			want: "repo",
		},
		{
			name: "a linked worktree with an absolute gitdir and no commondir",
			files: map[string]string{
				"repo/.git/worktrees/wt/HEAD": "ref: refs/heads/x\n",
				"wt/.git":                     "gitdir: {root}/repo/.git/worktrees/wt\n",
			},
			dir:  "wt",
			want: "repo",
		},
		{
			name: "a worktree inside the main repository",
			files: map[string]string{
				"repo/.git/worktrees/wt/commondir": "../..\n",
				"repo/.claude/worktrees/wt/.git":   "gitdir: ../../../.git/worktrees/wt\n",
			},
			dir:  "repo/.claude/worktrees/wt",
			want: "repo",
		},
		{
			name: "a submodule is a repository of its own",
			files: map[string]string{
				"super/.git/modules/lib/HEAD": "ref: refs/heads/main\n",
				"super/lib/.git":              "gitdir: ../.git/modules/lib\n",
				"super/lib/pkg/":              "",
			},
			dir:  "super/lib/pkg",
			want: "super/lib",
		},
		{
			name: "a worktree of a submodule leads back to the submodule",
			files: map[string]string{
				"super/.git/modules/lib/config":                "[core]\n\tbare = false\n\tworktree = ../../../lib\n",
				"super/.git/modules/lib/worktrees/w/commondir": "../..\n",
				"super/lib/.git":                               "gitdir: ../.git/modules/lib\n",
				"libwt/.git":                                   "gitdir: ../super/.git/modules/lib/worktrees/w\n",
			},
			dir:  "libwt",
			want: "super/lib",
		},
		{
			name: "a worktree of a bare repository is named by the bare repository",
			files: map[string]string{
				"bare.git/worktrees/w/commondir": "../..\n",
				"w/.git":                         "gitdir: ../bare.git/worktrees/w\n",
			},
			dir:  "w",
			want: "bare.git",
		},
		{
			name:  "a .git file that says nothing still marks the checkout",
			files: map[string]string{"odd/.git": "garbage\n"},
			dir:   "odd",
			want:  "odd",
		},
		{
			name:  "reached through a symbolic link",
			files: map[string]string{"repo/.git/": "", "repo/a/": ""},
			dir:   "repo/a",
			link:  "elsewhere",
			want:  "repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The temporary directory itself may sit behind a link
			// (/var -> /private/var on macOS), so the expectation is built
			// from its resolved form, which is what Find promises.
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			files := map[string]string{}
			for name, content := range tt.files {
				files[name] = strings.ReplaceAll(content, "{root}", root)
			}
			layout(t, root, files)
			dir := filepath.Join(root, filepath.FromSlash(tt.dir))
			if tt.link != "" {
				link := filepath.Join(root, tt.link)
				if err := os.Symlink(dir, link); err != nil {
					t.Skipf("cannot make a symbolic link here: %v", err)
				}
				dir = link
			}
			want := None
			if tt.want != "" {
				want = filepath.Join(root, filepath.FromSlash(tt.want))
			}
			if got := Find(dir); got != want {
				t.Errorf("Find(%s) = %q, want %q", tt.dir, got, want)
			}
		})
	}
}

// TestFindSameKeyFromEveryWorktree is the promise the server map is built
// on: asked from the main checkout, from a worktree and from deep inside
// either, Find gives the one answer.
func TestFindSameKeyFromEveryWorktree(t *testing.T) {
	root := t.TempDir()
	layout(t, root, map[string]string{
		"repo/.git/worktrees/a/commondir": "../..\n",
		"repo/.git/worktrees/b/commondir": "../..\n",
		"repo/x/":                         "",
		"a/.git":                          "gitdir: ../repo/.git/worktrees/a\n",
		"b/.git":                          "gitdir: ../repo/.git/worktrees/b\n",
		"b/y/z/":                          "",
	})
	want := Find(filepath.Join(root, "repo"))
	if want == None {
		t.Fatal("the main checkout is in no repository")
	}
	for _, dir := range []string{"repo/x", "a", "b/y/z"} {
		if got := Find(filepath.Join(root, dir)); got != want {
			t.Errorf("Find(%s) = %q, want %q", dir, got, want)
		}
	}
}
