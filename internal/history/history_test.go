package history_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// TestReadSkipsWhatItCannotUse pins the promise the skip comment makes: a
// line the reader cannot use costs that line and nothing else. An
// over-long one used to take the rest of the log with it, because
// bufio.Scanner stops the whole scan at the first line past its buffer.
func TestReadSkipsWhatItCannotUse(t *testing.T) {
	huge := `{"group":"generated","note":"` + strings.Repeat("x", history.MaxRecordBytes) + `"}`
	first := `{"group":"api","reviewedAt":"2026-08-16T10:00:00Z"}`
	last := `{"group":"web","reviewedAt":"2026-08-16T11:00:00Z"}`

	for _, tt := range []struct {
		name    string
		log     string
		want    []string
		skipped history.Skipped
	}{
		{
			name: "an over-long line in the middle",
			log:  first + "\n" + huge + "\n" + last + "\n",
			want: []string{"api", "web"},
			// This is the case that used to return nothing at all.
			skipped: history.Skipped{Long: 1},
		},
		{
			name:    "an over-long line first",
			log:     huge + "\n" + first + "\n",
			want:    []string{"api"},
			skipped: history.Skipped{Long: 1},
		},
		{
			name:    "an over-long line last, with no newline after it",
			log:     first + "\n" + huge,
			want:    []string{"api"},
			skipped: history.Skipped{Long: 1},
		},
		{
			name:    "two over-long lines back to back",
			log:     huge + "\n" + huge + "\n" + last + "\n",
			want:    []string{"web"},
			skipped: history.Skipped{Long: 2},
		},
		{
			name:    "broken and over-long together",
			log:     "{not json}\n" + first + "\n" + huge + "\n" + last + "\n",
			want:    []string{"api", "web"},
			skipped: history.Skipped{Broken: 1, Long: 1},
		},
		{
			name: "blank lines are not a complaint",
			log:  "\n" + first + "\n   \n" + last + "\n\n",
			want: []string{"api", "web"},
		},
		{
			name: "a record right at the limit is still a record",
			log: `{"group":"big","note":"` +
				strings.Repeat("x", history.MaxRecordBytes-len(`{"group":"big","note":""}`)) +
				`"}` + "\n",
			want: []string{"big"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			records, skipped, err := history.ReadSkipped(strings.NewReader(tt.log), history.Filter{})
			if err != nil {
				t.Fatalf("ReadSkipped = %v", err)
			}
			var got []string
			for _, rec := range records {
				got = append(got, rec.Group)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("read %v, want %v", got, tt.want)
			}
			if skipped != tt.skipped {
				t.Errorf("skipped %+v, want %+v", skipped, tt.skipped)
			}
			if skipped.Any() != (tt.skipped != history.Skipped{}) {
				t.Errorf("Any() = %v for %+v", skipped.Any(), skipped)
			}
			// Read is the same read, without the count.
			plain, err := history.Read(strings.NewReader(tt.log), history.Filter{})
			if err != nil {
				t.Fatalf("Read = %v", err)
			}
			if len(plain) != len(records) {
				t.Errorf("Read got %d record(s), ReadSkipped %d", len(plain), len(records))
			}
		})
	}
}

