package export_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/export"
	"github.com/tenntenn/sbnn/internal/model"
)

const sampleDiff = `diff --git a/docs/new.md b/docs/new.md
new file mode 100644
--- /dev/null
+++ b/docs/new.md
@@ -0,0 +1,2 @@
+# New
+body
diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var x = 1
+var x = 2
`

func group(t *testing.T, baseDir string) *model.Group {
	t.Helper()
	files := diff.Parse(sampleDiff)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	return &model.Group{
		Name: "default",
		Diffs: []*model.Diff{{
			ID:      "d1",
			Title:   "first",
			BaseDir: baseDir,
			Raw:     sampleDiff,
			Files:   files,
		}},
	}
}

func TestBuildFreezesMarkdown(t *testing.T) {
	p := export.Build(group(t, ""), "test", time.Now())

	if len(p.Diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(p.Diffs))
	}
	if p.Diffs[0].Raw != "" {
		t.Error("the raw diff should be dropped from the page")
	}
	if len(p.Previews) != 1 {
		t.Fatalf("got %d previews, want 1 (only Markdown files)", len(p.Previews))
	}
	md := p.Diffs[0].Files[0]
	prev, ok := p.Previews["d1:"+md.ID]
	if !ok {
		t.Fatalf("no preview for %s", md.Path())
	}
	if prev.Content != "# New\nbody\n" || !prev.Complete {
		t.Errorf("preview = %+v", prev)
	}
	if prev.Source != string("reconstructed") {
		t.Errorf("source = %q, want reconstructed for a file that is not on disk", prev.Source)
	}
}

const notebookAndImageDiff = `diff --git a/analysis.ipynb b/analysis.ipynb
new file mode 100644
--- /dev/null
+++ b/analysis.ipynb
@@ -0,0 +1,1 @@
+{"cells": [], "nbformat": 4}
diff --git a/logo.png b/logo.png
new file mode 100644
Binary files /dev/null and b/logo.png differ
`

func TestBuildFreezesNotebookAsPreview(t *testing.T) {
	files := diff.Parse(notebookAndImageDiff)
	g := &model.Group{
		Name: "default",
		Diffs: []*model.Diff{{
			ID:    "d1",
			Title: "first",
			Raw:   notebookAndImageDiff,
			Files: files,
		}},
	}
	p := export.Build(g, "test", time.Now())

	nb := files[0]
	if !nb.IsNotebook {
		t.Fatal("analysis.ipynb should be flagged as a notebook")
	}
	prev, ok := p.Previews["d1:"+nb.ID]
	if !ok {
		t.Fatalf("no preview for %s", nb.Path())
	}
	if !strings.Contains(prev.Content, `"cells"`) || !prev.Complete {
		t.Errorf("preview = %+v", prev)
	}
}

func TestBuildFreezesWorktreeImageAsDataURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("fakepng"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := diff.Parse(notebookAndImageDiff)
	g := &model.Group{
		Name: "default",
		Diffs: []*model.Diff{{
			ID:      "d1",
			Title:   "first",
			BaseDir: dir,
			Raw:     notebookAndImageDiff,
			Files:   files,
		}},
	}
	p := export.Build(g, "test", time.Now())

	img := files[1]
	if !img.IsImage {
		t.Fatal("logo.png should be flagged as an image")
	}
	got, ok := p.Images["d1:"+img.ID]
	if !ok {
		t.Fatalf("no image for %s", img.Path())
	}
	if want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fakepng")); got.DataURL != want {
		t.Errorf("dataUrl = %q, want %q", got.DataURL, want)
	}
}

func TestBuildSkipsImageWithoutWorktreeFile(t *testing.T) {
	files := diff.Parse(notebookAndImageDiff)
	g := &model.Group{
		Name:  "default",
		Diffs: []*model.Diff{{ID: "d1", Title: "first", Raw: notebookAndImageDiff, Files: files}},
	}
	p := export.Build(g, "test", time.Now())
	if len(p.Images) != 0 {
		t.Errorf("images = %+v, want none without a working tree copy", p.Images)
	}
}

func TestBuildPrefersWorktree(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "new.md"), []byte("# From disk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := export.Build(group(t, dir), "test", time.Now())
	for _, prev := range p.Previews {
		if prev.Source != "worktree" || prev.Content != "# From disk\n" {
			t.Errorf("preview = %+v, want the working tree file", prev)
		}
	}
}

