package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
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
	// The patch is applied here, so the working tree carries the new side
	// and more of it than the diff does.
	worktree := "# New\nbody\nonly in the working tree\n"
	if err := os.WriteFile(real, []byte(worktree), 0o600); err != nil {
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

// --dangerously-allow-remote-access binds sbnn somewhere other than loopback,
// and the browser then reports the address the user actually typed. That
// address is not opts.Bind and it is not loopback, so matching Origin against
// either one refuses every write and leaves a read-only page behind a flag
// that advertises a working one. What identifies the page in that setup is
// the browser's own same-origin verdict, and failing that, the Host the
// request was dialled at.
func TestCrossOriginUnderRemoteBind(t *testing.T) {
	_, srv := newTestServer(t, func(o *Options) {
		o.Bind = "0.0.0.0"
		o.Port = 6280
		o.AllowRemote = true
	})

	const lan = "192.168.1.5:6280"
	cases := []struct {
		name    string
		host    string
		headers map[string]string
		want    bool // refused as cross-origin
	}{
		{"the page the user typed, named by the browser", lan,
			map[string]string{"Origin": "http://" + lan, "Sec-Fetch-Site": "same-origin"}, false},
		{"the page the user typed, on a browser too old for Sec-Fetch-Site", lan,
			map[string]string{"Origin": "http://" + lan}, false},
		{"the address bar", lan,
			map[string]string{"Sec-Fetch-Site": "none"}, false},
		{"loopback still counts", "localhost:6280",
			map[string]string{"Origin": "http://localhost:6280"}, false},
		{"another site, named by the browser", lan,
			map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, true},
		{"another site, as a simple request with no Sec-Fetch-Site", lan,
			map[string]string{"Origin": "https://evil.example"}, true},
		{"another port on the same machine", lan,
			map[string]string{"Origin": "http://192.168.1.5:1"}, true},
		{"a sandboxed page, which sends Origin: null", lan,
			map[string]string{"Origin": "null"}, true},
		{"the command line, which names no origin", lan, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/_/api/groups/default/review", strings.NewReader(`{}`))
			r.Host = tc.host
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			reason, got := srv.crossOrigin(r)
			if got != tc.want {
				t.Errorf("crossOrigin() = %v (%q), want %v", got, reason, tc.want)
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

// A fileId that names no file of the diff anchors the comment to nothing:
// the page keys its sections on diffId:fileId, so the comment counted in
// every "N open comment(s)" total was shown on no line at all.
func TestHandleAddCommentRejectsUnknownFileID(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]
	other := added.Diff.Files[1]

	cases := []struct {
		name   string
		diffID string
		fileID string
		want   int
	}{
		{"a file of this diff", added.Diff.ID, file.ID, http.StatusOK},
		{"another file of this diff", added.Diff.ID, other.ID, http.StatusOK},
		{"a fileId of no file", added.Diff.ID, "bogus", http.StatusBadRequest},
		{"an empty-looking but wrong fileId", added.Diff.ID, "f1-00000000", http.StatusBadRequest},
		{"an unknown diffId is still refused", "nosuchdiff", file.ID, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var comment model.Comment
			out := any(nil)
			if tc.want == http.StatusOK {
				out = &comment
			}
			resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
				DiffID: tc.diffID, FileID: tc.fileID, Path: file.Path(), Side: "new",
				StartLine: 1, EndLine: 1, Body: "hi",
			}, out)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %s, want %d", resp.Status, tc.want)
			}
			if tc.want == http.StatusOK && comment.FileID != tc.fileID {
				t.Errorf("stored fileId = %q, want %q", comment.FileID, tc.fileID)
			}
		})
	}

	// Every stored comment must name a file that really is in its diff,
	// or the page and the comment count disagree for good.
	var comments []*model.Comment
	getJSON(t, ts.URL+"/_/api/groups/default/comments", &comments)
	if len(comments) != 2 {
		t.Errorf("stored %d comments, want 2", len(comments))
	}
	for _, c := range comments {
		if _, _, ok := findFileOfDiff(added.Diff, c.FileID); !ok {
			t.Errorf("stored comment anchored to no file of the diff: %+v", c)
		}
	}
}

