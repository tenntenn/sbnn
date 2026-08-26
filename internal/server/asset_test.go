package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/asset"
)

// assetDiff adds a Markdown file that draws three pictures the diff itself
// never mentions: one next to it, one too heavy to carry, and one outside the
// directory the diff was sent from.
const assetDiff = `diff --git a/docs/guide.md b/docs/guide.md
new file mode 100644
--- /dev/null
+++ b/docs/guide.md
@@ -0,0 +1,6 @@
+# Guide
+
+![the shape of it](diagram.png)
+![the heavy one](huge.png)
+![what is out there](../../secret.png)
+![nothing there](gone.png)
`

var diagramBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'd', 'i', 'a'}

// assetTree lays out the working tree the assetDiff document points into, and
// returns the directory the diff is sent from.
func assetTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "docs", "diagram.png"), diagramBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "docs", "huge.png"), make([]byte, asset.MaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	// Really there, and really outside what the diff covers.
	if err := os.WriteFile(filepath.Join(root, "secret.png"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	return work
}

func assetContent(t *testing.T, ts *httptest.Server, work string) (FileContentResponse, string, string) {
	t.Helper()
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: assetDiff, BaseDir: work}, &added)
	file := added.Diff.Files[0]
	var got FileContentResponse
	resp := getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+file.ID+"/content", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
	return got, added.Diff.ID, file.ID
}

func TestFileContentResolvesTheImagesTheMarkdownPointsAt(t *testing.T) {
	work := assetTree(t)
	ts, _ := newTestServer(t)
	got, diffID, fileID := assetContent(t, ts, work)

	near, ok := got.Assets["diagram.png"]
	if !ok {
		t.Fatalf("no asset for diagram.png: %+v", got.Assets)
	}
	if near.Status != asset.StatusOK || near.URL == "" {
		t.Fatalf("diagram.png = %+v, want a URL to fetch it from", near)
	}
	// The URL is the endpoint for this very file, so nothing else can be
	// asked for through it.
	wantPrefix := "/_/api/groups/default/diffs/" + diffID + "/files/" + fileID + "/asset?path="
	if !strings.HasPrefix(near.URL, wantPrefix) {
		t.Errorf("url = %q, want it to start with %q", near.URL, wantPrefix)
	}

	resp, err := http.Get(ts.URL + near.URL)
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(diagramBytes) {
		t.Errorf("body = %q, want the working tree file", body)
	}
}

func TestFileContentSaysWhyAnImageIsNotThere(t *testing.T) {
	work := assetTree(t)
	ts, _ := newTestServer(t)
	got, _, _ := assetContent(t, ts, work)

	for src, want := range map[string]asset.Status{
		"huge.png":         asset.StatusTooLarge,
		"../../secret.png": asset.StatusOutside,
		"gone.png":         asset.StatusMissing,
	} {
		e, ok := got.Assets[src]
		if !ok {
			t.Errorf("no entry for %s: %+v", src, got.Assets)
			continue
		}
		if e.Status != want {
			t.Errorf("%s status = %q, want %q", src, e.Status, want)
		}
		if e.URL != "" {
			t.Errorf("%s was given the URL %q, which must not be served", src, e.URL)
		}
		if e.Path == "" {
			t.Errorf("%s has no path, so the page cannot name it", src)
		}
	}
}

func TestFileAssetServesOnlyWhatTheDocumentNames(t *testing.T) {
	work := assetTree(t)
	ts, _ := newTestServer(t)
	_, diffID, fileID := assetContent(t, ts, work)

	base := ts.URL + "/_/api/groups/default/diffs/" + diffID + "/files/" + fileID + "/asset?path="
	for _, path := range []string{
		"../secret.png",  // out of the tree
		"/etc/passwd",    // rooted elsewhere
		"docs/huge.png",  // named, but past the cap
		"docs/other.png", // in the tree, never mentioned
		"docs/../../secret.png",
		"",
	} {
		resp, err := http.Get(base + url.QueryEscape(path))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET path=%q = %s, want 404; body %q", path, resp.Status, body)
		}
		if strings.Contains(string(body), "secret") && path != "../secret.png" && path != "docs/../../secret.png" {
			t.Errorf("GET path=%q served %q", path, body)
		}
	}
}

