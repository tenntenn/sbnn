package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// replyServer is a server holding sampleDiff and three comments on README.md:
// c1 on new lines 2-3, c2 on old line 2, and c3 on the file as a whole.
func replyServer(t *testing.T) (url string, srv *Server) {
	t.Helper()
	ts, srv := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]
	for _, req := range []AddCommentRequest{
		{DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: "new", StartLine: 2, EndLine: 3, Body: "rephrase?", Question: true},
		{DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Side: "old", StartLine: 2, EndLine: 2, Body: "why drop it?"},
		{DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(), Body: "about the file"},
	} {
		if resp := postJSON(t, ts.URL+"/_/api/groups/default/comments", req, nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("adding %q: %s", req.Body, resp.Status)
		}
	}
	return ts.URL, srv
}

func TestPostAReply(t *testing.T) {
	for _, tt := range []struct {
		name       string
		req        AddCommentRequest
		wantStatus int
		// wantInError is part of the message a refusal must carry.
		wantInError string
	}{
		{name: "by id alone", req: AddCommentRequest{ReplyTo: "c1", Body: "yes"}, wantStatus: http.StatusOK},
		{name: "naming the thread's own place", req: AddCommentRequest{ReplyTo: "c1", Path: "README.md", Side: "new", StartLine: 2, EndLine: 3, Body: "yes"}, wantStatus: http.StatusOK},
		{name: "a suggestion on new lines", req: AddCommentRequest{ReplyTo: "c1", Suggestion: "better line", Body: "like this"}, wantStatus: http.StatusOK},
		{name: "to a comment that is not there", req: AddCommentRequest{ReplyTo: "c99", Body: "yes"}, wantStatus: http.StatusNotFound, wantInError: "c99"},
		{name: "naming another path", req: AddCommentRequest{ReplyTo: "c1", Path: "docs/new.md", Body: "yes"}, wantStatus: http.StatusBadRequest, wantInError: "docs/new.md"},
		{name: "naming other lines", req: AddCommentRequest{ReplyTo: "c1", Path: "README.md", StartLine: 3, Body: "yes"}, wantStatus: http.StatusBadRequest, wantInError: "line 3"},
		{name: "naming the other side", req: AddCommentRequest{ReplyTo: "c1", Side: "old", Body: "yes"}, wantStatus: http.StatusBadRequest, wantInError: "old side"},
		{name: "naming another diff", req: AddCommentRequest{ReplyTo: "c1", DiffID: "d9", Body: "yes"}, wantStatus: http.StatusBadRequest, wantInError: "d9"},
		{name: "as a question", req: AddCommentRequest{ReplyTo: "c1", Body: "really?", Question: true}, wantStatus: http.StatusBadRequest, wantInError: "question"},
		{name: "with nothing to say", req: AddCommentRequest{ReplyTo: "c1"}, wantStatus: http.StatusBadRequest},
		{name: "a suggestion on old lines", req: AddCommentRequest{ReplyTo: "c2", Suggestion: "x"}, wantStatus: http.StatusBadRequest, wantInError: "old"},
		{name: "a suggestion on the whole file", req: AddCommentRequest{ReplyTo: "c3", Suggestion: "x"}, wantStatus: http.StatusBadRequest, wantInError: "whole file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := replyServer(t)
			resp := postJSON(t, url+"/_/api/groups/default/comments", tt.req, nil)
			body := readBody(t, resp)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %s (%s), want %d", resp.Status, body, tt.wantStatus)
			}
			if tt.wantInError != "" && !strings.Contains(body, tt.wantInError) {
				t.Errorf("error %q does not mention %q", body, tt.wantInError)
			}
			if resp.StatusCode != http.StatusOK {
				return
			}
			var comments []*model.Comment
			getJSON(t, url+"/_/api/groups/default/comments", &comments)
			reply := comments[len(comments)-1]
			if reply.ReplyTo != "c1" || reply.StartLine != 2 || reply.EndLine != 3 || reply.Side != "new" || reply.Snippet != comments[0].Snippet {
				t.Errorf("reply = %+v, want it on c1's lines", reply)
			}
		})
	}
}

// readBody reads what a response said, for an error message.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func patch(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(body))
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

func TestPatchingAReply(t *testing.T) {
	url, _ := replyServer(t)
	postJSON(t, url+"/_/api/groups/default/comments", AddCommentRequest{ReplyTo: "c1", Body: "yes"}, nil)

	if resp := patch(t, url+"/_/api/groups/default/comments/c4", `{"question":true}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("making a reply a question: %s, want 400", resp.Status)
	}
	if resp := patch(t, url+"/_/api/groups/default/comments/c4", `{"resolved":true}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("resolving from a reply: %s", resp.Status)
	}
	var g model.Group
	getJSON(t, url+"/_/api/groups/default", &g)
	for _, c := range g.Comments {
		if want := c.ID == "c1" || c.ID == "c4"; c.Resolved != want {
			t.Errorf("%s resolved = %v, want %v", c.ID, c.Resolved, want)
		}
	}

	var st Status
	getJSON(t, url+"/_/api/status", &st)
	if sum := st.Groups[0]; sum.Comments != 3 || sum.Unresolved != 2 {
		t.Errorf("status counts %d comments, %d open; want 3 threads, 2 open", sum.Comments, sum.Unresolved)
	}
}

func TestDeletingAThreadOverHTTP(t *testing.T) {
	url, _ := replyServer(t)
	postJSON(t, url+"/_/api/groups/default/comments", AddCommentRequest{ReplyTo: "c1", Body: "yes"}, nil)
	req, err := http.NewRequest(http.MethodDelete, url+"/_/api/groups/default/comments/c1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var comments []*model.Comment
	getJSON(t, url+"/_/api/groups/default/comments", &comments)
	for _, c := range comments {
		if c.ID == "c1" || c.ReplyTo == "c1" {
			t.Errorf("%s is still there after its thread was deleted", c.ID)
		}
	}
}

// A reply comes out of the prompt under its comment, with the comment's ID
// to answer it by.
func TestThePromptCarriesReplies(t *testing.T) {
	url, _ := replyServer(t)
	postJSON(t, url+"/_/api/groups/default/comments", AddCommentRequest{ReplyTo: "c1", Author: "claude", Body: "It reads better."}, nil)
	got := getText(t, url+"/_/api/groups/default/prompt")
	for _, want := range []string{
		"3 comment(s) to address.",
		"Comment ID: c1",
		"This one is a question, and it has been answered below.",
		"### Reply from claude\n\nIt reads better.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not contain %q:\n%s", want, got)
		}
	}
}

// A hook is told how many threads are open, not how many entries the event
// carries: the replies travel in the event with their comment.
func TestHookEnvCountsThreads(t *testing.T) {
	s := newHookServer(t)
	env := envMap(s.hookEnv(ReviewEvent{
		Group: "api",
		Comments: []*model.Comment{
			{ID: "c1", Body: "why?"},
			{ID: "c2", ReplyTo: "c1", Body: "because"},
			{ID: "c3", Body: "typo"},
		},
	}))
	if got := env["SBNN_COMMENTS"]; got != "2" {
		t.Errorf("SBNN_COMMENTS = %q, want 2", got)
	}
}
