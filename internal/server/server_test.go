package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/model"
)

const sampleDiff = `diff --git a/README.md b/README.md
index 1111111..2222222 100644
--- a/README.md
+++ b/README.md
@@ -1,2 +1,3 @@
 # sbnn
-old line
+new line
+another line
diff --git a/docs/new.md b/docs/new.md
new file mode 100644
--- /dev/null
+++ b/docs/new.md
@@ -0,0 +1,2 @@
+# New
+body
`

func newTestServer(t *testing.T, opts ...func(*Options)) (*httptest.Server, *Server) {
	t.Helper()
	o := Options{
		SessionFile: filepath.Join(t.TempDir(), "session.json"),
		CacheDir:    t.TempDir(),
		Version:     "test",
		Mo:          mo.New("mo-not-installed-for-tests", 0, ""),
	}
	for _, f := range opts {
		f(&o)
	}
	srv, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

func postJSON(t *testing.T, url string, body any, out any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp
}

func getJSON(t *testing.T, url string, out any) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp
}

func TestAddDiffAndReadGroup(t *testing.T) {
	ts, _ := newTestServer(t)

	var added AddDiffResponse
	resp := postJSON(t, ts.URL+"/_/api/groups/default/diffs",
		AddDiffRequest{Title: "first", BaseDir: "/tmp", Content: sampleDiff}, &added)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if added.Diff == nil || len(added.Diff.Files) != 2 {
		t.Fatalf("stored diff = %+v", added.Diff)
	}

	var g model.Group
	getJSON(t, ts.URL+"/_/api/groups/default", &g)
	if len(g.Diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(g.Diffs))
	}
	files := g.Diffs[0].Files
	if files[0].Status != model.StatusModified || files[0].ViewMode != model.ViewSplit {
		t.Errorf("README.md = %s/%s", files[0].Status, files[0].ViewMode)
	}
	if files[1].Status != model.StatusAdded || files[1].ViewMode != model.ViewUnified {
		t.Errorf("docs/new.md = %s/%s, want added/unified", files[1].Status, files[1].ViewMode)
	}
	if !files[1].IsMarkdown {
		t.Error("docs/new.md should be markdown")
	}
}

func TestAddDiffRejectsGarbage(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/diffs",
		AddDiffRequest{Content: "this is not a diff\n"}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400", resp.Status)
	}
}

func TestGroupsAreSeparate(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	postJSON(t, ts.URL+"/_/api/groups/api/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	var st Status
	getJSON(t, ts.URL+"/_/api/status", &st)
	if len(st.Groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(st.Groups))
	}
	if st.Groups[0].Name != DefaultGroup {
		t.Errorf("first group = %q, want the default group first", st.Groups[0].Name)
	}
	if st.MoAvailable {
		t.Error("MoAvailable = true, want false when the mo binary is missing")
	}
}

