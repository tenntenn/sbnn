package diff_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

func TestParseGitDiff(t *testing.T) {
	src := `diff --git a/main.go b/main.go
index 1234567..89abcde 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@ func main() {
 package main

-import "fmt"
+import (
+	"fmt"
+)

 func main() {
`
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "main.go" || f.NewPath != "main.go" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if f.Status != model.StatusModified {
		t.Errorf("status = %q, want modified", f.Status)
	}
	if f.Additions != 3 || f.Deletions != 1 {
		t.Errorf("additions/deletions = %d/%d, want 3/1", f.Additions, f.Deletions)
	}
	if f.ViewMode != model.ViewSplit {
		t.Errorf("viewMode = %q, want split", f.ViewMode)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 5 || h.NewStart != 1 || h.NewLines != 6 {
		t.Errorf("hunk range = %d,%d %d,%d", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	}
	if h.Section != "func main() {" {
		t.Errorf("section = %q", h.Section)
	}
	// Line numbering: "package main" is old 1 / new 1, the added import block
	// only has new numbers.
	first := h.Lines[0]
	if first.Kind != model.LineContext || first.OldNumber != 1 || first.NewNumber != 1 {
		t.Errorf("first line = %+v", first)
	}
	var adds []model.Line
	for _, l := range h.Lines {
		if l.Kind == model.LineAdd {
			adds = append(adds, l)
		}
	}
	if len(adds) != 3 || adds[0].NewNumber != 3 || adds[0].OldNumber != 0 {
		t.Errorf("added lines = %+v", adds)
	}
}

func TestParseNewFileIsUnified(t *testing.T) {
	src := `diff --git a/docs/spec.md b/docs/spec.md
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/docs/spec.md
@@ -0,0 +1,3 @@
+# Spec
+
+Hello.
`
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	f := files[0]
	if f.Status != model.StatusAdded {
		t.Errorf("status = %q, want added", f.Status)
	}
	if f.ViewMode != model.ViewUnified {
		t.Errorf("viewMode = %q, want unified for a new file", f.ViewMode)
	}
	if f.OldPath != "" || f.NewPath != "docs/spec.md" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if !f.IsMarkdown {
		t.Error("IsMarkdown = false, want true")
	}
	got, complete := diff.Reconstruct(f)
	if !complete {
		t.Error("Reconstruct: complete = false, want true for an added file")
	}
	if got != "# Spec\n\nHello.\n" {
		t.Errorf("Reconstruct = %q", got)
	}
}

func TestParseDeletedFile(t *testing.T) {
	src := `diff --git a/old.txt b/old.txt
deleted file mode 100644
index e69de29..0000000
--- a/old.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-line1
-line2
`
	files := diff.Parse(src)
	f := files[0]
	if f.Status != model.StatusDeleted {
		t.Errorf("status = %q, want deleted", f.Status)
	}
	if f.NewPath != "" || f.OldPath != "old.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if f.ViewMode != model.ViewUnified {
		t.Errorf("viewMode = %q, want unified", f.ViewMode)
	}
	if f.Path() != "old.txt" {
		t.Errorf("Path() = %q", f.Path())
	}
}

func TestParseRename(t *testing.T) {
	src := `diff --git a/a.txt b/b.txt
similarity index 87%
rename from a.txt
rename to b.txt
index 1234567..89abcde 100644
--- a/a.txt
+++ b/b.txt
@@ -1,2 +1,2 @@
 keep
-old
+new
`
	f := diff.Parse(src)[0]
	if f.Status != model.StatusRenamed {
		t.Errorf("status = %q, want renamed", f.Status)
	}
	if f.OldPath != "a.txt" || f.NewPath != "b.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
}

func TestParseBinaryAndMode(t *testing.T) {
	src := `diff --git a/logo.png b/logo.png
index 1234567..89abcde 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/run.sh b/run.sh
old mode 100644
new mode 100755
`
	files := diff.Parse(src)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if !files[0].IsBinary {
		t.Error("logo.png: IsBinary = false, want true")
	}
	if files[0].ViewMode != model.ViewUnified {
		t.Errorf("logo.png: viewMode = %q, want unified", files[0].ViewMode)
	}
	if files[1].Status != model.StatusMode {
		t.Errorf("run.sh: status = %q, want mode", files[1].Status)
	}
	if files[1].OldMode != "100644" || files[1].NewMode != "100755" {
		t.Errorf("run.sh: modes = %q, %q", files[1].OldMode, files[1].NewMode)
	}
}

func TestParsePlainDiff(t *testing.T) {
	// "diff -u old new" output: no git header, timestamps after a tab.
	src := "--- old.txt\t2026-08-15 10:00:00.000000000 +0900\n" +
		"+++ new.txt\t2026-08-15 10:00:01.000000000 +0900\n" +
		"@@ -1,3 +1,3 @@\n" +
		" a\n" +
		"-b\n" +
		"+B\n" +
		" c\n"
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "old.txt" || f.NewPath != "new.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if f.Additions != 1 || f.Deletions != 1 {
		t.Errorf("additions/deletions = %d/%d", f.Additions, f.Deletions)
	}
}

