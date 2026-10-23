package server

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// threadFixture is a group holding two diffs and, on the first, a thread of
// a comment and one reply plus a comment on its own; on the second, one more
// comment. It returns the store and the ids in the order they were written:
// c1 (root), c2 (reply to c1), c3 (alone), c4 (on the second diff).
func threadFixture(t *testing.T, path string) *Store {
	t.Helper()
	s := NewStore(path)
	d1 := s.AddDiff(DefaultGroup, &model.Diff{Files: []*model.File{{ID: "f1", NewPath: "a.go"}}})
	d2 := s.AddDiff(DefaultGroup, &model.Diff{Files: []*model.File{{ID: "f1", NewPath: "b.go"}}})
	for _, c := range []*model.Comment{
		{DiffID: d1.ID, FileID: "f1", Path: "a.go", Side: "new", StartLine: 3, EndLine: 5, Snippet: "x := 1", Body: "why?", Question: true},
		{ReplyTo: "c1", Author: "claude", Body: "because"},
		{DiffID: d1.ID, FileID: "f1", Path: "a.go", Side: "new", StartLine: 9, EndLine: 9, Body: "typo"},
		{DiffID: d2.ID, FileID: "f1", Path: "b.go", Side: "old", StartLine: 1, EndLine: 1, Body: "gone"},
	} {
		c.Group = DefaultGroup
		if _, err := s.AddComment(c); err != nil {
			t.Fatalf("AddComment(%+v) = %v", c, err)
		}
	}
	return s
}

// ids lists the comments of the default group as id:resolved, in order.
func ids(t *testing.T, s *Store) []string {
	t.Helper()
	comments, _ := s.Comments(DefaultGroup)
	out := make([]string, 0, len(comments))
	for _, c := range comments {
		state := "open"
		if c.Resolved {
			state = "resolved"
		}
		out = append(out, c.ID+":"+state)
	}
	return out
}

func TestAReplyTakesTheAnchorOfItsThread(t *testing.T) {
	for _, tt := range []struct {
		name    string
		replyTo string
		// The request may carry a place of its own, and a question, and
		// both are overruled by the thread.
		given       model.Comment
		wantReplyTo string
	}{
		{name: "to the first comment", replyTo: "c1", wantReplyTo: "c1"},
		{name: "to a reply joins the same thread", replyTo: "c2", wantReplyTo: "c1"},
		{
			name: "a place and a question it was given are not kept", replyTo: "c1", wantReplyTo: "c1",
			given: model.Comment{DiffID: "d2", Path: "b.go", Side: "old", StartLine: 40, EndLine: 41, Snippet: "y", Question: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := threadFixture(t, "")
			c := tt.given
			c.Group, c.ReplyTo, c.Body = DefaultGroup, tt.replyTo, "more"
			got, err := s.AddComment(&c)
			if err != nil {
				t.Fatalf("AddComment() = %v", err)
			}
			root, _ := s.Comment(DefaultGroup, "c1")
			if got.ReplyTo != tt.wantReplyTo {
				t.Errorf("replyTo = %q, want %q", got.ReplyTo, tt.wantReplyTo)
			}
			if got.DiffID != root.DiffID || got.FileID != root.FileID || got.Path != root.Path || got.Side != root.Side ||
				got.StartLine != root.StartLine || got.EndLine != root.EndLine || got.Snippet != root.Snippet {
				t.Errorf("reply anchored at %s %s %s:%d-%d (%s) %q, want the thread's %s %s %s:%d-%d (%s) %q",
					got.DiffID, got.FileID, got.Path, got.StartLine, got.EndLine, got.Side, got.Snippet,
					root.DiffID, root.FileID, root.Path, root.StartLine, root.EndLine, root.Side, root.Snippet)
			}
			if got.Question {
				t.Error("a reply was stored as a question")
			}
		})
	}
}

func TestAReplyToNothingIsRefused(t *testing.T) {
	s := threadFixture(t, "")
	s.AddDiff("api", &model.Diff{Files: []*model.File{{ID: "f1", NewPath: "c.go"}}})
	if _, err := s.AddComment(&model.Comment{Group: "api", DiffID: "d3", FileID: "f1", Path: "c.go", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, group, replyTo string
	}{
		{"an unknown id", DefaultGroup, "c99"},
		{"a comment of another group", DefaultGroup, "c5"},
		{"from the other group back", "api", "c1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.AddComment(&model.Comment{Group: tt.group, ReplyTo: tt.replyTo, Body: "hi"})
			if !errors.Is(err, ErrNoComment) {
				t.Errorf("AddComment(reply to %s in %s) = %v, want ErrNoComment", tt.replyTo, tt.group, err)
			}
		})
	}
}

// resolveStep resolves or reopens the comment id.
type resolveStep struct {
	id       string
	resolved bool
}

