package source_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// A diff is text sbnn did not write. A path that climbs out of the directory
// the diff was sent from is refused, because whatever is read here is shown
// in the preview and baked into exported pages.
func TestAbsPathRefusesToLeaveTheBaseDir(t *testing.T) {
	base := t.TempDir()
	inside := filepath.Join(base, "docs", "guide.md")

	cases := []struct {
		name string
		rel  string
		want string
	}{
		{"an ordinary path", "docs/guide.md", inside},
		{"a path that stays inside after a detour", "docs/../docs/guide.md", inside},
		{"a path that climbs out", "../../../../etc/passwd", ""},
		{"a path that climbs out and back into a sibling", "../other/secret.md", ""},
		{"an absolute path elsewhere", "/etc/passwd", ""},
		{"the base itself", ".", base},
		{"nothing", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.rel == "/etc/passwd" {
				t.Skip("absolute paths are spelled differently here")
			}
			if got := source.AbsPath(base, tc.rel); got != tc.want {
				t.Errorf("AbsPath(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}
}

// Without a base directory nothing is read from disk at all.
func TestAbsPathNeedsABaseDir(t *testing.T) {
	if got := source.AbsPath("", "/etc/passwd"); got != "" {
		t.Errorf("AbsPath = %q, want nothing", got)
	}
}

// A refused path is not an error the reader sees: the new side is rebuilt
// from the diff, which is what happens for any file that is not on disk.
func TestNewSideFallsBackWhenThePathEscapes(t *testing.T) {
	base := t.TempDir()
	secret := filepath.Join(filepath.Dir(base), "secret.txt")
	if err := os.WriteFile(secret, []byte("token=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	got := source.NewSide(base, &model.File{NewPath: "../" + filepath.Base(secret)})
	if got.Kind != source.FromDiff {
		t.Errorf("kind = %q, want the diff to be the source", got.Kind)
	}
	if got.Content == "token=hunter2\n" {
		t.Error("the file outside the base directory was read anyway")
	}
}

// A symlink is a way out of the base directory that no amount of lexical
// cleaning catches: the path stays inside, the bytes come from wherever the
// link points. Resolving both sides before the comparison is what closes it,
// and links that stay inside keep working.
func TestAbsPathRefusesASymlinkOutOfTheBaseDir(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()

	mkdir(t, filepath.Join(base, "docs"))
	write(t, filepath.Join(base, "docs", "guide.md"), "inside\n")
	mkdir(t, filepath.Join(outside, "vault"))
	write(t, filepath.Join(outside, "secret.md"), "token=hunter2\n")
	write(t, filepath.Join(outside, "vault", "secret.md"), "token=hunter2\n")

	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(base, "escape.md"))
	symlink(t, filepath.Join(outside, "vault"), filepath.Join(base, "escapedir"))
	symlink(t, filepath.Join(outside, "gone.md"), filepath.Join(base, "dangling.md"))
	symlink(t, filepath.Join(base, "docs", "guide.md"), filepath.Join(base, "here.md"))
	symlink(t, filepath.Join(base, "docs"), filepath.Join(base, "heredir"))

	cases := []struct {
		name string
		rel  string
		want string
	}{
		{"a link to a file outside", "escape.md", ""},
		{"a file under a directory linked outside", "escapedir/secret.md", ""},
		{"a link whose target is not there", "dangling.md", ""},
		{"a link that stays inside", "here.md", filepath.Join(base, "here.md")},
		{"a file under a directory linked inside", "heredir/guide.md", filepath.Join(base, "heredir", "guide.md")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := source.AbsPath(base, tc.rel); got != tc.want {
				t.Errorf("AbsPath(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}
}

// The base directory itself is often reached through a link — /tmp is one on
// macOS — and a file plainly inside it must not be refused just because the
// two spellings differ.
func TestAbsPathAcceptsABaseDirReachedThroughALink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	mkdir(t, filepath.Join(real, "docs"))
	write(t, filepath.Join(real, "docs", "guide.md"), "inside\n")
	base := filepath.Join(root, "link")
	symlink(t, real, base)

	cases := []struct {
		name string
		rel  string
		want string
	}{
		{"a file inside the linked base", "docs/guide.md", filepath.Join(base, "docs", "guide.md")},
		{"a file that climbs out of the linked base", "../real/docs/guide.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := source.AbsPath(base, tc.rel); got != tc.want {
				t.Errorf("AbsPath(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}
}

// A diff names files that are not on disk — anything it adds, anything since
// deleted — and those still have to resolve, because the caller decides what
// to do about the missing file, not this.
func TestAbsPathAllowsAPathThatIsNotThereYet(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	symlink(t, outside, filepath.Join(base, "escapedir"))

	cases := []struct {
		name string
		rel  string
		want string
	}{
		{"a file the diff adds", "new.md", filepath.Join(base, "new.md")},
		{"a file in a directory the diff adds", "docs/new/deep.md", filepath.Join(base, "docs", "new", "deep.md")},
		{"a file added under a directory linked outside", "escapedir/new.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := source.AbsPath(base, tc.rel); got != tc.want {
				t.Errorf("AbsPath(%q) = %q, want %q", tc.rel, got, tc.want)
			}
		})
	}
}

// The refusal has to hold where it matters: the bytes behind a link out of
// the base directory never reach the preview or the exported page.
func TestNewSideFallsBackWhenALinkEscapes(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "id_rsa")
	write(t, secret, "token=hunter2\n")
	symlink(t, secret, filepath.Join(base, "link.md"))

	got := source.NewSide(base, &model.File{NewPath: "link.md"})
	if got.Kind != source.FromDiff {
		t.Errorf("kind = %q, want the diff to be the source", got.Kind)
	}
	if strings.Contains(got.Content, "hunter2") {
		t.Error("the file the link pointed at was read anyway")
	}
	if got.Path != "" {
		t.Errorf("path = %q, want nothing read from disk", got.Path)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// symlink skips the test where symlinks need a privilege the test process
// does not have, which is the common case on Windows.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
}
