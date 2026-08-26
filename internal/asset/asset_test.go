package asset_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/asset"
	"github.com/tenntenn/sbnn/internal/model"
)

// write puts a file of n bytes in the tree, creating the directories it needs.
func write(t *testing.T, dir, rel string, n int) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func statusOf(refs []asset.Ref, src string) (asset.Ref, bool) {
	for _, r := range refs {
		if r.Src == src {
			return r, true
		}
	}
	return asset.Ref{}, false
}

func TestRefsFindsTheImagesADocumentPointsAt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "docs/diagram.png", 10)
	write(t, dir, "docs/sub/inline.png", 10)
	write(t, dir, "logo.png", 10)
	write(t, dir, "docs/spaced name.png", 10)
	write(t, dir, "docs/ref.png", 10)
	write(t, dir, "docs/raw.png", 10)

	md := strings.Join([]string{
		"# Doc",
		"",
		"![a](diagram.png)",
		"![b](./sub/inline.png)",
		"![c](../logo.png)",
		"![d](<spaced name.png>)",
		`<img src="raw.png" alt="raw">`,
		"![e][r]",
		"",
		"[r]: ref.png",
	}, "\n")

	refs := asset.Refs(dir, "docs/doc.md", md)
	want := map[string]string{
		"diagram.png":      "docs/diagram.png",
		"./sub/inline.png": "docs/sub/inline.png",
		"../logo.png":      "logo.png",
		"spaced name.png":  "docs/spaced name.png",
		"raw.png":          "docs/raw.png",
		"ref.png":          "docs/ref.png",
	}
	if len(refs) != len(want) {
		t.Fatalf("found %d references, want %d: %+v", len(refs), len(want), refs)
	}
	for src, rel := range want {
		r, ok := statusOf(refs, src)
		if !ok {
			t.Errorf("no reference for %q", src)
			continue
		}
		if r.Rel != rel {
			t.Errorf("%q resolved to %q, want %q", src, r.Rel, rel)
		}
		if r.Status != asset.StatusOK {
			t.Errorf("%q status = %q, want ok", src, r.Status)
		}
	}
}

func TestRefsRefusesPathsOutsideTheDiffDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "outside.png")
	if err := os.WriteFile(outside, make([]byte, 10), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	// A path that stays inside lexically and leaves the tree through a
	// symlink is the case source.AbsPath resolves both sides to catch.
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "docs", "link.png")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	md := strings.Join([]string{
		"![a](../../etc/passwd)",
		"![b](/etc/hosts)",
		"![c](%2E%2E/%2E%2E/outside.png)",
		"![d](link.png)",
		"![e](../../outside.png)",
		"![f](sub/../../../outside.png)",
	}, "\n\n")

	refs := asset.Refs(dir, "docs/doc.md", md)
	if len(refs) != 6 {
		t.Fatalf("found %d references, want 6: %+v", len(refs), refs)
	}
	for _, r := range refs {
		if r.Status != asset.StatusOutside {
			t.Errorf("%q status = %q, want outside", r.Src, r.Status)
		}
		if r.Path != "" {
			t.Errorf("%q resolved to %q on disk, which must not happen", r.Src, r.Path)
		}
	}
}

func TestRefsPlacesTheOversizedImageOutOfLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "small.png", 1024)
	write(t, dir, "big.png", asset.MaxBytes+1)

	refs := asset.Refs(dir, "doc.md", "![s](small.png)\n\n![b](big.png)")
	small, _ := statusOf(refs, "small.png")
	big, _ := statusOf(refs, "big.png")
	if small.Status != asset.StatusOK {
		t.Errorf("small.png status = %q, want ok", small.Status)
	}
	if big.Status != asset.StatusTooLarge {
		t.Errorf("big.png status = %q, want too-large", big.Status)
	}
	if big.Size != asset.MaxBytes+1 {
		t.Errorf("big.png size = %d, want %d", big.Size, asset.MaxBytes+1)
	}
	if big.Label() != "big.png" {
		t.Errorf("big.png label = %q, want the path to stay readable", big.Label())
	}
}

func TestRefsStopsAtTheDocumentBudget(t *testing.T) {
	dir := t.TempDir()
	const each = asset.MaxBytes
	var lines []string
	for i := range 6 {
		name := string(rune('a'+i)) + ".png"
		write(t, dir, name, each)
		lines = append(lines, "!["+name+"]("+name+")")
	}
	refs := asset.Refs(dir, "doc.md", strings.Join(lines, "\n\n"))

	var carried int64
	for _, r := range refs {
		if r.Status == asset.StatusOK {
			carried += r.Size
		}
	}
	if carried > asset.MaxTotalBytes {
		t.Errorf("carried %d bytes, past the %d budget", carried, asset.MaxTotalBytes)
	}
	if last := refs[len(refs)-1]; last.Status != asset.StatusOverBudget {
		t.Errorf("last reference status = %q, want over-budget", last.Status)
	}
}