func indexHTML(refs ...string) []byte {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
	for _, ref := range refs {
		switch {
		case strings.HasSuffix(ref, ".css"):
			fmt.Fprintf(&b, "<link rel=\"stylesheet\" crossorigin href=%q>\n", ref)
		default:
			fmt.Fprintf(&b, "<script type=\"module\" crossorigin src=%q></script>\n", ref)
		}
	}
	b.WriteString("</head>\n<body>\n<div id=\"root\"></div>\n</body>\n</html>\n")
	return []byte(b.String())
}

func assets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: indexHTML("/assets/index.js", "/assets/index.css")},
		"assets/index.css": {Data: []byte(".diff{color:red}")},
		"assets/index.js":  {Data: []byte("console.log('sbnn')")},
	}
}

func TestRenderStandalonePage(t *testing.T) {
	p := export.Build(group(t, ""), "test", time.Now())
	page, err := export.Render(p, assets(), export.Options{Title: "review of x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<!doctype html>",
		"<title>review of x</title>",
		".diff{color:red}",
		"console.log('sbnn')",
		`<div id="root"></div>`,
		"window.__SBNN_DATA__ = {",
		"</html>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	// The page has to stand on its own: no request back to a sbnn server.
	if strings.Contains(page, "/_/api/") {
		t.Error("the exported page references the sbnn API")
	}
}

func TestRenderFragmentHasNoDocumentTags(t *testing.T) {
	p := export.Build(group(t, ""), "test", time.Now())
	page, err := export.Render(p, assets(), export.Options{Fragment: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"<!doctype", "<html", "<head", "<body"} {
		if strings.Contains(strings.ToLower(page), unwanted) {
			t.Errorf("fragment contains %q", unwanted)
		}
	}
	if !strings.Contains(page, `<div id="root"></div>`) {
		t.Error("fragment has no mount point")
	}
}

func TestRenderEmbedsReadableJSON(t *testing.T) {
	p := export.Build(group(t, ""), "test", time.Now())
	page, err := export.Render(p, assets(), export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "window.__SBNN_DATA__ = "
	_, rest, ok := strings.Cut(page, prefix)
	if !ok {
		t.Fatal("no payload in the page")
	}
	payload, _, ok := strings.Cut(rest, ";</script>")
	if !ok {
		t.Fatal("payload is not terminated")
	}
	var decoded export.Payload
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if decoded.Group != "default" || len(decoded.Diffs) != 1 {
		t.Errorf("payload = %+v", decoded)
	}
	// encoding/json escapes the characters that could end the script early.
	if strings.Contains(payload, "</script") {
		t.Error("payload can terminate its own script element")
	}
}

func TestRenderNeedsBuiltUI(t *testing.T) {
	p := export.Build(group(t, ""), "test", time.Now())
	if _, err := export.Render(p, fstest.MapFS{}, export.Options{}); err == nil {
		t.Fatal("want an error when the UI is not built into the binary")
	}
}

// TestRenderRejectsCodeSplitBuild pins the day Vite emits a second chunk:
// the chunks used to be concatenated in name order into one module, which
// produces a page that is silently blank in the browser.
func TestRenderRejectsCodeSplitBuild(t *testing.T) {
	tests := []struct {
		name  string
		fs    fstest.MapFS
		wants []string
	}{
		{
			name: "chunk imported by the entry",
			fs: fstest.MapFS{
				"index.html":           {Data: indexHTML("/assets/index-aaa.js", "/assets/index-aaa.css")},
				"assets/index-aaa.css": {Data: []byte(".diff{color:red}")},
				"assets/index-aaa.js":  {Data: []byte("import './vendor-bbb.js';console.log('sbnn')")},
				"assets/vendor-bbb.js": {Data: []byte("export const react = 1")},
			},
			wants: []string{"assets/index-aaa.js", "assets/vendor-bbb.js"},
		},
		{
			name: "two entries in index.html",
			fs: fstest.MapFS{
				"index.html":  {Data: indexHTML("/assets/a.js", "/assets/b.js")},
				"assets/a.js": {Data: []byte("console.log('a')")},
				"assets/b.js": {Data: []byte("console.log('b')")},
			},
			wants: []string{"assets/a.js", "assets/b.js"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := export.Build(group(t, ""), "test", time.Now())
			page, err := export.Render(p, tt.fs, export.Options{})
			if err == nil {
				t.Fatalf("Render succeeded on a code split build; page = %q", page)
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestRenderFollowsIndexHTML checks that the entry module and the stylesheet
// order come from index.html, not from the name order of assets/.
func TestRenderFollowsIndexHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: indexHTML("/assets/zz-entry.js", "/assets/zz.css", "/assets/aa.css")},
		"assets/zz-entry.js": {Data: []byte("console.log('entry')")},
		"assets/zz.css":      {Data: []byte(".second{}")},
		"assets/aa.css":      {Data: []byte(".first{}")},
		// Not referenced and not a script: no reason to inline it.
		"assets/font.woff2": {Data: []byte("woff")},
	}
	p := export.Build(group(t, ""), "test", time.Now())
	page, err := export.Render(p, fsys, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "console.log('entry')") {
		t.Error("the entry module was not inlined")
	}
	second, first := strings.Index(page, ".second{}"), strings.Index(page, ".first{}")
	if second < 0 || first < 0 {
		t.Fatalf("stylesheets missing: .second at %d, .first at %d", second, first)
	}
	if second > first {
		t.Error("stylesheets are not in index.html order")
	}
	if strings.Contains(page, "woff") {
		t.Error("a font was inlined as CSS or script")
	}
}

func TestRenderReportsMissingAsset(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: indexHTML("/assets/gone.js")},
	}
	p := export.Build(group(t, ""), "test", time.Now())
	if _, err := export.Render(p, fsys, export.Options{}); err == nil {
		t.Fatal("want an error when index.html references a script that is not embedded")
	} else if !strings.Contains(err.Error(), "gone.js") {
		t.Errorf("error %q does not name the missing asset", err)
	}
}

func TestRenderNeedsAScript(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":       {Data: indexHTML("/assets/index.css")},
		"assets/index.css": {Data: []byte(".diff{}")},
	}
	p := export.Build(group(t, ""), "test", time.Now())
	if _, err := export.Render(p, fsys, export.Options{}); err == nil {
		t.Fatal("want an error when index.html loads no script")
	}
}

// TestBuildCarriesTheReview pins the three fields that say how the review
// ended. Without them an exported page renders a submitted review as if it
// had never been submitted.
func TestBuildCarriesTheReview(t *testing.T) {
	reviewedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name    string
		set     func(*model.Group)
		wantAt  time.Time
		wantNot string
		wantVer model.Verdict
	}{
		{
			name: "approved with a note",
			set: func(g *model.Group) {
				g.ReviewedAt = reviewedAt
				g.ReviewNote = "Ship it, the naming nits are optional."
				g.ReviewVerdict = model.VerdictApproved
			},
			wantAt:  reviewedAt,
			wantNot: "Ship it, the naming nits are optional.",
			wantVer: model.VerdictApproved,
		},
		{
			name: "changes requested without a note",
			set: func(g *model.Group) {
				g.ReviewedAt = reviewedAt
				g.ReviewVerdict = model.VerdictChangesRequested
			},
			wantAt:  reviewedAt,
			wantVer: model.VerdictChangesRequested,
		},
		{
			name: "never submitted",
			set:  func(*model.Group) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := group(t, "")
			tt.set(g)
			p := export.Build(g, "test", time.Now())
			if !p.ReviewedAt.Equal(tt.wantAt) {
				t.Errorf("ReviewedAt = %v, want %v", p.ReviewedAt, tt.wantAt)
			}
			if p.ReviewNote != tt.wantNot {
				t.Errorf("ReviewNote = %q, want %q", p.ReviewNote, tt.wantNot)
			}
			if p.ReviewVerdict != tt.wantVer {
				t.Errorf("ReviewVerdict = %q, want %q", p.ReviewVerdict, tt.wantVer)
			}
		})
	}
}

// TestRenderEmbedsTheReviewUnderTheLiveNames checks the JSON the page reads:
// the static client has to see the same keys the live API sends, or it
// cannot render a frozen review the way it renders a live one.
func TestRenderEmbedsTheReviewUnderTheLiveNames(t *testing.T) {
	g := group(t, "")
	g.ReviewedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	g.ReviewNote = "One question about the retry loop."
	g.ReviewVerdict = model.VerdictApproved

	page, err := export.Render(export.Build(g, "test", time.Now()), assets(), export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payloadJSON(t, page), &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"reviewedAt":    "2026-03-04T05:06:07Z",
		"reviewNote":    "One question about the retry loop.",
		"reviewVerdict": "approved",
	}
	for key, val := range want {
		if got := decoded[key]; got != val {
			t.Errorf("%s = %v, want %v", key, got, val)
		}
	}

	// A group sent through the live API marshals the same three keys, so a
	// page cannot end up reading one name from a server and another from an
	// exported payload.
	live, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var liveDecoded map[string]any
	if err := json.Unmarshal(live, &liveDecoded); err != nil {
		t.Fatal(err)
	}
	for key, val := range want {
		if got := liveDecoded[key]; got != val {
			t.Errorf("model.Group marshals %s as %v, want %v", key, got, val)
		}
	}
}

// TestRenderOmitsAnUnsubmittedReview keeps an unreviewed group free of keys
// that would make the page draw a review banner.
func TestRenderOmitsAnUnsubmittedReview(t *testing.T) {
	page, err := export.Render(export.Build(group(t, ""), "test", time.Now()), assets(), export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payloadJSON(t, page), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"reviewedAt", "reviewNote", "reviewVerdict"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("%s is present for a group that was never reviewed", key)
		}
	}
	if decoded["version"] != float64(export.PayloadVersion) {
		t.Errorf("version = %v, want %d", decoded["version"], export.PayloadVersion)
	}
}