// findFileOfDiff reports whether the diff has a file with this id.
func findFileOfDiff(d *model.Diff, fileID string) (*model.Diff, *model.File, bool) {
	for _, f := range d.Files {
		if f.ID == fileID {
			return d, f, true
		}
	}
	return nil, nil, false
}

// A comment has to point at a line that can exist. The Line model uses
// 1-based numbers with 0 meaning "not on this side", so a non-positive
// startLine names nothing, and an endLine before the start is not a range.
func TestHandleAddCommentRejectsNonPositiveLines(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	cases := []struct {
		name      string
		startLine int
		endLine   int
		want      int
		wantEnd   int // the stored endLine, checked when want is 200
	}{
		{"a negative start line", -5, -1, http.StatusBadRequest, 0},
		{"line zero", 0, 0, http.StatusBadRequest, 0},
		{"an end line before the start", 3, 2, http.StatusBadRequest, 0},
		{"no end line at all means the one line", 2, 0, http.StatusOK, 2},
		{"a proper range", 2, 3, http.StatusOK, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var comment model.Comment
			resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
				DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: "new",
				StartLine: tc.startLine, EndLine: tc.endLine, Body: "hi",
			}, nil)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %s, want %d", resp.Status, tc.want)
			}
			if tc.want != http.StatusOK {
				return
			}
			// The accepted ones must also be stored with the range asked for.
			resp = postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
				DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: "new",
				StartLine: tc.startLine, EndLine: tc.endLine, Body: "hi",
			}, &comment)
			if comment.StartLine != tc.startLine || comment.EndLine != tc.wantEnd {
				t.Errorf("stored range = %d-%d, want %d-%d",
					comment.StartLine, comment.EndLine, tc.startLine, tc.wantEnd)
			}
		})
	}

	// Nothing that was refused may have reached the store.
	var comments []*model.Comment
	getJSON(t, ts.URL+"/_/api/groups/default/comments", &comments)
	for _, c := range comments {
		if c.StartLine < 1 || c.EndLine < c.StartLine {
			t.Errorf("stored comment with an impossible range: %+v", c)
		}
	}
}

// The API used to fold every side that was not the literal "old" into
// "new", so "OLD" attached the comment to the new side -- a different
// file, on lines that mean something else -- and said 200. The CLI is
// strict about the same field, so the two layers disagreed silently.
func TestHandleAddCommentValidatesSide(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	cases := []struct {
		name     string
		side     string
		want     int
		wantSide string // the side actually stored, checked when want is 200
	}{
		{"lower case new", "new", http.StatusOK, "new"},
		{"lower case old", "old", http.StatusOK, "old"},
		{"empty means new", "", http.StatusOK, "new"},
		{"upper case old is old, not new", "OLD", http.StatusOK, "old"},
		{"mixed case old", "Old", http.StatusOK, "old"},
		{"padded old", "  old  ", http.StatusOK, "old"},
		{"upper case new", "NEW", http.StatusOK, "new"},
		{"a different word", "left", http.StatusBadRequest, ""},
		{"a typo", "NEWW", http.StatusBadRequest, ""},
		{"nonsense", "sideways", http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := AddCommentRequest{
				DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: tc.side,
				StartLine: 1, EndLine: 1, Body: "hi",
			}
			if tc.want != http.StatusOK {
				resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", req, nil)
				if resp.StatusCode != tc.want {
					t.Fatalf("status = %s, want %d", resp.Status, tc.want)
				}
				return
			}
			var comment model.Comment
			resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", req, &comment)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %s, want %d", resp.Status, tc.want)
			}
			// A 200 is not enough: the point of the bug was that the
			// wrong side was stored while the status looked fine.
			if comment.Side != tc.wantSide {
				t.Errorf("stored side = %q, want %q", comment.Side, tc.wantSide)
			}
		})
	}

	// Nothing refused reached the store, and every stored side is canonical.
	var comments []*model.Comment
	getJSON(t, ts.URL+"/_/api/groups/default/comments", &comments)
	if len(comments) != 7 {
		t.Errorf("stored %d comments, want 7", len(comments))
	}
	for _, c := range comments {
		if c.Side != "new" && c.Side != "old" {
			t.Errorf("stored comment with a non-canonical side: %+v", c)
		}
	}
}