func TestCommentLifecycle(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	var comment model.Comment
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID:    added.Diff.ID,
		FileID:    file.ID,
		Path:      file.Path(),
		Side:      "new",
		StartLine: 2,
		EndLine:   3,
		Body:      "please rephrase",
		Snippet:   "+new line\n+another line",
	}, &comment)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if comment.ID == "" || comment.Resolved {
		t.Fatalf("comment = %+v", comment)
	}

	prompt := getText(t, ts.URL+"/_/api/groups/default/prompt")
	if !strings.Contains(prompt, "please rephrase") || !strings.Contains(prompt, "README.md:2-3") {
		t.Errorf("prompt = %q", prompt)
	}

	// Resolving hides the comment from the prompt unless asked for.
	patch, err := http.NewRequest(http.MethodPatch,
		ts.URL+"/_/api/groups/default/comments/"+comment.ID,
		strings.NewReader(`{"resolved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(patch)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if got := getText(t, ts.URL+"/_/api/groups/default/prompt"); strings.Contains(got, "please rephrase") {
		t.Errorf("resolved comment still in the prompt: %q", got)
	}
	if got := getText(t, ts.URL+"/_/api/groups/default/prompt?resolved=true"); !strings.Contains(got, "please rephrase") {
		t.Errorf("resolved comment missing with ?resolved=true: %q", got)
	}

	del, err := http.NewRequest(http.MethodDelete, ts.URL+"/_/api/groups/default/comments/"+comment.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %s", res.Status)
	}

	var comments []*model.Comment
	getJSON(t, ts.URL+"/_/api/groups/default/comments", &comments)
	if len(comments) != 0 {
		t.Errorf("got %d comments after delete, want 0", len(comments))
	}
}

func TestCommentNeedsKnownDiff(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: "nope", FileID: "nope", Path: "x", StartLine: 1, Body: "hi",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400", resp.Status)
	}
}

func TestSessionSurvivesRestart(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session.json")
	ts, _ := newTestServer(t, func(o *Options) { o.SessionFile = sessionFile })
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Title: "kept", Content: sampleDiff}, nil)

	// A second server reading the same file is what --restart does.
	restarted, err := New(Options{SessionFile: sessionFile, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	g, ok := restarted.Store().Group(DefaultGroup)
	if !ok || len(g.Diffs) != 1 || g.Diffs[0].Title != "kept" {
		t.Fatalf("restored group = %+v (ok=%v)", g, ok)
	}
}

func TestPreviewNeedsMarkdown(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: goDiff}, &added)
	file := added.Diff.Files[0]
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/preview", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for a non Markdown file", resp.Status)
	}
}

func TestPreviewReportsMissingMo(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[1] // docs/new.md
	var body map[string]string
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/preview", &body)
	if resp.StatusCode != http.StatusFailedDependency {
		t.Fatalf("status = %s, want 424 when mo is missing", resp.Status)
	}
	if !strings.Contains(body["error"], "mo is not installed") {
		t.Errorf("error = %q", body["error"])
	}
}

func TestPreviewUsesMo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub mo is a shell script")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "mo")
	// The stub records its arguments and answers like `mo --json`.
	script := `#!/bin/sh
echo "$@" > ` + filepath.Join(dir, "args") + `
path=$(eval echo \${$#})
printf '{"url":"http://localhost:6275","files":[{"url":"http://localhost:6275/sbnn-default?file=abc","name":"new.md","path":"%s"}]}\n' "$path"
`
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	ts, _ := newTestServer(t, func(o *Options) {
		o.Mo = mo.New(stub, 6275, "localhost")
		o.CacheDir = cacheDir
	})

	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[1] // docs/new.md, a new file not on disk

	var preview PreviewResponse
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/preview", &preview)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if preview.Source != SourceReconstructed || !preview.Complete {
		t.Errorf("preview = %+v, want a complete reconstruction of a new file", preview)
	}
	if preview.MoURL != "http://localhost:6275/sbnn-default?file=abc" {
		t.Errorf("moUrl = %q", preview.MoURL)
	}
	content, err := os.ReadFile(preview.Path)
	if err != nil {
		t.Fatalf("reconstructed file: %v", err)
	}
	if string(content) != "# New\nbody\n" {
		t.Errorf("reconstructed content = %q", content)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--target sbnn-default") {
		t.Errorf("mo args = %q, want sbnn's own mo group", args)
	}
}

func TestPreviewPrefersWorktreeFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub mo is a shell script")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "mo")
	script := "#!/bin/sh\n" +
		`printf '{"url":"http://localhost:6275","files":[]}\n'` + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(work, "docs", "new.md")
	if err := os.WriteFile(real, []byte("# From the working tree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ts, _ := newTestServer(t, func(o *Options) { o.Mo = mo.New(stub, 6275, "localhost") })
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs",
		AddDiffRequest{Content: sampleDiff, BaseDir: work}, &added)
	file := added.Diff.Files[1]

	var preview PreviewResponse
	getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/preview", &preview)
	if preview.Source != SourceWorktree || preview.Path != real {
		t.Errorf("preview = %+v, want the working tree file %s", preview, real)
	}
}

func TestValidateGroupName(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", DefaultGroup, false},
		{"api", "api", false},
		{"feature-1.2_x", "feature-1.2_x", false},
		{"_internal", "", true},
		{"../etc", "", true},
		{"a/b", "", true},
		{"with space", "", true},
	} {
		got, err := ValidateGroupName(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ValidateGroupName(%q) = %q, want an error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ValidateGroupName(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func getText(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const goDiff = `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-var x = 1
+var x = 2
`

func TestFileContentServesMarkdown(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[1] // docs/new.md, a new file not on disk

	var got FileContentResponse
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/content", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if got.Content != "# New\nbody\n" || got.Source != SourceReconstructed || !got.Complete {
		t.Errorf("content = %+v", got)
	}
	// The Markdown is served without mo being involved at all.
	if !strings.Contains(got.Content, "# New") {
		t.Errorf("content = %q", got.Content)
	}
}

func TestFileContentNeedsMarkdown(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: goDiff}, &added)
	file := added.Diff.Files[0]
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/content", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for a non Markdown file", resp.Status)
	}
}

const notebookDiff = `diff --git a/analysis.ipynb b/analysis.ipynb
new file mode 100644
--- /dev/null
+++ b/analysis.ipynb
@@ -0,0 +1,3 @@
+{"cells": [{"cell_type": "markdown", "source": ["# Title"]}],
+"metadata": {},
+"nbformat": 4}
`

func TestFileContentServesNotebook(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: notebookDiff}, &added)
	file := added.Diff.Files[0]
	if !file.IsNotebook {
		t.Fatal("analysis.ipynb should be flagged as a notebook")
	}

	var got FileContentResponse
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/content", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if !strings.Contains(got.Content, `"cells"`) {
		t.Errorf("content = %q", got.Content)
	}
}

const imageDiff = `diff --git a/logo.png b/logo.png
new file mode 100644
Binary files /dev/null and b/logo.png differ
`

func TestFileImageServesWorktreeFile(t *testing.T) {
	work := t.TempDir()
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'f', 'a', 'k', 'e'}
	if err := os.WriteFile(filepath.Join(work, "logo.png"), pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: imageDiff, BaseDir: work}, &added)
	file := added.Diff.Files[0]
	if !file.IsImage {
		t.Fatal("logo.png should be flagged as an image")
	}

	resp, err := http.Get(ts.URL + "/_/api/groups/default/diffs/" + added.Diff.ID + "/files/" + file.ID + "/image")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(pngBytes) {
		t.Errorf("body = %q, want %q", got, pngBytes)
	}
}

func TestFileImageNeedsWorktreeFile(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: imageDiff}, &added)
	file := added.Diff.Files[0]

	resp, err := http.Get(ts.URL + "/_/api/groups/default/diffs/" + added.Diff.ID + "/files/" + file.ID + "/image")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 when the working tree has nothing to show", resp.Status)
	}
}

func TestSuggestionInPrompt(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	var comment model.Comment
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID:     added.Diff.ID,
		FileID:     file.ID,
		Path:       file.Path(),
		Side:       "new",
		StartLine:  2,
		EndLine:    3,
		Snippet:    "+new line\n+another line",
		Suggestion: "a better line\nand another",
	}, &comment)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	// A suggestion is a complete comment on its own: no body needed, and it
	// is stored inside the body as a fenced block, the way GitHub does it.
	if got := model.Suggestions(comment.Body); len(got) != 1 || got[0] != "a better line\nand another" {
		t.Fatalf("suggestions = %q (body %q)", got, comment.Body)
	}

	prompt := getText(t, ts.URL+"/_/api/groups/default/prompt")
	if !strings.Contains(prompt, "```suggestion\na better line\nand another\n```") {
		t.Errorf("prompt lacks an applicable suggestion block: %q", prompt)
	}
	if !strings.Contains(prompt, "replaces README.md:2-3") {
		t.Errorf("prompt does not name the replaced lines: %q", prompt)
	}
}

func TestSuggestionOnlyOnTheNewSide(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(),
		Side: "old", StartLine: 2, EndLine: 2, Suggestion: "nope",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for a suggestion on the old side", resp.Status)
	}
}

func TestEmptyCommentRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: added.Diff.Files[0].ID, Path: "README.md", StartLine: 1,
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for a comment with neither body nor suggestion", resp.Status)
	}
}

func TestUpdateCommentSuggestion(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	var comment model.Comment
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: added.Diff.Files[0].ID, Path: "README.md",
		Side: "new", StartLine: 2, EndLine: 2, Body: "hmm",
	}, &comment)

	req, err := http.NewRequest(http.MethodPatch,
		ts.URL+"/_/api/groups/default/comments/"+comment.ID,
		strings.NewReader("{\"body\":\"hmm\\n\\n```suggestion\\nreplaced\\n```\"}"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var updated model.Comment
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if got := model.Suggestions(updated.Body); len(got) != 1 || got[0] != "replaced" {
		t.Errorf("updated body = %q, want a suggestion block in it", updated.Body)
	}
	if !strings.HasPrefix(updated.Body, "hmm") {
		t.Errorf("updated body = %q, want the prose kept", updated.Body)
	}
}

func TestCommentByPathResolvesFileAndSnippet(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	// An agent knows the path and the line, not sbnn's internal IDs.
	var comment model.Comment
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		Path:      "README.md",
		Author:    "claude",
		Side:      "new",
		StartLine: 2,
		EndLine:   3,
		Body:      "is this the wording you want?",
	}, &comment)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if comment.DiffID == "" || comment.FileID == "" {
		t.Errorf("comment was not attached to a file: %+v", comment)
	}
	if comment.Author != "claude" {
		t.Errorf("author = %q", comment.Author)
	}
	// The reviewed code is filled in from the diff.
	if comment.Snippet != "+new line\n+another line" {
		t.Errorf("snippet = %q", comment.Snippet)
	}

	if prompt := getText(t, ts.URL+"/_/api/groups/default/prompt"); !strings.Contains(prompt, "From: claude") {
		t.Errorf("prompt does not say who commented: %q", prompt)
	}
}

func TestCommentByPathUnknownFile(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		Path: "nowhere.md", StartLine: 1, Body: "hi",
	}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %s, want 404 for a file no diff carries", resp.Status)
	}
}

func TestCommentByPathLineNotInDiff(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		Path: "README.md", StartLine: 900, Body: "hi",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for a line outside the diff", resp.Status)
	}
}

func TestSubmitReviewNotifiesAndRunsHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a shell command")
	}
	ts, srv := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	// A hook stands in for the agent that is no longer waiting.
	marker := filepath.Join(t.TempDir(), "ran")
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{
		Command: "cat > " + marker,
	}, nil)

	// Something waiting on the event stream is told as well.
	events := make(chan string, 1)
	go func() {
		resp, err := http.Get(ts.URL + "/_/events")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 && strings.Contains(string(buf[:n]), `"type":"review"`) {
				events <- string(buf[:n])
				return
			}
			if err != nil {
				return
			}
		}
	}()
	// Give the subscriber a moment to be registered before submitting.
	for i := 0; i < 50 && srv.broker.count() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	var g model.Group
	resp := postJSON(t, ts.URL+"/_/api/groups/default/review", SubmitReviewRequest{Note: "ok apart from one thing"}, &g)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	if g.ReviewedAt.IsZero() || !g.Reviewed() {
		t.Fatalf("group = %+v, want a submitted review", g)
	}

	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Error("no review event was pushed")
	}

	// The hook receives the prompt on stdin.
	deadline := time.Now().Add(5 * time.Second)
	var written []byte
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(marker)
		if err == nil && len(b) > 0 {
			written = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(written) == 0 {
		t.Fatal("the hook did not run")
	}
	if !strings.Contains(string(written), "ok apart from one thing") {
		t.Errorf("the hook got %q, want the review note in the prompt", written)
	}
}

func TestReviewIsStaleAfterANewDiff(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	postJSON(t, ts.URL+"/_/api/groups/default/review", SubmitReviewRequest{}, nil)

	var st Status
	getJSON(t, ts.URL+"/_/api/status", &st)
	if !st.Groups[0].Reviewed {
		t.Fatal("the group should count as reviewed")
	}

	// A new round starts when the next diff lands.
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: goDiff}, nil)
	getJSON(t, ts.URL+"/_/api/status", &st)
	if st.Groups[0].Reviewed {
		t.Error("a diff sent after the review should make it stale again")
	}
}

func TestHooksAreListedAndCleared(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "echo one"}, nil)
	// The same hook twice is one hook.
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "echo one"}, nil)
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{URL: "http://example.com/x"}, nil)

	var hooks []*model.Hook
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 2 {
		t.Fatalf("got %d hooks, want 2", len(hooks))
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_/api/groups/default/hooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 0 {
		t.Errorf("got %d hooks after clearing, want 0", len(hooks))
	}
}

func TestHookNeedsSomethingToDo(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %s, want 400 for an empty hook", resp.Status)
	}
}

func TestClosingAReviewTakesEverythingWithIt(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: added.Diff.Files[0].ID, Path: "README.md",
		Side: "new", StartLine: 2, Body: "hmm",
	}, nil)
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "true"}, nil)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_/api/groups/default", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %s", res.Status)
	}

	var st Status
	getJSON(t, ts.URL+"/_/api/status", &st)
	if len(st.Groups) != 0 {
		t.Fatalf("groups = %+v, want none left", st.Groups)
	}
	// The hooks went with the review: nothing fires for a review nobody has.
	var hooks []*model.Hook
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 0 {
		t.Errorf("got %d hooks after closing, want 0", len(hooks))
	}
}

func TestClosingEveryReview(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	postJSON(t, ts.URL+"/_/api/groups/api/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_/api/groups", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var removed struct {
		Removed int `json:"removed"`
	}
	if err := json.NewDecoder(res.Body).Decode(&removed); err != nil {
		t.Fatal(err)
	}
	if removed.Removed != 2 {
		t.Errorf("removed = %d, want 2", removed.Removed)
	}
	var st Status
	getJSON(t, ts.URL+"/_/api/status", &st)
	if len(st.Groups) != 0 {
		t.Errorf("groups = %+v, want none left", st.Groups)
	}
}

// sbnn listens on loopback without authentication, so any page the user has
// open can reach it. A hook is a shell command sbnn runs, which makes a POST
// from another site code execution; the guard is what stands between the
// two, and reads have to keep working for the CLI.
func TestCrossOriginRequestsAreRefused(t *testing.T) {
	ts, srv := newTestServer(t)
	// The guard compares ports with the one sbnn was configured with, which
	// httptest picks for us.
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	srv.opts.Port, err = strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	srv.opts.Bind = u.Hostname()

	hooks := ts.URL + "/_/api/groups/default/hooks"
	cases := []struct {
		name    string
		method  string
		url     string
		headers map[string]string
		want    int
	}{
		{"a page on another site, as a simple request", http.MethodPost, hooks,
			map[string]string{"Origin": "https://evil.example", "Content-Type": "text/plain"}, http.StatusForbidden},
		{"a page on another site, named by the browser", http.MethodPost, hooks,
			map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"another port on this machine", http.MethodPost, hooks,
			map[string]string{"Origin": "http://localhost:1"}, http.StatusForbidden},
		{"a sandboxed page, which sends Origin: null", http.MethodPost, hooks,
			map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"shutting the server down from another site", http.MethodPost, ts.URL + "/_/api/shutdown",
			map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"sbnn's own page", http.MethodPost, hooks,
			map[string]string{"Origin": ts.URL, "Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"sbnn's own page under another loopback name", http.MethodPost, hooks,
			map[string]string{"Origin": "http://127.0.0.1:" + u.Port()}, http.StatusOK},
		{"the command line, which names no origin", http.MethodPost, hooks, nil, http.StatusOK},
		{"reading, which CORS already guards", http.MethodGet, ts.URL + "/_/api/status",
			map[string]string{"Origin": "https://evil.example"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.url, strings.NewReader(`{"command":"echo hi"}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
		})
	}
}