// TestBuildSaysWhetherTheVerdictStillCoversTheDiffs pins the flag the page
// needs to tell a current verdict from a stale one. A live page reads that
// from the status summary; an exported page has no server to ask, so a page
// exported one diff after an approval would otherwise show Approved against
// a change nobody has looked at.
func TestBuildSaysWhetherTheVerdictStillCoversTheDiffs(t *testing.T) {
	reviewedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	tests := []struct {
		name string
		set  func(*model.Group)
		want bool
	}{
		{
			name: "approved, and nothing has arrived since",
			set: func(g *model.Group) {
				g.ReviewedAt = reviewedAt
				g.ReviewVerdict = model.VerdictApproved
				g.Diffs[0].CreatedAt = reviewedAt.Add(-time.Hour)
			},
			want: true,
		},
		{
			name: "approved, then another diff arrived",
			set: func(g *model.Group) {
				g.ReviewedAt = reviewedAt
				g.ReviewVerdict = model.VerdictApproved
				g.Diffs[0].CreatedAt = reviewedAt.Add(time.Hour)
			},
			want: false,
		},
		{
			name: "never submitted",
			set:  func(*model.Group) {},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := group(t, "")
			tt.set(g)
			if got := export.Build(g, "test", time.Now()).Reviewed; got != tt.want {
				t.Errorf("Reviewed = %v, want %v (Group.Reviewed() = %v)", got, tt.want, g.Reviewed())
			}
		})
	}
}