// A suggestion replaces lines of the new file, so it cannot sit on the
// old side however that side was spelled. The check used to run before
// the side was folded, so "OLD" slipped past it.
func TestHandleAddCommentRejectsSuggestionOnFoldedOldSide(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	for _, side := range []string{"old", "OLD", " Old "} {
		t.Run(side, func(t *testing.T) {
			resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
				DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: side,
				StartLine: 1, EndLine: 1, Body: "try this", Suggestion: "replacement",
			}, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %s, want 400", resp.Status)
			}
		})
	}
}

// GET /_/api/groups/{group} returns the whole group, and the page refetches it
// on every change event, so its size is paid again for each edit - measured at
// 6.61 MB for a 500-file review. Diff.Raw is the original diff text and no
// client reads it: it is declared in web/src/types.ts and used nowhere, and
// export.Build already drops it. The store must keep it, since an export or a
// re-parse still wants it.
func TestGroupResponseOmitsRawDiffText(t *testing.T) {
	ts, srv := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs",
		AddDiffRequest{Title: "first", Content: sampleDiff}, nil)

	resp, err := http.Get(ts.URL + "/_/api/groups/default")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "diff --git") {
		t.Errorf("the group response still carries the raw diff text: %s", body)
	}

	// The parsed files - what the UI actually renders - are still there.
	var g model.Group
	if err := json.Unmarshal(body, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Diffs) != 1 || len(g.Diffs[0].Files) != 2 {
		t.Fatalf("group = %+v, want the parsed files intact", g)
	}
	if g.Diffs[0].Raw != "" {
		t.Errorf("Raw = %q, want it dropped from the response", g.Diffs[0].Raw)
	}

	// The store keeps it.
	stored, ok := srv.Store().Group(DefaultGroup)
	if !ok {
		t.Fatal("the group is gone from the store")
	}
	if !strings.Contains(stored.Diffs[0].Raw, "diff --git") {
		t.Errorf("the store lost the raw diff text: %q", stored.Diffs[0].Raw)
	}
}

// The saving this buys, on the endpoint the page refetches on every event.
func BenchmarkGroupResponseSize(b *testing.B) {
	srv, err := New(Options{SessionFile: filepath.Join(b.TempDir(), "s.json"), Version: "test"})
	if err != nil {
		b.Fatal(err)
	}
	raw := strings.Repeat(sampleDiff, 100)
	srv.Store().AddDiff(DefaultGroup, &model.Diff{Raw: raw, Files: diff.Parse(raw)})

	g, _ := srv.Store().Group(DefaultGroup)
	with, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		b.Fatal(err)
	}
	g, _ = srv.Store().Group(DefaultGroup)
	without, err := json.MarshalIndent(withoutRawDiffs(g), "", "  ")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		g, _ := srv.Store().Group(DefaultGroup)
		if _, err := json.MarshalIndent(withoutRawDiffs(g), "", "  "); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(len(with)), "bytes-with-raw")
	b.ReportMetric(float64(len(without)), "bytes-without-raw")
	b.ReportMetric(100*float64(len(with)-len(without))/float64(len(with)), "%-saved")
}

