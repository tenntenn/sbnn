package history_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/model"
)

func at(minutes int) time.Time {
	return time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute)
}

func group(name string, note string, comments ...*model.Comment) *model.Group {
	return &model.Group{
		Name:       name,
		ReviewedAt: at(60),
		ReviewNote: note,
		Diffs: []*model.Diff{{
			CreatedAt: at(0),
			Files: []*model.File{
				{NewPath: "internal/server/server.go", Additions: 10, Deletions: 2},
				{NewPath: "README.md", Additions: 3, Deletions: 0},
			},
		}},
		Comments: comments,
	}
}

func TestFromGroupKeepsWhatAReviewSaid(t *testing.T) {
	rec := history.FromGroup(group("api", "looks fine",
		&model.Comment{Path: "internal/server/server.go", StartLine: 12, EndLine: 14,
			Body: "rename this\n\n```suggestion\nfunc parse() {\n```"},
		&model.Comment{Path: "README.md", Author: "claude", StartLine: 3, Body: "typo", Resolved: true},
	))

	if rec.Group != "api" || rec.Note != "looks fine" {
		t.Errorf("record = %+v", rec)
	}
	if rec.Files != 2 || rec.Additions != 13 || rec.Deletions != 2 {
		t.Errorf("stats = %d files, +%d -%d", rec.Files, rec.Additions, rec.Deletions)
	}
	if rec.Wait() != time.Hour {
		t.Errorf("wait = %s, want the hour between the diff and the review", rec.Wait())
	}
	if len(rec.Comments) != 2 {
		t.Fatalf("got %d comments", len(rec.Comments))
	}
	if got := rec.Comments[0].Suggestions; len(got) != 1 || got[0] != "func parse() {" {
		t.Errorf("suggestions = %q", got)
	}
	if !rec.Comments[1].Resolved || rec.Comments[1].Author != "claude" {
		t.Errorf("second comment = %+v", rec.Comments[1])
	}
}

func TestAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	// Nothing recorded yet is not an error: there is simply nothing to say.
	if records, err := history.Load(path, history.Filter{}); err != nil || records != nil {
		t.Fatalf("Load on a missing log = %v, %v", records, err)
	}

	for _, name := range []string{"api", "web", "api"} {
		if err := history.Append(path, history.FromGroup(group(name, ""))); err != nil {
			t.Fatal(err)
		}
	}

	all, err := history.Load(path, history.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d records, want 3", len(all))
	}
	only, err := history.Load(path, history.Filter{Group: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 2 {
		t.Errorf("got %d records for one group, want 2", len(only))
	}
	newest, err := history.Load(path, history.Filter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(newest) != 1 || newest[0].Group != "api" {
		t.Errorf("--limit kept %+v, want the newest", newest)
	}
	// A line nothing can read is skipped, not fatal.
	if err := history.Append(path, history.Record{}); err != nil {
		t.Fatal(err)
	}
	if records, err := history.Read(strings.NewReader("{not json}\n"), history.Filter{}); err != nil || len(records) != 0 {
		t.Errorf("Read on a broken line = %v, %v", records, err)
	}
}

func TestCommentsFlattensReviews(t *testing.T) {
	records := []history.Record{
		history.FromGroup(group("api", "",
			&model.Comment{Path: "a.go", StartLine: 1, Body: "first"},
			&model.Comment{Path: "b.md", Author: "claude", StartLine: 2, Body: "second"})),
		history.FromGroup(group("web", "")),
	}
	records[0].Labels = map[string]string{"branch": "main"}

	comments := history.Comments(records)
	if len(comments) != 2 {
		t.Fatalf("got %d comments, want the review without any to add none", len(comments))
	}
	first := comments[0]
	if first.Group != "api" || first.Path != "a.go" || first.Body != "first" {
		t.Errorf("first = %+v", first)
	}
	if first.ReviewedAt != at(60) || first.Labels["branch"] != "main" {
		t.Errorf("first should carry its review's time and labels, got %+v", first)
	}
	if first.Who() != "reviewer" || comments[1].Who() != "claude" {
		t.Errorf("authors = %q, %q", first.Who(), comments[1].Who())
	}
	if first.Extension() != ".go" || comments[1].Extension() != ".md" {
		t.Errorf("extensions = %q, %q", first.Extension(), comments[1].Extension())
	}
}

func TestSummarize(t *testing.T) {
	records := []history.Record{
		history.FromGroup(group("api", "",
			&model.Comment{Path: "internal/server/server.go", Body: "a"},
			&model.Comment{Path: "internal/server/server.go", Body: "b\n\n```suggestion\nx\n```"},
			&model.Comment{Path: "README.md", Author: "claude", Body: "c", Resolved: true},
		)),
		history.FromGroup(group("web", "")),
	}
	s := history.Summarize(records)

	if s.Reviews != 2 || s.Comments != 3 || s.Suggestions != 1 || s.Resolved != 1 {
		t.Errorf("stats = %+v", s)
	}
	if s.Silent != 1 {
		t.Errorf("silent = %d, want the review with nothing to say", s.Silent)
	}
	if s.CommentsPerReview != 1.5 {
		t.Errorf("commentsPerReview = %v, want 1.5", s.CommentsPerReview)
	}
	if s.MedianWait != time.Hour {
		t.Errorf("medianWait = %s", s.MedianWait)
	}
	if len(s.Paths) == 0 || s.Paths[0].Key != "internal/server/server.go" || s.Paths[0].Count != 2 {
		t.Errorf("paths = %+v, want the most commented file first", s.Paths)
	}
	if len(s.Extensions) == 0 || s.Extensions[0].Key != ".go" {
		t.Errorf("extensions = %+v", s.Extensions)
	}
	// The comments written in the browser and the ones from an agent are
	// told apart.
	authors := map[string]int{}
	for _, a := range s.Authors {
		authors[a.Key] = a.Count
	}
	if authors["reviewer"] != 2 || authors["claude"] != 1 {
		t.Errorf("authors = %+v", s.Authors)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		in   string
		want time.Time
	}{
		{"7d", now.AddDate(0, 0, -7)},
		{"36h", now.Add(-36 * time.Hour)},
		{"90m", now.Add(-90 * time.Minute)},
		{"2026-08-01", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{"", time.Time{}},
	} {
		got, err := history.ParseSince(tt.in, now)
		if err != nil {
			t.Errorf("ParseSince(%q) = %v", tt.in, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("ParseSince(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{
		"last tuesday",
		// Only the documented forms are read; anything that merely starts
		// with a number and a "d" is not seven days.
		"7days",
		"7dx",
		"7d3h",
		"7dd",
		"7 d",
		"-7d",
		"0d",
	} {
		if got, err := history.ParseSince(in, now); err == nil {
			t.Errorf("ParseSince(%q) = %s, want an error", in, got)
		}
	}
}
