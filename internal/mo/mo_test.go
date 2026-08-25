package mo

import (
	"os"
	"path/filepath"
	"testing"
)

// tempFiles returns a directory with the given files created in it. The
// directory itself is resolved, so that a path built from it is already
// canonical and the tests compare what they mean to compare.
func tempFiles(t *testing.T, names ...string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResultURLFor(t *testing.T) {
	dir := tempFiles(t, "a.md", "b.md")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	res := &Result{
		URL: "http://localhost:6275/sbnn-default",
		Files: []File{
			{Path: filepath.Join(dir, "a.md"), Name: "a.md", URL: "http://localhost:6275/sbnn-default?file=a"},
			{Path: filepath.Join(dir, "b.md"), Name: "b.md", URL: "http://localhost:6275/sbnn-default?file=b"},
			// mo lists what it opened, and an entry may arrive with
			// no path at all. It names no file, so nothing may match
			// it - least of all an empty path handed in by a caller
			// that has nothing to look up.
			{Name: "pasted", URL: "http://localhost:6275/sbnn-default?file=pasted"},
		},
	}

	tests := map[string]struct {
		path string
		want string
	}{
		"exactly as mo reported it": {
			filepath.Join(dir, "b.md"),
			"http://localhost:6275/sbnn-default?file=b",
		},
		"needs cleaning": {
			dir + "/sub/../a.md",
			"http://localhost:6275/sbnn-default?file=a",
		},
		"dot element": {
			dir + "/./b.md",
			"http://localhost:6275/sbnn-default?file=b",
		},
		// The group holds more than one file, so there is no single file
		// to guess at; anything else must be reported as a miss.
		"not in the group": {
			filepath.Join(dir, "elsewhere.md"),
			"",
		},
		"empty path": {"", ""},
		// Present in the answer, but with no path of its own: it can
		// only be reached by asking for the file it belongs to, which
		// mo did not say.
		"a file mo listed without a path": {filepath.Join(dir, "pasted"), ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := res.URLFor(tt.path); got != tt.want {
				t.Errorf("URLFor(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// The old code answered a miss with the only file's URL, or with the group
// page. Both frame a page about some other file while looking like a success.
func TestResultURLForDoesNotGuess(t *testing.T) {
	dir := tempFiles(t, "a.md")
	group := "http://localhost:6275/sbnn-default"

	single := &Result{
		URL:   group,
		Files: []File{{Path: filepath.Join(dir, "a.md"), URL: group + "?file=a"}},
	}
	if got := single.URLFor(filepath.Join(dir, "other.md")); got != "" {
		t.Errorf("URLFor of a path mo never mentioned = %q, want %q", got, "")
	}

	none := &Result{URL: group}
	if got := none.URLFor(filepath.Join(dir, "a.md")); got != "" {
		t.Errorf("URLFor against an empty file list = %q, want %q", got, "")
	}
}

func TestResultURLForResolvesSymlinks(t *testing.T) {
	dir := tempFiles(t, "a.md")
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(filepath.Join(dir, "a.md"), link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	url := "http://localhost:6275/sbnn-default?file=a"

	// mo reports the file it actually opened; sbnn asked through the link.
	reportsTarget := &Result{
		URL:   "http://localhost:6275/sbnn-default",
		Files: []File{{Path: filepath.Join(dir, "a.md"), URL: url}},
	}
	if got := reportsTarget.URLFor(link); got != url {
		t.Errorf("URLFor(%q) = %q, want %q", link, got, url)
	}

	// And the other way round.
	reportsLink := &Result{
		URL:   "http://localhost:6275/sbnn-default",
		Files: []File{{Path: link, URL: url}},
	}
	if got := reportsLink.URLFor(filepath.Join(dir, "a.md")); got != url {
		t.Errorf("URLFor of the symlink target = %q, want %q", got, url)
	}
}

func TestResultURLForMatchesARelativePath(t *testing.T) {
	dir := tempFiles(t, "a.md")
	t.Chdir(dir)

	url := "http://localhost:6275/sbnn-default?file=a"
	res := &Result{
		URL:   "http://localhost:6275/sbnn-default",
		Files: []File{{Path: filepath.Join(dir, "a.md"), URL: url}},
	}
	if got := res.URLFor("a.md"); got != url {
		t.Errorf("URLFor(%q) = %q, want %q", "a.md", got, url)
	}
}
