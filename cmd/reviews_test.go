package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/history"
)

// stream is two comments with everything the text columns draw from.
func stream() []history.CommentRecord {
	return []history.CommentRecord{
		{
			Group:      "api",
			ReviewedAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.Local),
			Labels:     map[string]string{"branch": "main"},
			Comment: history.Comment{
				Path: "internal/server/server.go", Side: "new",
				StartLine: 12, EndLine: 14,
				Body: "rename this\nand a second line that must not leak",
			},
		},
		{
			Group:      "web",
			ReviewedAt: time.Date(2026, 8, 16, 10, 0, 0, 0, time.Local),
			Comment: history.Comment{
				Path: "README.md", Author: "claude", Side: "new",
				StartLine: 3, EndLine: 3,
				Body:        "a\ttab becomes a space",
				Suggestions: []string{"fixed"},
			},
		},
	}
}

// The tab-separated text form is piped into cut and awk, which makes its
// columns a contract: date, group, path:lines, author, first line of the
// body. Changing any of them breaks pipelines that cannot be seen from
// here, so this test fails on any change, deliberate or not.
func TestCommentStreamTextColumnsAreAContract(t *testing.T) {
	var buf bytes.Buffer
	if err := printCommentStream(&buf, stream(), "text"); err != nil {
		t.Fatal(err)
	}
	want := "" +
		"2026-08-15\tapi\tinternal/server/server.go:12-14\treviewer\trename this\n" +
		"2026-08-16\tweb\tREADME.md:3\tclaude\ta tab becomes a space\n"
	if got := buf.String(); got != want {
		t.Errorf("the text columns changed:\ngot  %q\nwant %q", got, want)
	}
}

// jsonl means one JSON object per line; an indented object is not jsonl.
func TestCommentStreamJSONLIsOneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	if err := printCommentStream(&buf, stream(), "jsonl"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d line(s) for 2 comments:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line is not a JSON object: %q: %v", line, err)
		}
		// The record has to stand on its own: the fields jq joins on
		// must be on every line, flat, not nested under the comment.
		for _, key := range []string{"group", "reviewedAt", "path", "startLine", "body"} {
			if _, ok := rec[key]; !ok {
				t.Errorf("line lacks %q: %s", key, line)
			}
		}
	}
}

// reviewed is two reviews with enough on them for a summary to be about
// something.
func reviewed() []history.Record {
	return []history.Record{
		{
			Group:      "api",
			ReviewedAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.Local),
			Files:      2, Additions: 10, Deletions: 1,
			Comments: []history.Comment{{Path: "a.go", Side: "new", StartLine: 1, EndLine: 1, Body: "x"}},
		},
		{
			Group:      "web",
			ReviewedAt: time.Date(2026, 8, 16, 10, 0, 0, 0, time.Local),
			Files:      1, Additions: 3, Deletions: 3,
		},
	}
}

// --stats says "print what the reviews say together", which is a promise
// about what is printed, not about the format it is printed in. It used to
// be read only inside the text branch, so json and jsonl silently printed
// the per-review stream instead and nothing said the flag had been ignored.
func TestStatsIsHonouredInEveryFormat(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := printReviewStats(&buf, reviewed(), "json"); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("not a JSON object: %v\n%s", err, buf.String())
		}
		// The aggregate and only the aggregate: no per-review list, and
		// no "stats" wrapper to reach through.
		if _, ok := got["stats"]; ok {
			t.Errorf("--stats still wraps the aggregate: %s", buf.String())
		}
		if n, ok := got["reviews"].(float64); !ok || n != 2 {
			t.Errorf(`"reviews" is not the review count: %v`, got["reviews"])
		}
		if n, ok := got["comments"].(float64); !ok || n != 1 {
			t.Errorf(`"comments" is not the comment count: %v`, got["comments"])
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		var buf bytes.Buffer
		if err := printReviewStats(&buf, reviewed(), "jsonl"); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("the aggregate is one thing, got %d line(s):\n%s", len(lines), buf.String())
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("line is not a JSON object: %q: %v", lines[0], err)
		}
		if n, ok := got["reviews"].(float64); !ok || n != 2 {
			t.Errorf(`"reviews" is not the review count: %v`, got["reviews"])
		}
	})

	t.Run("text", func(t *testing.T) {
		var buf bytes.Buffer
		if err := printReviewStats(&buf, reviewed(), "text"); err != nil {
			t.Fatal(err)
		}
		if want := "2 review(s), 1 comment(s)"; !strings.Contains(buf.String(), want) {
			t.Errorf("the text aggregate lost its first line:\nwant %q in\n%s", want, buf.String())
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if err := printReviewStats(&bytes.Buffer{}, reviewed(), "yaml"); err == nil {
			t.Error("an unknown format is an error, not silence")
		}
	})
}

// An empty log says so rather than printing a summary of nothing.
func TestStatsOnAnEmptyLogSaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := printReviewStats(&buf, nil, "text"); err != nil {
		t.Fatal(err)
	}
	if want := "no review has been submitted yet\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