// postChunked sends body with no Content-Length, the way curl does with
// -H "Transfer-Encoding: chunked" and the way many HTTP/2 clients send.
// Wrapping the reader hides its length from net/http, so the request goes
// out chunked and the server sees ContentLength == -1.
func postChunked(t *testing.T, url, body string, out any) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, io.NopCloser(strings.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if req.ContentLength != 0 {
		t.Fatalf("the request carried a Content-Length of %d, so it is not the case under test", req.ContentLength)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp
}

// A review submitted with an unknown Content-Length used to have its body
// thrown away without an error, so "approved" and "changes-requested"
// were both recorded as a plain "commented" with no note. sbnn wait
// --exit-code and sbnn submit --exit-code act on that value, so the
// verdict arrived downstream as the opposite of what was decided.
func TestSubmitReviewReadsAChunkedBody(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		want        int
		wantVerdict model.Verdict
		wantNote    string
	}{
		{"approved", `{"verdict":"approved","note":"looks right"}`, http.StatusOK, model.VerdictApproved, "looks right"},
		{"changes requested", `{"verdict":"changes-requested","note":"not yet"}`, http.StatusOK, model.VerdictChangesRequested, "not yet"},
		{"commented", `{"verdict":"commented","note":"a thought"}`, http.StatusOK, model.VerdictCommented, "a thought"},
		{"a note with no verdict", `{"note":"just a note"}`, http.StatusOK, model.VerdictCommented, "just a note"},
		{"an empty object", `{}`, http.StatusOK, model.VerdictCommented, ""},
		{"a genuinely empty body is not an error", ``, http.StatusOK, model.VerdictCommented, ""},
		{"a bad verdict is still refused", `{"verdict":"lgtm-ish"}`, http.StatusBadRequest, "", ""},
		{"malformed json is still refused", `{"verdict":`, http.StatusBadRequest, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, _ := newTestServer(t)
			postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)

			var g model.Group
			out := any(nil)
			if tc.want == http.StatusOK {
				out = &g
			}
			resp := postChunked(t, ts.URL+"/_/api/groups/default/review", tc.body, out)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %s, want %d", resp.Status, tc.want)
			}
			if tc.want != http.StatusOK {
				return
			}
			// The status was 200 before the fix as well. What was lost
			// was the verdict and the note.
			if g.ReviewVerdict != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", g.ReviewVerdict, tc.wantVerdict)
			}
			if g.ReviewNote != tc.wantNote {
				t.Errorf("note = %q, want %q", g.ReviewNote, tc.wantNote)
			}
			// And it must be what the store kept, not just what came back.
			var reread model.Group
			getJSON(t, ts.URL+"/_/api/groups/default", &reread)
			if reread.ReviewVerdict != tc.wantVerdict || reread.ReviewNote != tc.wantNote {
				t.Errorf("stored verdict/note = %q/%q, want %q/%q",
					reread.ReviewVerdict, reread.ReviewNote, tc.wantVerdict, tc.wantNote)
			}
		})
	}
}

// A request with no body at all still submits a commented review, which
// is what a reviewer who did not choose is saying.
func TestSubmitReviewWithNoBodyAtAll(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/_/api/groups/default/review", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s, want 200", resp.Status)
	}
	var g model.Group
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.ReviewVerdict != model.VerdictCommented {
		t.Errorf("verdict = %q, want %q", g.ReviewVerdict, model.VerdictCommented)
	}
	if !g.Reviewed() {
		t.Errorf("group = %+v, want a submitted review", g)
	}
}

