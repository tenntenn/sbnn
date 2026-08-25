package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// previewRel is the path of the file every case of TestPreviewResolve
// changes.
const previewRel = "docs/guide.md"

// appliedFile is a file whose new side is
//
//	alpha / bravo / charlie / delta / new line / echo
//
// and whose single hunk only describes the last three lines, so that
// rebuilding it from the diff can only ever be partial.
func appliedFile() *model.File {
	return &model.File{
		ID:         "f1",
		OldPath:    previewRel,
		NewPath:    previewRel,
		Status:     model.StatusModified,
		IsMarkdown: true,
		Hunks: []*model.Hunk{{
			Header:   "@@ -4,3 +4,3 @@",
			OldStart: 4, OldLines: 3,
			NewStart: 4, NewLines: 3,
			Lines: []model.Line{
				{Kind: model.LineContext, OldNumber: 4, NewNumber: 4, Content: "delta"},
				{Kind: model.LineDelete, OldNumber: 5, Content: "old line"},
				{Kind: model.LineAdd, NewNumber: 5, Content: "new line"},
				{Kind: model.LineContext, OldNumber: 6, NewNumber: 6, Content: "echo"},
			},
		}},
	}
}

const (
	// newSideOnDisk is what the file looks like once the patch is applied.
	newSideOnDisk = "alpha\nbravo\ncharlie\ndelta\nnew line\necho\n"
	// oldSideOnDisk is what it looks like while the patch is still just a
	// patch: the very case "cat change.patch | sbnn" is for.
	oldSideOnDisk = "alpha\nbravo\ncharlie\ndelta\nold line\necho\n"
)