func TestFileContentLeavesANonMarkdownFileWithoutAssets(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: notebookDiff}, &added)
	var got FileContentResponse
	getJSON(t, ts.URL+"/_/api/groups/default/diffs/"+added.Diff.ID+"/files/"+added.Diff.Files[0].ID+"/content", &got)
	if got.Assets != nil {
		t.Errorf("assets = %+v, want none for a notebook", got.Assets)
	}
}

// gatedDiff adds, next to the Markdown document, one file per kind of source
// the preview endpoint learned to serve as text, each of them writing an
// image reference the way a Markdown document would - in a comment, in a
// string, in a value. None of them is a document, so none of them may hand
// out the bytes of docs/diagram.png.
const gatedDiff = `diff --git a/docs/guide.md b/docs/guide.md
new file mode 100644
--- /dev/null
+++ b/docs/guide.md
@@ -0,0 +1,3 @@
+# Guide
+
+![the shape of it](diagram.png)
diff --git a/docs/evil.go b/docs/evil.go
new file mode 100644
--- /dev/null
+++ b/docs/evil.go
@@ -0,0 +1,3 @@
+package docs
+
+// ![the shape of it](diagram.png)
diff --git a/docs/evil.ts b/docs/evil.ts
new file mode 100644
--- /dev/null
+++ b/docs/evil.ts
@@ -0,0 +1,1 @@
+export const doc = "![the shape of it](diagram.png)";
diff --git a/docs/evil.json b/docs/evil.json
new file mode 100644
--- /dev/null
+++ b/docs/evil.json
@@ -0,0 +1,1 @@
+{"doc": "![the shape of it](diagram.png)"}
diff --git a/docs/evil.yaml b/docs/evil.yaml
new file mode 100644
--- /dev/null
+++ b/docs/evil.yaml
@@ -0,0 +1,1 @@
+doc: '![the shape of it](diagram.png)'
diff --git a/docs/evil.txt b/docs/evil.txt
new file mode 100644
--- /dev/null
+++ b/docs/evil.txt
@@ -0,0 +1,1 @@
+![the shape of it](diagram.png)
diff --git a/docs/evil b/docs/evil
new file mode 100644
--- /dev/null
+++ b/docs/evil
@@ -0,0 +1,2 @@
+#!/bin/sh
+# ![the shape of it](diagram.png)
`

// TestFileAssetIsMarkdownOnly pins the endpoint to documents.
//
// Serving a source file as text is a separate feature that arrived later, and
// it shares previewableText with this endpoint: the moment a .go file became
// previewable, the image an /asset request names stopped having to come from
// a document at all, and any string or comment in the tree that spelt out a
// Markdown image became a way to ask the server for that file's bytes.
// Resolving relative images is a Markdown concern, so the gate here is
// IsMarkdown, the same one /content applies before it lists any asset.
func TestFileAssetIsMarkdownOnly(t *testing.T) {
	work := assetTree(t)
	ts, _ := newTestServer(t)

	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: gatedDiff, BaseDir: work}, &added)

	byPath := map[string]string{}
	for _, f := range added.Diff.Files {
		byPath[f.Path()] = f.ID
	}
	base := ts.URL + "/_/api/groups/default/diffs/" + added.Diff.ID + "/files/"

	for _, tt := range []struct {
		path string
		want int
	}{
		{"docs/guide.md", http.StatusOK}, // the document itself, unchanged
		{"docs/evil.go", http.StatusNotFound},
		{"docs/evil.ts", http.StatusNotFound},
		{"docs/evil.json", http.StatusNotFound},
		{"docs/evil.yaml", http.StatusNotFound},
		{"docs/evil.txt", http.StatusNotFound},
		{"docs/evil", http.StatusNotFound},
	} {
		id, ok := byPath[tt.path]
		if !ok {
			t.Errorf("%s is not in the diff: %v", tt.path, byPath)
			continue
		}
		resp, err := http.Get(base + url.PathEscape(id) + "/asset?path=" + url.QueryEscape("docs/diagram.png"))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tt.want {
			t.Errorf("GET asset of %s = %s, want %d; body %q", tt.path, resp.Status, tt.want, body)
			continue
		}
		if tt.want == http.StatusOK && string(body) != string(diagramBytes) {
			t.Errorf("GET asset of %s served %q, want the image", tt.path, body)
		}
		if tt.want == http.StatusNotFound && string(body) == string(diagramBytes) {
			t.Errorf("GET asset of %s served the image behind a 404", tt.path)
		}
	}
}