func TestParseMultipleFiles(t *testing.T) {
	src := `--- a/one.txt
+++ b/one.txt
@@ -1 +1 @@
-1
+one
--- a/two.txt
+++ b/two.txt
@@ -1 +1 @@
-2
+two
`
	files := diff.Parse(src)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].NewPath != "one.txt" || files[1].NewPath != "two.txt" {
		t.Errorf("paths = %q, %q", files[0].NewPath, files[1].NewPath)
	}
	if files[0].ID == files[1].ID {
		t.Error("file IDs collide")
	}
}

func TestParseNoNewlineAtEOF(t *testing.T) {
	src := `--- a/f.txt
+++ b/f.txt
@@ -1 +1 @@
-a
\ No newline at end of file
+b
\ No newline at end of file
`
	f := diff.Parse(src)[0]
	lines := f.Hunks[0].Lines
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !lines[0].NoNewline || !lines[1].NoNewline {
		t.Errorf("NoNewline flags = %v, %v", lines[0].NoNewline, lines[1].NoNewline)
	}
}

func TestParsePathWithSpaces(t *testing.T) {
	src := `diff --git a/my dir/my file.md b/my dir/my file.md
index 1234567..89abcde 100644
--- a/my dir/my file.md
+++ b/my dir/my file.md
@@ -1 +1 @@
-x
+y
`
	f := diff.Parse(src)[0]
	if f.NewPath != "my dir/my file.md" {
		t.Errorf("newPath = %q", f.NewPath)
	}
}

func TestParseQuotedPath(t *testing.T) {
	src := "diff --git \"a/\\346\\227\\245\\346\\234\\254\\350\\252\\236.md\" \"b/\\346\\227\\245\\346\\234\\254\\350\\252\\236.md\"\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ \"b/\\346\\227\\245\\346\\234\\254\\350\\252\\236.md\"\n" +
		"@@ -0,0 +1 @@\n" +
		"+hi\n"
	f := diff.Parse(src)[0]
	if f.NewPath != "日本語.md" {
		t.Errorf("newPath = %q, want 日本語.md", f.NewPath)
	}
}

func TestReconstructPartial(t *testing.T) {
	src := `--- a/doc.md
+++ b/doc.md
@@ -10,3 +10,3 @@
 intro
-old
+new
`
	f := diff.Parse(src)[0]
	got, complete := diff.Reconstruct(f)
	if complete {
		t.Error("complete = true, want false for a partial hunk")
	}
	if !strings.Contains(got, "sbnn: 9 line(s) not included") {
		t.Errorf("missing gap marker: %q", got)
	}
	if !strings.Contains(got, "new") || strings.Contains(got, "old") {
		t.Errorf("reconstructed content = %q", got)
	}
}