func TestPreviewResolve(t *testing.T) {
	for _, tt := range []struct {
		name string
		file *model.File
		// disk is written to the working tree before resolving; an empty
		// string means the file is not on disk at all.
		disk         string
		wantSource   PreviewSource
		wantComplete bool
	}{
		{
			name:         "applied patch is served from the working tree",
			file:         appliedFile(),
			disk:         newSideOnDisk,
			wantSource:   SourceWorktree,
			wantComplete: true,
		},
		{
			name:       "unapplied patch leaves the old side on disk",
			file:       appliedFile(),
			disk:       oldSideOnDisk,
			wantSource: SourceReconstructed,
			// The hunk starts at line 4, so the rebuilt Markdown is
			// missing the first three lines and says so.
			wantComplete: false,
		},
		{
			name:         "line endings alone do not condemn a working tree file",
			file:         appliedFile(),
			disk:         strings.ReplaceAll(newSideOnDisk, "\n", "\r\n"),
			wantSource:   SourceWorktree,
			wantComplete: true,
		},
		{
			name:         "a file shorter than the hunk claims was not patched",
			file:         appliedFile(),
			disk:         "alpha\nbravo\n",
			wantSource:   SourceReconstructed,
			wantComplete: false,
		},
		{
			name: "a binary file is taken as it is",
			file: func() *model.File {
				f := appliedFile()
				f.IsBinary = true
				return f
			}(),
			disk:         oldSideOnDisk,
			wantSource:   SourceWorktree,
			wantComplete: true,
		},
		{
			name: "a file without hunks is taken as it is",
			file: func() *model.File {
				f := appliedFile()
				f.Hunks = nil
				f.Status = model.StatusMode
				return f
			}(),
			disk:         oldSideOnDisk,
			wantSource:   SourceWorktree,
			wantComplete: true,
		},
		{
			name:         "nothing on disk is rebuilt from the diff",
			file:         appliedFile(),
			disk:         "",
			wantSource:   SourceReconstructed,
			wantComplete: false,
		},
		{
			name: "an added file matching the diff is served from the working tree",
			file: &model.File{
				ID:         "f2",
				OldPath:    model.DevNull,
				NewPath:    previewRel,
				Status:     model.StatusAdded,
				IsMarkdown: true,
				Hunks: []*model.Hunk{{
					Header:   "@@ -0,0 +1,2 @@",
					NewStart: 1, NewLines: 2,
					Lines: []model.Line{
						{Kind: model.LineAdd, NewNumber: 1, Content: "# New"},
						{Kind: model.LineAdd, NewNumber: 2, Content: "body"},
					},
				}},
			},
			disk:         "# New\nbody\n",
			wantSource:   SourceWorktree,
			wantComplete: true,
		},
		{
			name: "an added file the working tree knows nothing about is rebuilt",
			file: &model.File{
				ID:         "f3",
				OldPath:    model.DevNull,
				NewPath:    previewRel,
				Status:     model.StatusAdded,
				IsMarkdown: true,
				Hunks: []*model.Hunk{{
					Header:   "@@ -0,0 +1,2 @@",
					NewStart: 1, NewLines: 2,
					Lines: []model.Line{
						{Kind: model.LineAdd, NewNumber: 1, Content: "# New"},
						{Kind: model.LineAdd, NewNumber: 2, Content: "body"},
					},
				}},
			},
			disk:       "something else entirely\n",
			wantSource: SourceReconstructed,
			// An added file is fully described by its own hunks.
			wantComplete: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			onDisk := filepath.Join(base, filepath.FromSlash(previewRel))
			if tt.disk != "" {
				if err := os.MkdirAll(filepath.Dir(onDisk), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(onDisk, []byte(tt.disk), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			d := &model.Diff{ID: "d1", BaseDir: base, Files: []*model.File{tt.file}}
			p := &previewer{cacheDir: t.TempDir()}

			path, src, complete, err := p.resolve("default", d, tt.file)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if src != tt.wantSource || complete != tt.wantComplete {
				t.Errorf("resolve = %q, complete %v, want %q, complete %v",
					src, complete, tt.wantSource, tt.wantComplete)
			}
			switch tt.wantSource {
			case SourceWorktree:
				if path != onDisk {
					t.Errorf("path = %q, want the working tree file %q", path, onDisk)
				}
			case SourceReconstructed:
				if path == onDisk {
					t.Fatalf("path = %q, want a rebuilt file outside the working tree", path)
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("rebuilt file: %v", err)
				}
				want, _ := diff.Reconstruct(tt.file)
				if string(got) != want {
					t.Errorf("rebuilt content = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestWorktreeMatchesNewSide(t *testing.T) {
	for _, tt := range []struct {
		name string
		disk string
		want bool
	}{
		{"the new side itself", newSideOnDisk, true},
		{"the old side", oldSideOnDisk, false},
		{"empty", "", false},
		{"the new side without a trailing newline", strings.TrimSuffix(newSideOnDisk, "\n"), true},
		{"the new side shifted by a line", "shifted\n" + newSideOnDisk, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := worktreeMatchesNewSide(tt.disk, appliedFile()); got != tt.want {
				t.Errorf("worktreeMatchesNewSide(%q) = %v, want %v", tt.disk, got, tt.want)
			}
		})
	}
}

// combinedDiff is what "git show <merge>" prints for a file that both sides
// of a merge touched. Its "@@@" headers carry two old ranges and a new one,
// and this parser deliberately reads no numbers out of them, because they do
// not describe the two-way view sbnn shows. Every hunk therefore arrives
// with NewStart 0 and every line with no number at all.
const combinedDiff = `diff --cc docs/guide.md
index 1111111,2222222..3333333
--- a/docs/guide.md
+++ b/docs/guide.md
@@@ -1,4 -1,4 +1,5 @@@
  alpha
  bravo
 -charlie
 +charlie merged
  delta
+ echo
`

// TestCombinedDiffKeepsTheWorkingTreeFile is the regression test for a
// combined diff being read as proof that the working tree is wrong.
//
// worktreeMatchesNewSide walks each hunk from Hunk.NewStart. A combined hunk
// has no NewStart, so the walk began at line 0, the first comparison was out
// of range, and the answer was "does not match" - for every combined diff,
// whatever was on disk. A merge shown with "git show <merge>" is exactly the
// case where the working tree does hold the new side, so sbnn threw away the
// right file and served a partial rebuild of it instead.
//
// A check that cannot be run has to say so rather than convict.
func TestCombinedDiffKeepsTheWorkingTreeFile(t *testing.T) {
	files := diff.Parse(combinedDiff)
	if len(files) != 1 {
		t.Fatalf("parsed %d file(s), want 1", len(files))
	}
	f := files[0]
	f.ID, f.IsMarkdown = "f1", true
	if len(f.Hunks) == 0 {
		t.Fatal("the combined diff parsed to no hunks")
	}
	// The premise of the bug, pinned so that the test keeps testing it: a
	// combined hunk carries no new-side numbering.
	for i, h := range f.Hunks {
		if h.NewStart != 0 {
			t.Fatalf("hunk %d has NewStart %d; combined hunks are parsed without one, "+
				"so this test no longer covers what it was written for", i, h.NewStart)
		}
	}

	// The merged file, as it really is on disk after the merge.
	const merged = "alpha\nbravo\ncharlie merged\ndelta\necho\n"

	base := t.TempDir()
	onDisk := filepath.Join(base, filepath.FromSlash(previewRel))
	if err := os.MkdirAll(filepath.Dir(onDisk), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onDisk, []byte(merged), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &model.Diff{ID: "d1", BaseDir: base, Files: []*model.File{f}}
	p := &previewer{cacheDir: t.TempDir()}

	path, src, complete, err := p.resolve("default", d, f)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src != SourceWorktree {
		t.Errorf("resolve served %q, want %q: a combined diff cannot show the working tree is stale",
			src, SourceWorktree)
	}
	if path != onDisk {
		t.Errorf("path = %q, want the working tree file %q", path, onDisk)
	}
	if !complete {
		t.Error("the working tree file was reported partial")
	}

	// And the content handed to the preview is the file on disk, not a
	// rebuild that drops whatever the hunks did not mention.
	got := newSide(d, f)
	if got.Kind != source.FromWorktree {
		t.Errorf("newSide read from %v, want the working tree", got.Kind)
	}
	if got.Content != merged {
		t.Errorf("newSide content = %q, want the merged file %q", got.Content, merged)
	}
}

// A combined diff says nothing about the working tree, so nothing on disk
// can be contradicted by one - but a file that is genuinely absent is still
// rebuilt, and the ordinary two-way check still convicts.
func TestWorktreeMatchesNewSideWithoutNewSideNumbers(t *testing.T) {
	files := diff.Parse(combinedDiff)
	f := files[0]
	for _, tt := range []struct {
		name string
		disk string
	}{
		{"the merged file", "alpha\nbravo\ncharlie merged\ndelta\necho\n"},
		{"one of the parents", "alpha\nbravo\ncharlie\ndelta\n"},
		{"something else entirely", "nothing like it\n"},
		{"empty", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !worktreeMatchesNewSide(tt.disk, f) {
				t.Error("a combined diff was read as proof that the working tree is wrong")
			}
		})
	}

	// The two-way check is untouched: it still rejects the old side.
	if worktreeMatchesNewSide(oldSideOnDisk, appliedFile()) {
		t.Error("an unapplied two-way patch is no longer caught")
	}
}
