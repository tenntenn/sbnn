package export_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/asset"
	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/export"
	"github.com/tenntenn/sbnn/internal/model"
)

// Every image of this review passes the per-file cap and the page is still
// bounded (#356): ten images at MaxBytes were a 28MB page. The budget is spent
// in the order of the diff, and a file past it carries the verdict an
// oversized one does, so the exported page can say it was left out. The
// boundary is pinned the way the per-file cap is: filling the budget exactly is
// carried, one byte more is not.
func TestBuildBoundsTheImagesOfThePage(t *testing.T) {
	const m = asset.MaxBytes
	cases := []struct {
		name  string
		sizes []int
		// wantCarried is, per file in diff order, whether its bytes travel.
		wantCarried []bool
	}{
		{
			"exactly the budget",
			[]int{m, m, m, m},
			[]bool{true, true, true, true},
		},
		{
			"the budget plus one byte",
			[]int{m, m, m, m - 1, 2},
			[]bool{true, true, true, true, false},
		},
		{
			"the budget minus one byte plus one",
			[]int{m, m, m, m - 1, 1},
			[]bool{true, true, true, true, true},
		},
		{
			"ten images that each fit",
			[]int{m, m, m, m, m, m, m, m, m, m},
			[]bool{true, true, true, true, false, false, false, false, false, false},
		},
		{
			// A refused file is not charged, so a later small one still fits.
			"a small one after a refused one",
			[]int{m, m, m, m - 10, m, 10},
			[]bool{true, true, true, true, false, true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			var raw strings.Builder
			for i, n := range c.sizes {
				name := fmt.Sprintf("i%d.png", i)
				writeFile(t, filepath.Join(dir, name), n)
				fmt.Fprintf(&raw, "diff --git a/%[1]s b/%[1]s\nnew file mode 100644\nBinary files /dev/null and b/%[1]s differ\n", name)
			}
			g := &model.Group{
				Name: "default",
				Diffs: []*model.Diff{{
					ID: "d1", Title: "first", BaseDir: dir, Raw: raw.String(), Files: diff.Parse(raw.String()),
				}},
			}
			p := export.Build(g, "test", time.Now())

			files := p.Diffs[0].Files
			if len(files) != len(c.sizes) {
				t.Fatalf("got %d files, want %d", len(files), len(c.sizes))
			}
			var carried int
			for i, f := range files {
				_, has := p.Images["d1:"+f.ID]
				if has != c.wantCarried[i] {
					t.Errorf("file %d (%d bytes): carried = %v, want %v", i, c.sizes[i], has, c.wantCarried[i])
				}
				if has {
					carried += c.sizes[i]
					if f.ImageStatus != string(asset.StatusOK) {
						t.Errorf("file %d is carried but says %q", i, f.ImageStatus)
					}
					continue
				}
				if f.ImageStatus != string(asset.StatusOverBudget) || f.ImageSize != int64(c.sizes[i]) {
					t.Errorf("file %d says %q, %d, want over-budget and its size %d", i, f.ImageStatus, f.ImageSize, c.sizes[i])
				}
			}
			if carried > asset.MaxPageBytes {
				t.Errorf("carried %d bytes, past the %d budget", carried, asset.MaxPageBytes)
			}
		})
	}
}
