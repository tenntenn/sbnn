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
