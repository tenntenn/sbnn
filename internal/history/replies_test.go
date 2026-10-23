package history_test

import (
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/model"
)

// A record keeps a reply right after the comment it answers, naming it, so
// that a reader of the log can tell who was answering whom.
func TestFromGroupKeepsRepliesWithTheirComment(t *testing.T) {
	rec := history.FromGroup(group("api", "",
		&model.Comment{ID: "c1", Path: "README.md", StartLine: 3, Body: "why?", Question: true},
		&model.Comment{ID: "c2", Path: "README.md", StartLine: 9, Body: "typo"},
		&model.Comment{ID: "c3", ReplyTo: "c1", Path: "README.md", StartLine: 3, Author: "claude",
			Body: "because\n\n```suggestion\nx\n```"},
	))
	var got []string
	for _, c := range rec.Comments {
		got = append(got, c.ID+">"+c.ReplyTo)
	}
	if want := "c1> c3>c1 c2>"; strings.Join(got, " ") != want {
		t.Errorf("comments = %q, want %q", strings.Join(got, " "), want)
	}
	if rec.Threads() != 2 {
		t.Errorf("Threads() = %d, want 2", rec.Threads())
	}

	s := history.Summarize([]history.Record{rec})
	if s.Comments != 2 || s.Replies != 1 || s.Suggestions != 1 {
		t.Errorf("stats = %d comments, %d replies, %d suggestions; want 2, 1, 1", s.Comments, s.Replies, s.Suggestions)
	}
	for _, a := range s.Authors {
		if a.Key == "claude" {
			t.Errorf("authors = %+v: a reply is not a comment the review drew", s.Authors)
		}
	}
}

// Silent counts the reviews that opened no thread, which a review holding a
// comment and a reply to it did.
func TestSilentCountsThreads(t *testing.T) {
	quiet := history.FromGroup(group("api", ""))
	talking := history.FromGroup(group("api", "", &model.Comment{ID: "c1", Body: "x"},
		&model.Comment{ID: "c2", ReplyTo: "c1", Body: "y"}))
	if s := history.Summarize([]history.Record{quiet, talking}); s.Silent != 1 {
		t.Errorf("silent = %d, want 1", s.Silent)
	}
}