// A review submitted over the API is a review like any other: it wakes what
// is waiting, and it is written into the log. That is what lets a reviewer
// who is not a person - `sbnn submit` - end a round.
func TestSubmitReviewWithoutABrowser(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "reviews.jsonl")
	ts, _ := newTestServer(t, func(o *Options) { o.HistoryFile = logFile })

	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{
		Content: sampleDiff,
		Labels:  map[string]string{"rev": "deadbeef"},
	}, &AddDiffResponse{})
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		Path: "README.md", Side: "new", StartLine: 2, EndLine: 2,
		Body: "this reads oddly", Author: "code-review",
	}, &model.Comment{})

	var reviewed model.Group
	resp := postJSON(t, ts.URL+"/_/api/groups/default/review", SubmitReviewRequest{Note: "one thing"}, &reviewed)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !reviewed.Reviewed() || reviewed.ReviewNote != "one thing" {
		t.Errorf("group = %+v, want it marked reviewed", reviewed)
	}
	records, err := history.Load(logFile, history.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d record(s) in the log, want the review to have been kept", len(records))
	}
	rec := records[0]
	if rec.Note != "one thing" || rec.Labels["rev"] != "deadbeef" {
		t.Errorf("record = %+v", rec)
	}
	if len(rec.Comments) != 1 || rec.Comments[0].Author != "code-review" {
		t.Fatalf("comments = %+v, want the reviewer named", rec.Comments)
	}
	// Naming the author is what lets a later reading tell the two kinds of
	// reviewer apart.
	if got := history.Comments(records); len(got) != 1 || got[0].Who() != "code-review" {
		t.Errorf("flattened = %+v", got)
	}
}