// sendNoBody performs a bodyless request and returns the response.
func sendNoBody(t *testing.T, method, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// rawGroupFields reads GET .../groups/{group} and returns its top-level
// fields unparsed, so that null and [] can be told apart -- decoding into
// model.Group turns both into a nil slice and hides the bug.
func rawGroupFields(t *testing.T, url string) map[string]json.RawMessage {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s, want 200", resp.Status)
	}
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

// A group that exists but holds nothing marshalled its nil slices as
// null, while the very same endpoint answered [] for a group that did not
// exist at all. The shape of the response depended on how the group came
// to be, which every consumer of the API would have to know about.
func TestHandleGroupAlwaysReturnsArrays(t *testing.T) {
	cases := []struct {
		name string
		// empty names the fields that must come back as [] for this
		// group; a field holding something is checked separately.
		empty []string
		group string
		setup func(t *testing.T, ts *httptest.Server, group string)
	}{
		{
			name:  "a group that does not exist",
			group: "never-made",
			empty: []string{"diffs", "comments"},
			setup: func(t *testing.T, ts *httptest.Server, group string) {},
		},
		{
			name:  "a group created by a hook and nothing else",
			group: "hooked",
			empty: []string{"diffs", "comments"},
			setup: func(t *testing.T, ts *httptest.Server, group string) {
				resp := postJSON(t, ts.URL+"/_/api/groups/"+group+"/hooks",
					model.Hook{Command: "echo hi"}, nil)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("adding a hook: status = %s", resp.Status)
				}
			},
		},
		{
			name:  "a group whose only diff was deleted",
			group: "emptied",
			empty: []string{"diffs", "comments"},
			setup: func(t *testing.T, ts *httptest.Server, group string) {
				var added AddDiffResponse
				postJSON(t, ts.URL+"/_/api/groups/"+group+"/diffs", AddDiffRequest{Content: sampleDiff}, &added)
				resp := sendNoBody(t, http.MethodDelete,
					ts.URL+"/_/api/groups/"+group+"/diffs/"+added.Diff.ID)
				if resp.StatusCode != http.StatusNoContent {
					t.Fatalf("deleting the diff: status = %s", resp.Status)
				}
			},
		},
		{
			name: "a group whose comments were cleared",
			// This one keeps its diff, so only comments must be [].
			group: "cleared",
			empty: []string{"comments"},
			setup: func(t *testing.T, ts *httptest.Server, group string) {
				var added AddDiffResponse
				postJSON(t, ts.URL+"/_/api/groups/"+group+"/diffs", AddDiffRequest{Content: sampleDiff}, &added)
				postJSON(t, ts.URL+"/_/api/groups/"+group+"/comments", AddCommentRequest{
					DiffID: added.Diff.ID, FileID: added.Diff.Files[0].ID, Path: added.Diff.Files[0].Path(),
					Side: "new", StartLine: 1, EndLine: 1, Body: "hi",
				}, nil)
				resp := sendNoBody(t, http.MethodDelete, ts.URL+"/_/api/groups/"+group+"/comments")
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("clearing the comments: status = %s", resp.Status)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, _ := newTestServer(t)
			tc.setup(t, ts, tc.group)

			fields := rawGroupFields(t, ts.URL+"/_/api/groups/"+tc.group)
			for _, key := range tc.empty {
				got, ok := fields[key]
				if !ok {
					t.Errorf("the response has no %q field at all", key)
					continue
				}
				if string(got) != "[]" {
					t.Errorf("%q = %s, want []", key, got)
				}
			}
		})
	}
}