func TestSkippedString(t *testing.T) {
	for _, tt := range []struct {
		in   history.Skipped
		want string
	}{
		{history.Skipped{}, ""},
		{history.Skipped{Broken: 2}, "2 unreadable line(s)"},
		{history.Skipped{Long: 1}, "1 line(s) over 8 MiB"},
		{history.Skipped{Broken: 1, Long: 3}, "1 unreadable line(s), 3 line(s) over 8 MiB"},
	} {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Skipped%+v.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestLoadSkipsOverLongLine is the same promise through the file path, the
// one "sbnn reviews" actually takes.
func TestLoadSkipsOverLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	if err := history.Append(path, history.Record{Group: "api", ReviewedAt: at(0)}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"group":"generated","note":"` +
		strings.Repeat("x", history.MaxRecordBytes) + `"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := history.Append(path, history.Record{Group: "web", ReviewedAt: at(60)}); err != nil {
		t.Fatal(err)
	}

	records, skipped, err := history.LoadSkipped(path, history.Filter{})
	if err != nil {
		t.Fatalf("LoadSkipped = %v", err)
	}
	if len(records) != 2 || records[0].Group != "api" || records[1].Group != "web" {
		t.Errorf("LoadSkipped kept %+v, want the two readable records", records)
	}
	if want := (history.Skipped{Long: 1}); skipped != want {
		t.Errorf("skipped %+v, want %+v", skipped, want)
	}
	if got := skipped.String(); got != "1 line(s) over 8 MiB" {
		t.Errorf("skipped.String() = %q", got)
	}
}

// TestAppendKeepsRecordsWholeUnderConcurrency holds Append to what the log
// format needs: one line per record, whole, however many writers there
// are. HistoryFile is one file for the machine while a session file is one
// per port, so several servers writing at once is the ordinary case.
//
// This is the end the log is judged by, not the test that catches the lock
// going away: on Linux a write(2) to a regular file is serialised by the
// inode lock, so these records come back whole even with no lock of ours,
// and the test passes either way. TestAppendWaitsForTheAppendLock is the
// one that fails when Append stops locking.
func TestAppendKeepsRecordsWholeUnderConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	const writers, each = 8, 25
	note := strings.Repeat("x", 256<<10)

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := range writers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range each {
				rec := history.Record{
					Group:      fmt.Sprintf("g%d", w),
					ReviewedAt: at(i),
					Note:       note,
				}
				if err := history.Append(path, rec); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Append = %v", err)
	}

	// Every line has to be a record: a line that does not parse is a
	// review Read drops on the floor, silently.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != writers*each {
		t.Errorf("log has %d line(s), want %d", len(lines), writers*each)
	}
	for i, line := range lines {
		var rec history.Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d of %d does not parse (%d bytes): %v",
				i+1, len(lines), len(line), err)
		}
	}

	records, err := history.Load(path, history.Filter{})
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if len(records) != writers*each {
		t.Fatalf("Load read %d record(s), want %d", len(records), writers*each)
	}
	perGroup := map[string]int{}
	for _, rec := range records {
		perGroup[rec.Group]++
		if len(rec.Note) != len(note) {
			t.Fatalf("a record of %s came back with a note of %d bytes, want %d",
				rec.Group, len(rec.Note), len(note))
		}
	}
	for w := range writers {
		if got := perGroup[fmt.Sprintf("g%d", w)]; got != each {
			t.Errorf("g%d wrote %d record(s), want %d", w, got, each)
		}
	}
}

// TestAppendStillAppends is the plain case, so that taking a lock has not
// turned an append into something else.
func TestAppendStillAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	for _, name := range []string{"api", "web", "cli"} {
		if err := history.Append(path, history.Record{Group: name, ReviewedAt: at(0)}); err != nil {
			t.Fatalf("Append(%s) = %v", name, err)
		}
	}
	records, err := history.Load(path, history.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, rec := range records {
		got = append(got, rec.Group)
	}
	if strings.Join(got, ",") != "api,web,cli" {
		t.Errorf("log reads %v, want the three in the order they were written", got)
	}
	// An empty path is still the way to say "keep no log".
	if err := history.Append("", history.Record{Group: "api"}); err != nil {
		t.Errorf("Append with no path = %v", err)
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

// TestParseSinceDateIsLocal pins the bare-date form to the reviewer's own
// zone. time.Local is fixed when the process starts, so the only way to
// read a date in a zone other than the test runner's is to run this test
// again in one.
func TestParseSinceDateIsLocal(t *testing.T) {
	const zone = "Asia/Tokyo"
	if os.Getenv("SBNN_TEST_IN_TZ") == "" {
		if _, err := time.LoadLocation(zone); err != nil {
			t.Skipf("no zone info for %s: %v", zone, err)
		}
		cmd := exec.Command(os.Args[0], "-test.run", "^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "SBNN_TEST_IN_TZ=1", "TZ="+zone)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("in TZ=%s: %v\n%s", zone, err, out)
		}
		return
	}
	if time.Local.String() != zone {
		t.Skipf("TZ was not honoured: time.Local is %s", time.Local)
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	got, err := history.ParseSince("2026-01-31", now)
	if err != nil {
		t.Fatalf("ParseSince = %v", err)
	}
	want := time.Date(2026, 1, 31, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("ParseSince(\"2026-01-31\") = %s, want %s", got, want)
	}
	// The nine hours a UTC reading would have dropped are reviews the
	// reviewer sees listed under 2026-01-31, so they have to be kept.
	early := time.Date(2026, 1, 31, 0, 30, 0, 0, time.Local)
	if early.Before(got) {
		t.Errorf("a review at %s is before the start of its own day, %s", early, got)
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
		{"2026-08-01", time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)},
		{"2026-08-01T05:00:00+09:00", time.Date(2026, 8, 1, 5, 0, 0, 0, time.FixedZone("", 9*60*60))},
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