// A body over the limit used to be cut mid-JSON by an io.LimitReader, which
// does not say it truncated. The decoder then blamed the JSON, so a large but
// perfectly valid request came back as "400 invalid request: unexpected EOF" -
// the user is told they sent something malformed, nothing names the limit, and
// nothing says a limit was involved. The CLI already words it properly on the
// same value; every other client (curl, an MCP server, an editor extension)
// got the obscure one.
func TestOversizedBodyNamesTheLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	// A review note with a large stack trace pasted into it.
	huge := strings.Repeat("x", maxBodySize+(1<<10))
	body, err := json.Marshal(AddCommentRequest{
		DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(),
		Side: "new", StartLine: 2, Body: huge,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/comments",
		"application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %s, want 413: %s", resp.Status, got)
	}
	if strings.Contains(string(got), "unexpected EOF") {
		t.Errorf("body = %q, want the limit named rather than the JSON blamed", got)
	}
	if !strings.Contains(string(got), "too large") || !strings.Contains(string(got), "1MB") {
		t.Errorf("body = %q, want it to name the limit", got)
	}
}

// The two failures have to stay distinguishable: an overrun is 413 and names
// the limit, genuinely malformed JSON under the limit is still 400.
func TestDecodeBodySeparatesOverrunFromBadJSON(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		limit    int64
		want     int
		contains string
	}{
		{"a body over the limit", `{"content":"` + strings.Repeat("x", 128) + `"}`, 32,
			http.StatusRequestEntityTooLarge, "too large"},
		{"malformed JSON under the limit", `{"content":`, 1 << 20,
			http.StatusBadRequest, "invalid request"},
		{"a body that fits", `{"content":"ok"}`, 1 << 20, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var dst AddDiffRequest
			if decodeBody(w, r, tc.limit, "the diff", &dst) {
				if tc.want != http.StatusOK {
					t.Fatalf("decodeBody accepted %q, want %d", tc.body, tc.want)
				}
				return
			}
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", w.Code, tc.want, w.Body)
			}
			if !strings.Contains(w.Body.String(), tc.contains) {
				t.Errorf("body = %q, want it to contain %q", w.Body, tc.contains)
			}
		})
	}
}

// The command line and the server bound a diff by the same number. They used
// to be separate constants with the same value in two packages, free to drift.
func TestMaxDiffSizeIsExported(t *testing.T) {
	if MaxDiffSize != 32<<20 {
		t.Errorf("MaxDiffSize = %d, want 32MB", MaxDiffSize)
	}
	if megabytes(MaxDiffSize) != "32MB" {
		t.Errorf("megabytes(MaxDiffSize) = %q, want 32MB", megabytes(MaxDiffSize))
	}
}
