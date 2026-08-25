package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
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