func TestAThreadIsResolvedWhole(t *testing.T) {
	for _, tt := range []struct {
		name string
		// steps are applied in order.
		steps []resolveStep
		want  []string
	}{
		{
			name:  "resolving the first comment resolves its replies",
			steps: []resolveStep{{"c1", true}},
			want:  []string{"c1:resolved", "c2:resolved", "c3:open", "c4:open"},
		},
		{
			name:  "resolving a reply resolves its thread",
			steps: []resolveStep{{"c2", true}},
			want:  []string{"c1:resolved", "c2:resolved", "c3:open", "c4:open"},
		},
		{
			name:  "reopening from a reply reopens the thread",
			steps: []resolveStep{{"c1", true}, {"c2", false}},
			want:  []string{"c1:open", "c2:open", "c3:open", "c4:open"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			s := threadFixture(t, path)
			for _, step := range tt.steps {
				resolved := step.resolved
				if _, ok := s.UpdateComment(DefaultGroup, step.id, CommentPatch{Resolved: &resolved}); !ok {
					t.Fatalf("UpdateComment(%s) found nothing", step.id)
				}
			}
			if got := ids(t, s); !slices.Equal(got, tt.want) {
				t.Errorf("comments = %v, want %v", got, tt.want)
			}
			// The log holds one record per resolve, of the first comment
			// alone; replaying it has to reach the same state.
			if got := ids(t, reloaded(t, path)); !slices.Equal(got, tt.want) {
				t.Errorf("after a restart, comments = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAReplyAddedToAResolvedThreadIsResolved(t *testing.T) {
	s := threadFixture(t, "")
	resolved := true
	s.UpdateComment(DefaultGroup, "c1", CommentPatch{Resolved: &resolved})
	got, err := s.AddComment(&model.Comment{Group: DefaultGroup, ReplyTo: "c1", Body: "late"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resolved {
		t.Error("a reply to a resolved thread is open, so the thread would count as open again")
	}
}

func TestAQuestionStaysOnTheFirstComment(t *testing.T) {
	s := threadFixture(t, "")
	yes := true
	if c, _ := s.UpdateComment(DefaultGroup, "c2", CommentPatch{Question: &yes}); c.Question {
		t.Error("a reply became a question")
	}
}

func TestDeletingComments(t *testing.T) {
	for _, tt := range []struct {
		name string
		do   func(s *Store)
		want []string
	}{
		{
			name: "deleting the first comment deletes its replies",
			do:   func(s *Store) { s.DeleteComment(DefaultGroup, "c1") },
			want: []string{"c3:open", "c4:open"},
		},
		{
			name: "deleting a reply deletes only the reply",
			do:   func(s *Store) { s.DeleteComment(DefaultGroup, "c2") },
			want: []string{"c1:open", "c3:open", "c4:open"},
		},
		{
			name: "clearing the resolved keeps an open thread whole",
			do: func(s *Store) {
				yes := true
				s.UpdateComment(DefaultGroup, "c3", CommentPatch{Resolved: &yes})
				s.ClearComments(DefaultGroup, true)
			},
			want: []string{"c1:open", "c2:open", "c4:open"},
		},
		{
			name: "clearing the resolved takes a resolved thread whole",
			do: func(s *Store) {
				yes := true
				s.UpdateComment(DefaultGroup, "c2", CommentPatch{Resolved: &yes})
				s.ClearComments(DefaultGroup, true)
			},
			want: []string{"c3:open", "c4:open"},
		},
		{
			name: "deleting a diff deletes the threads on it",
			do:   func(s *Store) { s.DeleteDiff(DefaultGroup, "d1") },
			want: []string{"c4:open"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			s := threadFixture(t, path)
			tt.do(s)
			if got := ids(t, s); !slices.Equal(got, tt.want) {
				t.Errorf("comments = %v, want %v", got, tt.want)
			}
			if got := ids(t, reloaded(t, path)); !slices.Equal(got, tt.want) {
				t.Errorf("after a restart, comments = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClearCommentsCountsThreads(t *testing.T) {
	s := threadFixture(t, "")
	if got := s.ClearComments(DefaultGroup, false); got != 3 {
		t.Errorf("ClearComments() = %d, want 3: a comment and its reply are one thread", got)
	}
}

func TestSummaryCountsThreads(t *testing.T) {
	s := threadFixture(t, "")
	yes := true
	s.UpdateComment(DefaultGroup, "c3", CommentPatch{Resolved: &yes})
	sum := s.Summary("http://x")[0]
	if sum.Comments != 3 || sum.Unresolved != 2 {
		t.Errorf("summary = %d comments, %d unresolved; want 3 and 2", sum.Comments, sum.Unresolved)
	}
}

// A session file written before replies existed has no replyTo anywhere, and
// every comment in it opens a thread of its own.
func TestASessionFileFromBeforeRepliesLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{"version":2,"seq":3,"groups":[{"name":"default","diffs":[{"id":"d1","title":"diff 1"}],` +
		`"comments":[{"id":"c1","group":"default","diffId":"d1","path":"a.go","side":"new","startLine":3,"endLine":3,"body":"one","resolved":false},` +
		`{"id":"c2","group":"default","diffId":"d1","path":"a.go","side":"new","startLine":3,"endLine":3,"body":"two","resolved":true}]}]}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s := reloaded(t, path)
	if got, want := ids(t, s), []string{"c1:open", "c2:resolved"}; !slices.Equal(got, want) {
		t.Errorf("comments = %v, want %v", got, want)
	}
	g, _ := s.Group(DefaultGroup)
	if n := len(model.Threads(g.Comments)); n != 2 {
		t.Errorf("threads = %d, want 2", n)
	}
}