func TestIsMarkdown(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"README.md", true},
		{"docs/SPEC.MD", true},
		{"a.mdx", true},
		{"a.markdown", true},
		{"main.go", false},
		{"noext", false},
	} {
		if got := diff.IsMarkdown(tt.path); got != tt.want {
			t.Errorf("IsMarkdown(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsImage(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"logo.png", true},
		{"photo.JPG", true},
		{"photo.jpeg", true},
		{"icon.svg", true},
		{"anim.gif", true},
		{"pic.webp", true},
		{"pic.avif", true},
		{"favicon.ico", true},
		{"scan.bmp", true},
		{"README.md", false},
		{"main.go", false},
		{"noext", false},
	}
	for _, c := range cases {
		if got := diff.IsImage(c.path); got != c.want {
			t.Errorf("IsImage(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestImageContentType(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"logo.png", "image/png"},
		{"photo.JPEG", "image/jpeg"},
		{"icon.svg", "image/svg+xml"},
		{"main.go", ""},
	}
	for _, c := range cases {
		if got := diff.ImageContentType(c.path); got != c.want {
			t.Errorf("ImageContentType(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestIsNotebook(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"analysis.ipynb", true},
		{"notebooks/EDA.IPYNB", true},
		{"README.md", false},
		{"noext", false},
	}
	for _, c := range cases {
		if got := diff.IsNotebook(c.path); got != c.want {
			t.Errorf("IsNotebook(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	if files := diff.Parse(""); len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

// A rename with edits is one file, not two: the rename headers name it and
// the "--- / +++" pair that follows belongs to the same entry.
func TestParseRenameWithEditsIsOneFile(t *testing.T) {
	src := `diff --git a/old.txt b/new.txt
similarity index 80%
rename from old.txt
rename to new.txt
index 1234567..89abcde 100644
--- a/old.txt
+++ b/new.txt
@@ -1,2 +1,2 @@
 keep
-was
+now
`
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
	}
	f := files[0]
	if f.Status != model.StatusRenamed || f.OldPath != "old.txt" || f.NewPath != "new.txt" {
		t.Errorf("file = %s %q -> %q", f.Status, f.OldPath, f.NewPath)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("the edits went missing: %d hunk(s)", len(f.Hunks))
	}
	if f.Additions != 1 || f.Deletions != 1 {
		t.Errorf("stats = +%d -%d", f.Additions, f.Deletions)
	}
}

// A copy with edits has the same shape as a rename.
func TestParseCopyWithEditsIsOneFile(t *testing.T) {
	src := `diff --git a/src.txt b/copy.txt
similarity index 90%
copy from src.txt
copy to copy.txt
--- a/src.txt
+++ b/copy.txt
@@ -1 +1 @@
-old
+new
`
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
	}
	if files[0].Status != model.StatusCopied || files[0].NewPath != "copy.txt" || len(files[0].Hunks) != 1 {
		t.Errorf("file = %+v", files[0])
	}
}

// A plain diff of several files still splits on every "--- / +++" pair,
// which is the only thing separating them.
func TestParsePlainDiffStillSplitsOnHeaders(t *testing.T) {
	src := `--- a.txt	2026-08-16 10:00:00
+++ a.txt	2026-08-16 10:01:00
@@ -1 +1 @@
-a
+A
--- b.txt	2026-08-16 10:00:00
+++ b.txt	2026-08-16 10:01:00
@@ -1 +1 @@
-b
+B
`
	files := diff.Parse(src)
	if len(files) != 2 {
		t.Fatalf("got %d file(s), want 2:\n%s", len(files), describe(files))
	}
	if files[0].NewPath != "a.txt" || files[1].NewPath != "b.txt" {
		t.Errorf("paths = %q, %q", files[0].NewPath, files[1].NewPath)
	}
}

// "diff --combined" is longer than "diff --cc"; both name the file.
func TestParseCombinedHeaderSpellings(t *testing.T) {
	for _, header := range []string{"diff --cc merged.txt", "diff --combined merged.txt"} {
		files := diff.Parse(header + `
@@@ -1,1 -1,1 +1,1 @@@
++merged
`)
		if len(files) != 1 || files[0].NewPath != "merged.txt" {
			t.Errorf("%q gave %s", header, describe(files))
		}
	}
}

func describe(files []*model.File) string {
	var b strings.Builder
	for i, f := range files {
		fmt.Fprintf(&b, "  [%d] %s %q -> %q, %d hunk(s)\n", i, f.Status, f.OldPath, f.NewPath, len(f.Hunks))
	}
	return b.String()
}

// A combined diff carries one marker column per parent, so " +y" - a space
// then a plus - is a line added relative to the second parent, not a context
// line whose content starts with a plus.
func TestParseCombinedMarkerColumns(t *testing.T) {
	type line struct {
		kind    model.LineKind
		content string
	}
	tests := []struct {
		name      string
		src       string
		want      []line
		additions int
		deletions int
	}{
		{
			name: "two parents",
			src: "diff --cc a.txt\n--- a/a.txt\n+++ b/a.txt\n" +
				"@@@ -1,2 -1,2 +1,2 @@@\n  ctx\n- x\n +y\n",
			want: []line{
				{model.LineContext, "ctx"},
				{model.LineDelete, "x"},
				{model.LineAdd, "y"},
			},
			additions: 1,
			deletions: 1,
		},
		{
			name: "changed against both parents",
			src: "diff --cc a.txt\n@@@ -1,2 -1,2 +1,2 @@@\n" +
				"--gone\n++new\n  ctx\n",
			want: []line{
				{model.LineDelete, "gone"},
				{model.LineAdd, "new"},
				{model.LineContext, "ctx"},
			},
			additions: 1,
			deletions: 1,
		},
		{
			name: "three parents carry three columns",
			src: "diff --cc a.txt\n@@@@ -1,1 -1,1 -1,1 +1,1 @@@@\n" +
				"   ctx\n  +third\n---gone\n",
			want: []line{
				{model.LineContext, "ctx"},
				{model.LineAdd, "third"},
				{model.LineDelete, "gone"},
			},
			additions: 1,
			deletions: 1,
		},
		{
			name: "content keeps its own leading markers",
			src:  "diff --cc a.txt\n@@@ -1,1 -1,1 +1,1 @@@\n  - not a marker\n +  indented\n",
			want: []line{
				{model.LineContext, "- not a marker"},
				{model.LineAdd, "  indented"},
			},
			additions: 1,
			deletions: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := diff.Parse(tt.src)
			if len(files) != 1 || len(files[0].Hunks) != 1 {
				t.Fatalf("want 1 file with 1 hunk, got:\n%s", describe(files))
			}
			f := files[0]
			var got []line
			for _, l := range f.Hunks[0].Lines {
				got = append(got, line{l.Kind, l.Content})
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("lines = %v, want %v", got, tt.want)
			}
			if f.Additions != tt.additions || f.Deletions != tt.deletions {
				t.Errorf("got +%d -%d, want +%d -%d",
					f.Additions, f.Deletions, tt.additions, tt.deletions)
			}
		})
	}
}

// Text that is not a body line at all must keep its characters rather than
// lose the first ones to a marker column that is not there.
func TestParseCombinedKeepsNonBodyText(t *testing.T) {
	files := diff.Parse("diff --cc a.txt\n@@@ -1,1 -1,1 +1,1 @@@\n  ctx\n2 files changed\n")
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("want 1 file with 1 hunk, got:\n%s", describe(files))
	}
	lines := files[0].Hunks[0].Lines
	if len(lines) != 2 || lines[1].Content != "2 files changed" || lines[1].Kind != model.LineContext {
		t.Errorf("last line = %+v, want context %q", lines[len(lines)-1], "2 files changed")
	}
}

// A hunk header whose numbers carry a sign of their own is malformed. Accepting
// it produced hunks numbered from a negative line, which Line.OldNumber cannot
// express and the UI renders as a blank, uncommentable gutter.
func TestParseRejectsSignedHunkHeaderNumbers(t *testing.T) {
	const headers = `--- a/a.txt
+++ b/a.txt
`
	bad := []string{
		"@@ --1,2 +1,2 @@", // negative old start
		"@@ -1,2 +-1,2 @@", // negative new start
		"@@ -1,-5 +1,1 @@", // negative old count
		"@@ -1,1 +1,-5 @@", // negative new count
		"@@ -+1,2 +1,2 @@", // explicit plus on the old start
		"@@ -1,+2 +1,2 @@", // explicit plus on the old count
		"@@ - ,2 +1,2 @@",  // not a number at all
		"@@ -1,2 +1, @@",   // empty count
	}
	for _, header := range bad {
		t.Run(header, func(t *testing.T) {
			files := diff.Parse(headers + header + "\n-a\n+b\n")
			if len(files) != 1 {
				t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
			}
			if n := len(files[0].Hunks); n != 0 {
				t.Errorf("header %q was accepted: %d hunk(s), OldStart=%d OldLines=%d NewStart=%d NewLines=%d",
					header, n, files[0].Hunks[0].OldStart, files[0].Hunks[0].OldLines,
					files[0].Hunks[0].NewStart, files[0].Hunks[0].NewLines)
			}
		})
	}
}

// A start of 0 is how git writes the missing side of an added or deleted file,
// so it must keep parsing.
func TestParseAcceptsZeroAndPlainHunkHeaderNumbers(t *testing.T) {
	tests := []struct {
		header                                 string
		body                                   string
		oldStart, oldLines, newStart, newLines int
	}{
		{"@@ -0,0 +1,2 @@", "+a\n+b\n", 0, 0, 1, 2},
		{"@@ -1,2 +0,0 @@", "-a\n-b\n", 1, 2, 0, 0},
		{"@@ -1 +1 @@", "-a\n+b\n", 1, 1, 1, 1},
		{"@@ -12,3 +14,4 @@", " a\n-b\n+c\n+d\n", 12, 3, 14, 4},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			files := diff.Parse("--- a/a.txt\n+++ b/a.txt\n" + tt.header + "\n" + tt.body)
			if len(files) != 1 || len(files[0].Hunks) != 1 {
				t.Fatalf("want 1 file with 1 hunk, got:\n%s", describe(files))
			}
			h := files[0].Hunks[0]
			if h.OldStart != tt.oldStart || h.OldLines != tt.oldLines ||
				h.NewStart != tt.newStart || h.NewLines != tt.newLines {
				t.Errorf("got -%d,%d +%d,%d, want -%d,%d +%d,%d",
					h.OldStart, h.OldLines, h.NewStart, h.NewLines,
					tt.oldStart, tt.oldLines, tt.newStart, tt.newLines)
			}
			for _, l := range h.Lines {
				if l.OldNumber < 0 || l.NewNumber < 0 {
					t.Errorf("line %+v has a negative number", l)
				}
			}
		})
	}
}

// A diff that never names its file must not produce a file whose path is the
// empty string: the reviewer would see a nameless row, and any lookup handed
// an empty path would match it.
func TestParseUnnamedFileGetsPlaceholderPath(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		status  model.FileStatus
		oldPath string
		newPath string
	}{
		{
			name:    "bare hunk",
			src:     "@@ -1,2 +1,2 @@\n-a\n+b\n c\n",
			status:  model.StatusModified,
			newPath: diff.UnnamedPath,
		},
		{
			name:    "git header without paths",
			src:     "diff --git\n@@ -1,1 +1,1 @@\n-a\n+b\n",
			status:  model.StatusModified,
			newPath: diff.UnnamedPath,
		},
		{
			name:    "both sides are /dev/null",
			src:     "--- /dev/null\n+++ /dev/null\n@@ -0,0 +1,1 @@\n+a\n",
			status:  model.StatusAdded,
			newPath: diff.UnnamedPath,
		},
		{
			name:    "deletion without paths keeps the name on the old side",
			src:     "diff --git \ndeleted file mode 100644\n@@ -1,1 +0,0 @@\n-a\n",
			status:  model.StatusDeleted,
			oldPath: diff.UnnamedPath,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := diff.Parse(tt.src)
			if len(files) != 1 {
				t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
			}
			f := files[0]
			if f.Path() == "" {
				t.Errorf("Path() is empty:\n%s", describe(files))
			}
			if f.Status != tt.status || f.OldPath != tt.oldPath || f.NewPath != tt.newPath {
				t.Errorf("got %s %q -> %q, want %s %q -> %q",
					f.Status, f.OldPath, f.NewPath, tt.status, tt.oldPath, tt.newPath)
			}
		})
	}
}

// The placeholder is only for entries the diff never named; a real path is
// never replaced, and a one-sided path stays one-sided.
func TestParseNamedFilesKeepTheirPaths(t *testing.T) {
	src := `diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1,1 +0,0 @@
-a
`
	files := diff.Parse(src)
	if len(files) != 1 {
		t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
	}
	if f := files[0]; f.OldPath != "gone.txt" || f.NewPath != "" {
		t.Errorf("got %q -> %q, want %q -> %q", f.OldPath, f.NewPath, "gone.txt", "")
	}
}

// The counts a combined diff reports are read off the marker columns, and
// there is one column per parent. Getting them wrong is not only a wrong pair
// of numbers on the file header: the same Additions and Deletions go to the
// sidebar, to the "%d file(s), +%d -%d" line sbnn prints, and to the record
// the review history keeps, so a merge commit gets reviewed and then filed
// under stats that never matched the screen. They also decide the view mode,
// which is how a merge that only adds lines ends up in the split view.
//
// The counting is fixed here on its own so that a change to how a combined
// hunk body is read has to keep them right.
func TestParseCombinedCounts(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		additions int
		deletions int
	}{
		{
			// The hunk from the issue: one line replaced, one parent
			// each side of it.
			name: "one line replaced",
			src: "diff --cc a.txt\n--- a/a.txt\n+++ b/a.txt\n" +
				"@@@ -1,2 -1,2 +1,2 @@@\n  ctx\n- x\n +y\n",
			additions: 1,
			deletions: 1,
		},
		{
			name: "added against the second parent only",
			src: "diff --cc a.txt\n@@@ -1,1 -1,3 +1,3 @@@\n" +
				"  ctx\n +one\n +two\n",
			additions: 2,
			deletions: 0,
		},
		{
			name: "removed against the first parent only",
			src: "diff --cc a.txt\n@@@ -1,3 -1,1 +1,1 @@@\n" +
				"  ctx\n- one\n- two\n",
			additions: 0,
			deletions: 2,
		},
		{
			name: "changed against both parents counts once a side",
			src: "diff --cc a.txt\n@@@ -1,2 -1,2 +1,2 @@@\n" +
				"--gone\n++new\n  ctx\n",
			additions: 1,
			deletions: 1,
		},
		{
			name: "three parents, marker in the last column",
			src: "diff --cc a.txt\n@@@@ -1,1 -1,1 -1,1 +1,1 @@@@\n" +
				"   ctx\n  +third\n---gone\n",
			additions: 1,
			deletions: 1,
		},
		{
			name: "context is never counted",
			src: "diff --cc a.txt\n@@@ -1,3 -1,3 +1,3 @@@\n" +
				"  ctx\n  ctx\n  ctx\n",
			additions: 0,
			deletions: 0,
		},
		{
			// Content that begins with a marker character sits past
			// the columns and is not a marker.
			name: "content beginning with a marker is not counted",
			src: "diff --cc a.txt\n@@@ -1,2 -1,2 +1,2 @@@\n" +
				"  - not a marker\n  + not a marker\n",
			additions: 0,
			deletions: 0,
		},
		{
			name: "several hunks add up",
			src: "diff --cc a.txt\n@@@ -1,2 -1,2 +1,2 @@@\n  ctx\n +a\n" +
				"@@@ -9,2 -9,2 +9,2 @@@\n  ctx\n- b\n +c\n",
			additions: 2,
			deletions: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := diff.Parse(tt.src)
			if len(files) != 1 {
				t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
			}
			f := files[0]
			if f.Additions != tt.additions || f.Deletions != tt.deletions {
				t.Errorf("got +%d -%d, want +%d -%d", f.Additions, f.Deletions, tt.additions, tt.deletions)
			}
		})
	}
}

