package server

// Tests for the line range a comment is stored with: a range that runs past
// the end of the diff is clamped to the last line the diff really shows,
// whichever shape of the request carried it. They sit in their own file
// rather than at the end of server_test.go so that other work on
// internal/server does not have to land in the same place.

import (
	"net/http"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

// A comment was accepted as soon as its snippet was non-empty, so a range
// starting inside a hunk and ending past it was stored with an endLine the
// diff never showed. The page draws a comment on the row for its endLine,
// so such a comment was drawn on no row and the reviewer could not see it,
// while sbnn comments claimed a range like "2-900".
//
// Every case runs through both shapes of the request. The page sends
// diffId, fileId and a snippet it worked out itself; an agent on the
// command line sends only a path. The clamp lived in the path branch
// alone, so the shape the page actually uses kept storing endLine=900.
func TestHandleAddCommentClampsEndLineToTheDiff(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)

	// README.md covers new lines 1-3 and old lines 1-2; docs/new.md is a
	// new file covering new lines 1-2.
	cases := []struct {
		name      string
		path      string
		side      string
		startLine int
		endLine   int
		wantEnd   int
	}{
		{"past the end of the only hunk", "README.md", "new", 2, 900, 3},
		{"one line past the end", "README.md", "new", 2, 4, 3},
		{"exactly the last line stays", "README.md", "new", 2, 3, 3},
		{"a range well inside stays", "README.md", "new", 1, 2, 2},
		{"a single line stays", "README.md", "new", 3, 3, 3},
		{"the old side clamps to the old numbering", "README.md", "old", 1, 50, 2},
		{"a new file clamps too", "docs/new.md", "new", 1, 99, 2},
	}
	shapes := []struct {
		name    string
		request func(path, side string, start, end int, fileID string) AddCommentRequest
	}{{
		// What an agent on the command line sends: a path, nothing else.
		name: "by path",
		request: func(path, side string, start, end int, _ string) AddCommentRequest {
			return AddCommentRequest{Path: path, Side: side, StartLine: start, EndLine: end, Body: "range"}
		},
	}, {
		// What the page sends: the file named outright, and a snippet it
		// captured itself, so the server never computes one.
		name: "by fileId, with the snippet the page captured",
		request: func(path, side string, start, end int, fileID string) AddCommentRequest {
			return AddCommentRequest{
				DiffID: added.Diff.ID, FileID: fileID, Path: path, Side: side,
				StartLine: start, EndLine: end, Body: "range", Snippet: "captured by the page",
			}
		},
	}, {
		// The same shape with no snippet: it is the file, not the
		// snippet, that says where the range stops.
		name: "by fileId, without a snippet",
		request: func(path, side string, start, end int, fileID string) AddCommentRequest {
			return AddCommentRequest{
				DiffID: added.Diff.ID, FileID: fileID, Path: path, Side: side,
				StartLine: start, EndLine: end, Body: "range",
			}
		},
	}}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					fileID, ok := fileIDForPath(added.Diff, tc.path)
					if !ok {
						t.Fatalf("the sample diff has no %s", tc.path)
					}
					var comment model.Comment
					resp := postJSON(t, ts.URL+"/_/api/groups/default/comments",
						shape.request(tc.path, tc.side, tc.startLine, tc.endLine, fileID), &comment)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("status = %s, want 200", resp.Status)
					}
					// The status was 200 before the fix too. What was
					// wrong was the range that got stored.
					if comment.StartLine != tc.startLine {
						t.Errorf("stored startLine = %d, want %d", comment.StartLine, tc.startLine)
					}
					if comment.EndLine != tc.wantEnd {
						t.Errorf("stored endLine = %d, want %d", comment.EndLine, tc.wantEnd)
					}
					if comment.Snippet == "" {
						t.Error("stored an empty snippet")
					}
					if comment.FileID != fileID {
						t.Errorf("stored fileId = %q, want %q", comment.FileID, fileID)
					}
				})
			}
		})
	}

	// No stored comment may claim a line the page has no row for, however
	// it was sent.
	var comments []*model.Comment
	getJSON(t, ts.URL+"/_/api/groups/default/comments", &comments)
	if len(comments) != len(cases)*len(shapes) {
		t.Fatalf("stored %d comment(s), want %d", len(comments), len(cases)*len(shapes))
	}
	for _, c := range comments {
		f, ok := findFileForComment(added.Diff, c)
		if !ok {
			t.Fatalf("comment on no file of the diff: %+v", c)
		}
		if last := lastCoveredLine(f, c.Side, c.StartLine, c.EndLine); last != c.EndLine {
			t.Errorf("comment %s ends at line %d, but the diff stops at %d", c.ID, c.EndLine, last)
		}
	}
}

// TestHandleAddCommentByFileIDFillsTheSnippet pins the other half of what
// the fileId branch used to skip: a client that names the file but sends
// no snippet was stored with an empty one, so the comment carried no code
// with it into sbnn comments or the review prompt.
func TestHandleAddCommentByFileIDFillsTheSnippet(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	fileID, ok := fileIDForPath(added.Diff, "README.md")
	if !ok {
		t.Fatal("the sample diff has no README.md")
	}

	var comment model.Comment
	resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: fileID, Path: "README.md", Side: "new",
		StartLine: 2, EndLine: 3, Body: "range",
	}, &comment)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s, want 200", resp.Status)
	}
	want := diff.Snippet(fileOf(t, added.Diff, fileID), "new", 2, 3)
	if want == "" {
		t.Fatal("the sample diff gives no snippet for README.md:2-3")
	}
	if comment.Snippet != want {
		t.Errorf("stored snippet = %q, want the one the path shape gets, %q", comment.Snippet, want)
	}

	// A snippet the client did send is still its own.
	var kept model.Comment
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: fileID, Path: "README.md", Side: "new",
		StartLine: 2, EndLine: 3, Body: "range", Snippet: "captured by the page",
	}, &kept)
	if kept.Snippet != "captured by the page" {
		t.Errorf("stored snippet = %q, want the one the client sent", kept.Snippet)
	}
}

// fileIDForPath returns the id the diff gave the file at path.
func fileIDForPath(d *model.Diff, path string) (string, bool) {
	for _, f := range d.Files {
		if f.Path() == path || f.OldPath == path {
			return f.ID, true
		}
	}
	return "", false
}

// fileOf returns the file of the diff with this id.
func fileOf(t *testing.T, d *model.Diff, fileID string) *model.File {
	t.Helper()
	for _, f := range d.Files {
		if f.ID == fileID {
			return f
		}
	}
	t.Fatalf("the diff has no file %q", fileID)
	return nil
}

// findFileForComment returns the file of the diff a comment points at.
func findFileForComment(d *model.Diff, c *model.Comment) (*model.File, bool) {
	for _, f := range d.Files {
		if f.ID == c.FileID {
			return f, true
		}
	}
	return nil, false
}