// The flag has to reach the page under a name the page reads, and it is a
// plain false rather than an absent key: absent is what a page written by an
// older sbnn looks like, and those two do not mean the same thing.
func TestRenderEmbedsWhetherTheReviewIsCurrent(t *testing.T) {
	g := group(t, "")
	g.ReviewedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	g.ReviewVerdict = model.VerdictApproved
	g.Diffs[0].CreatedAt = g.ReviewedAt.Add(time.Hour)

	page, err := export.Render(export.Build(g, "test", time.Now()), assets(), export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payloadJSON(t, page), &decoded); err != nil {
		t.Fatal(err)
	}
	got, ok := decoded["reviewed"]
	if !ok {
		t.Fatal("the payload has no reviewed key, so the page cannot tell a stale verdict from a current one")
	}
	if got != false {
		t.Errorf("reviewed = %v, want false: a diff arrived after the approval", got)
	}
	if decoded["reviewVerdict"] != "approved" {
		t.Errorf("reviewVerdict = %v, want approved: the review still happened", decoded["reviewVerdict"])
	}
}

// payloadJSON pulls window.__SBNN_DATA__ back out of a rendered page.
func payloadJSON(t *testing.T, page string) []byte {
	t.Helper()
	const prefix = "window.__SBNN_DATA__ = "
	_, rest, ok := strings.Cut(page, prefix)
	if !ok {
		t.Fatal("no payload in the page")
	}
	payload, _, ok := strings.Cut(rest, ";</script>")
	if !ok {
		t.Fatal("payload is not terminated")
	}
	return []byte(payload)
}