// Each file of a combined diff is counted on its own, and the total is what
// sbnn prints as its summary.
func TestParseCombinedCountsPerFile(t *testing.T) {
	src := "diff --cc a.txt\n@@@ -1,2 -1,2 +1,2 @@@\n  ctx\n +added\n" +
		"diff --cc b.txt\n@@@ -1,2 -1,2 +1,2 @@@\n  ctx\n- removed\n"
	files := diff.Parse(src)
	if len(files) != 2 {
		t.Fatalf("got %d file(s), want 2:\n%s", len(files), describe(files))
	}
	want := []struct{ add, del int }{{1, 0}, {0, 1}}
	var totalAdd, totalDel int
	for i, f := range files {
		if f.Additions != want[i].add || f.Deletions != want[i].del {
			t.Errorf("%s: got +%d -%d, want +%d -%d", f.NewPath, f.Additions, f.Deletions, want[i].add, want[i].del)
		}
		totalAdd += f.Additions
		totalDel += f.Deletions
	}
	if totalAdd != 1 || totalDel != 1 {
		t.Errorf("summary = +%d -%d, want +1 -1", totalAdd, totalDel)
	}
}

// An addition-only merge stays in the unified view. It is the deletion count
// that decides, so a miscounted marker column silently moves the whole file
// into the split view.
func TestParseCombinedAdditionsOnlyStaysUnified(t *testing.T) {
	files := diff.Parse("diff --cc a.txt\n@@@ -1,1 -1,2 +1,2 @@@\n  ctx\n +added\n")
	if len(files) != 1 {
		t.Fatalf("got %d file(s), want 1:\n%s", len(files), describe(files))
	}
	f := files[0]
	if f.Additions != 1 || f.Deletions != 0 {
		t.Fatalf("got +%d -%d, want +1 -0", f.Additions, f.Deletions)
	}
	if f.ViewMode != model.ViewUnified {
		t.Errorf("ViewMode = %q, want %q", f.ViewMode, model.ViewUnified)
	}
}