func TestRefsSkipsWhatItHasNothingToSayAbout(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "there.png", 10)
	md := strings.Join([]string{
		"![remote](https://example.com/x.png)",
		"![protocol relative](//example.com/x.png)",
		"![already data](data:image/png;base64,AAAA)",
		"![fenced](there.png) is drawn, but",
		"```",
		"![in code](there.png)",
		"```",
		"![gone](missing.png)",
		"![not an image](notes.txt)",
	}, "\n\n")

	refs := asset.Refs(dir, "doc.md", md)
	for _, unwanted := range []string{
		"https://example.com/x.png", "//example.com/x.png", "data:image/png;base64,AAAA",
	} {
		if _, ok := statusOf(refs, unwanted); ok {
			t.Errorf("%q should be left to the browser, not resolved here", unwanted)
		}
	}
	if r, _ := statusOf(refs, "missing.png"); r.Status != asset.StatusMissing {
		t.Errorf("missing.png status = %q, want missing", r.Status)
	}
	if r, _ := statusOf(refs, "notes.txt"); r.Status != asset.StatusUnsupported {
		t.Errorf("notes.txt status = %q, want unsupported", r.Status)
	}
	// The fenced reference is the same source as the drawn one, so the
	// count is what says the fence was skipped.
	if n := len(refs); n != 3 {
		t.Errorf("found %d references, want 3: %+v", n, refs)
	}
}

// An image that is part of the diff itself had no cap at all and never did:
// internal/export froze every one of them into the page unconditionally, so
// one 32MiB PNG made a 45MB page out of something meant to be mailed around
// (#323). The rule is the one a sibling image has had since #305, applied to
// the same kind of thing - and it has to be reached by the live page as well
// as by the exporter, which is why it lives here rather than in either.
func TestInDiffCapsAnImageOfTheDiffItself(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "small.png", 1024)
	write(t, dir, "big.png", asset.MaxBytes+1)
	write(t, dir, "exactly.png", asset.MaxBytes)
	write(t, dir, "notes.md", 10)

	cases := []struct {
		name       string
		file       *model.File
		wantStatus asset.Status
		wantSize   int64
	}{
		{"a picture that fits", imageFile("small.png"), asset.StatusOK, 1024},
		{"one byte past the cap", imageFile("big.png"), asset.StatusTooLarge, asset.MaxBytes + 1},
		// The cap is what is drawn up to, not the first size refused.
		{"exactly the cap", imageFile("exactly.png"), asset.StatusOK, asset.MaxBytes},
		{"not in the working tree", imageFile("gone.png"), asset.StatusMissing, 0},
		{"outside the directory the diff came from", imageFile("../elsewhere.png"), asset.StatusOutside, 0},
		{"not an image at all", &model.File{NewPath: "notes.md"}, "", 0},
		{
			"an image that was deleted, which has no picture to weigh",
			&model.File{NewPath: model.DevNull, OldPath: "small.png", IsImage: true, Status: model.StatusDeleted},
			"", 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, size := asset.InDiff(dir, c.file)
			if status != c.wantStatus || size != c.wantSize {
				t.Errorf("InDiff(%s) = %q, %d, want %q, %d", c.file.Path(), status, size, c.wantStatus, c.wantSize)
			}
		})
	}
}

// The same cap the sibling images are held to. Two numbers that drifted apart
// would be two answers to one question.
func TestInDiffUsesTheSameCapAsASiblingImage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.png", asset.MaxBytes+1)
	refs := asset.Refs(dir, "doc.md", "![x](big.png)")
	if len(refs) != 1 || refs[0].Status != asset.StatusTooLarge {
		t.Fatalf("a sibling image past MaxBytes is %+v, want too-large", refs)
	}
	if status, _ := asset.InDiff(dir, imageFile("big.png")); status != refs[0].Status {
		t.Errorf("the same file is %q as an image of the diff and %q as a sibling of a document", status, refs[0].Status)
	}
}

// RecordInDiff is how the verdict reaches the page: it travels on the file,
// so the page knows before it asks for any bytes.
func TestRecordInDiffWritesTheVerdictOntoEveryFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.png", asset.MaxBytes+1)
	write(t, dir, "small.png", 7)
	d := &model.Diff{BaseDir: dir, Files: []*model.File{
		imageFile("big.png"), imageFile("small.png"), {NewPath: "notes.md"},
	}}
	asset.RecordInDiff(d)
	if got := d.Files[0]; got.ImageStatus != string(asset.StatusTooLarge) || got.ImageSize != asset.MaxBytes+1 {
		t.Errorf("big.png = %q, %d, want too-large and its size", got.ImageStatus, got.ImageSize)
	}
	if got := d.Files[1]; got.ImageStatus != string(asset.StatusOK) || got.ImageSize != 7 {
		t.Errorf("small.png = %q, %d, want ok and its size", got.ImageStatus, got.ImageSize)
	}
	if got := d.Files[2]; got.ImageStatus != "" {
		t.Errorf("notes.md = %q, want no status: it is not an image", got.ImageStatus)
	}
}

func imageFile(path string) *model.File {
	return &model.File{NewPath: path, IsImage: true, IsBinary: true, Status: model.StatusAdded}
}