// A group that holds something still reports what it holds: the
// normalisation must only replace a nil slice, never an occupied one.
func TestHandleGroupKeepsWhatTheGroupHolds(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: added.Diff.Files[0].ID, Path: added.Diff.Files[0].Path(),
		Side: "new", StartLine: 1, EndLine: 1, Body: "hi",
	}, nil)

	var g model.Group
	getJSON(t, ts.URL+"/_/api/groups/default", &g)
	if len(g.Diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(g.Diffs))
	}
	if len(g.Comments) != 1 {
		t.Errorf("comments = %d, want 1", len(g.Comments))
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

// The command line and the server bound a diff by the same number, and they
// are still two constants in two packages. The old test here only compared
// MaxDiffSize with the literal it is written as, so cmd could be changed to
// 4MB and everything stayed green. This one reads cmd's constant and fails
// when the two drift, which is the whole reason the server's is exported.
func TestMaxDiffSizeMatchesTheCommandLine(t *testing.T) {
	path := filepath.Join("..", "..", "cmd", "root.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^const maxDiffSize = (.+)$`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("no `const maxDiffSize` in %s; if cmd now uses server.MaxDiffSize, delete this test", path)
	}
	got, err := constExpr(m[1])
	if err != nil {
		t.Fatalf("cmd/root.go: maxDiffSize = %q: %v", m[1], err)
	}
	if got != MaxDiffSize {
		t.Errorf("cmd/root.go maxDiffSize = %d, server.MaxDiffSize = %d; the two ends of the same limit have drifted",
			got, MaxDiffSize)
	}
}

// constExpr evaluates the handful of shapes the limit is written in: a plain
// number, or "n << k".
func constExpr(src string) (int64, error) {
	src = strings.TrimSpace(strings.SplitN(src, "//", 2)[0])
	lhs, rhs, shift := strings.Cut(src, "<<")
	n, err := strconv.ParseInt(strings.TrimSpace(lhs), 10, 64)
	if err != nil {
		return 0, err
	}
	if !shift {
		return n, nil
	}
	k, err := strconv.ParseInt(strings.TrimSpace(rhs), 10, 64)
	if err != nil {
		return 0, err
	}
	return n << k, nil
}

// A diff inside MaxDiffSize has to be accepted however much JSON escaping adds
// to it on the way. Bounding the request body by MaxDiffSize meant a
// 32,999,998-byte diff - under 32MB, and accepted by `sbnn`'s own stdin check
// - arrived as a 33,804,914-byte body and was refused with "the diff is too
// large (max 32MB)", which was not true of the diff.
func TestADiffJustUnderTheLimitIsAccepted(t *testing.T) {
	ts, _ := newTestServer(t)

	// Every line carries a quote, so JSON escaping grows the body the way the
	// reported diff did.
	var b strings.Builder
	b.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,2 @@\n-old\n")
	for b.Len() < MaxDiffSize-64 {
		b.WriteString("+a line with \"quotes\" in it\n")
	}
	content := b.String()
	if len(content) > MaxDiffSize {
		t.Fatalf("test diff is %d bytes, over the limit it is meant to sit under", len(content))
	}

	body, err := json.Marshal(AddDiffRequest{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) <= MaxDiffSize {
		t.Fatalf("body is %d bytes, not over MaxDiffSize (%d): this test would pass without the fix",
			len(body), MaxDiffSize)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/diffs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %s for a %d-byte diff in a %d-byte body, want 200: %s",
			resp.Status, len(content), len(body), got)
	}
}

// ...and a diff genuinely over the limit is still refused, with the size the
// sender can check: the diff's, not the envelope's.
func TestADiffOverTheLimitNamesTheDiff(t *testing.T) {
	ts, _ := newTestServer(t)

	content := "diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,2 @@\n-old\n+" +
		strings.Repeat("x", MaxDiffSize) + "\n"
	body, err := json.Marshal(AddDiffRequest{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/diffs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %s, want 413: %s", resp.Status, got)
	}
	if !strings.Contains(string(got), "the diff is too large (max 32MB)") {
		t.Errorf("body = %q, want the diff named with its own limit", got)
	}
}

// byteLimit is what every one of those messages ends with, so a limit it
// cannot render is a message that tells the sender nothing.
func TestByteLimit(t *testing.T) {
	for _, tt := range []struct {
		want string
		in   int64
	}{
		{in: MaxDiffSize, want: "32MB"},
		{in: maxDiffBodySize, want: "65MB"},
		{in: maxBodySize, want: "1MB"},
		// Regression: everything under a megabyte used to render "0MB", so a
		// refusal named a limit of nothing.
		{in: 512 << 10, want: "512KB"},
		{in: 1 << 10, want: "1KB"},
		{in: 900, want: "900B"},
		{in: 0, want: "0B"},
		{in: 3 << 19, want: "1.5MB"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			if got := byteLimit(tt.in); got != tt.want {
				t.Errorf("byteLimit(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
