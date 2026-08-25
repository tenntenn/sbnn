package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// The helpers in this package read command line input and turn it into what
// the rest of sbnn works with. They are pure, they are where the user's
// mistakes land first, and until now none of them was called by a test.
//
// Every identifier here starts with "helpers" on purpose: package cmd is
// getting test files from several directions at once, and a plainly named
// fixture would collide with theirs the moment both land.

// helpersRestoreVerdictFlags puts the submit flags back when the test ends.
// The flags are package globals, so a test that leaves them set changes what
// every later test in the package sees.
func helpersRestoreVerdictFlags(t *testing.T) {
	t.Helper()
	approve, changes, verdict := submitApprove, submitChanges, submitVerdict
	t.Cleanup(func() {
		submitApprove, submitChanges, submitVerdict = approve, changes, verdict
	})
}

// helpersRestoreOpenFlags does the same for the browser flags.
func helpersRestoreOpenFlags(t *testing.T) {
	t.Helper()
	open, no := openBrowser, noOpen
	t.Cleanup(func() {
		openBrowser, noOpen = open, no
	})
}

// helpersPiped reports whether this test binary's output is going somewhere
// other than a terminal, which is the condition shouldOpen looks at and the
// one thing about it a test cannot set.
func helpersPiped() bool {
	return !isTerminal(os.Stdout) && !isTerminal(os.Stderr)
}

func TestHelpersChosenVerdict(t *testing.T) {
	tests := []struct {
		name    string
		approve bool
		changes bool
		verdict string
		want    model.Verdict
		wantErr bool
	}{
		{name: "no flag at all is a plain commented review", want: model.VerdictCommented},
		{name: "--approve", approve: true, want: model.VerdictApproved},
		{name: "--request-changes", changes: true, want: model.VerdictChangesRequested},
		{name: "--verdict approved", verdict: "approved", want: model.VerdictApproved},
		{name: "--verdict commented", verdict: "commented", want: model.VerdictCommented},
		{
			name:    "--verdict changes-requested",
			verdict: "changes-requested",
			want:    model.VerdictChangesRequested,
		},
		{name: "--verdict lgtm", verdict: "lgtm", want: model.VerdictApproved},
		{
			name:    "--verdict request-changes",
			verdict: "request-changes",
			want:    model.VerdictChangesRequested,
		},
		{name: "surrounding space and case", verdict: "  Approved ", want: model.VerdictApproved},
		{name: "a word nobody defined", verdict: "maybe", wantErr: true},
		{name: "the verdict of another tool", verdict: "rejected", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpersRestoreVerdictFlags(t)
			submitApprove, submitChanges, submitVerdict = tt.approve, tt.changes, tt.verdict

			got, err := chosenVerdict()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("chosenVerdict() = %q, want an error", got)
				}
				// The message has to name what to type instead: the
				// flag is the only place the spellings are written
				// down for whoever got it wrong.
				if !strings.Contains(err.Error(), tt.verdict) {
					t.Errorf("error does not quote the input: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("chosenVerdict() failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("chosenVerdict() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHelpersGroupName(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "nothing given falls back to the default group", want: server.DefaultGroup},
		{name: "--target", flag: "api", want: "api"},
		{name: "$SBNN_TARGET", env: "api", want: "api"},
		{name: "--target wins over $SBNN_TARGET", flag: "web", env: "api", want: "web"},
		{name: "an empty $SBNN_TARGET is not a group name", env: "", want: server.DefaultGroup},
		{name: "dots, dashes and underscores", flag: "a.b-c_d", want: "a.b-c_d"},
		{name: "digits", flag: "9", want: "9"},
		{name: "64 characters is the limit", flag: strings.Repeat("a", 64), want: strings.Repeat("a", 64)},
		{name: "65 characters is over it", flag: strings.Repeat("a", 65), wantErr: true},
		{name: "a slash would be a path", flag: "a/b", wantErr: true},
		{name: "a space", flag: "my group", wantErr: true},
		{name: "leading dash", flag: "-api", wantErr: true},
		{name: "a bad $SBNN_TARGET is reported, not ignored", env: "a/b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(TargetEnv, tt.env)

			got, err := groupName(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("groupName(%q) = %q, want an error", tt.flag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("groupName(%q) failed: %v", tt.flag, err)
			}
			if got != tt.want {
				t.Errorf("groupName(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

// The two encoders are what --json and --format jsonl come out of, so the
// shape they write is the interface: indented for a human or jq, one object
// per line for a pipeline that reads line by line.
func TestHelpersJSONEncoder(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "a string", value: "api", want: "\"api\"\n"},
		{
			name:  "an object is indented by two spaces",
			value: map[string]string{"group": "api"},
			want:  "{\n  \"group\": \"api\"\n}\n",
		},
		{
			name:  "nesting indents further",
			value: map[string]map[string]int{"diff": {"files": 2}},
			want:  "{\n  \"diff\": {\n    \"files\": 2\n  }\n}\n",
		},
		{
			name:  "an array of objects",
			value: []map[string]int{{"a": 1}},
			want:  "[\n  {\n    \"a\": 1\n  }\n]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := jsonEncoder(&buf).Encode(tt.value); err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("jsonEncoder wrote\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestHelpersLineEncoder(t *testing.T) {
	tests := []struct {
		name   string
		values []any
		want   string
	}{
		{
			name:   "one object is one line",
			values: []any{map[string]string{"group": "api"}},
			want:   "{\"group\":\"api\"}\n",
		},
		{
			name: "two objects are two lines, not an array",
			values: []any{
				map[string]int{"a": 1},
				map[string]int{"b": 2},
			},
			want: "{\"a\":1}\n{\"b\":2}\n",
		},
		{
			name:   "a nested object stays on its line",
			values: []any{map[string]map[string]int{"diff": {"files": 2}}},
			want:   "{\"diff\":{\"files\":2}}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := lineEncoder(&buf)
			for _, v := range tt.values {
				if err := enc.Encode(v); err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
			}
			got := buf.String()
			if got != tt.want {
				t.Errorf("lineEncoder wrote\n%q\nwant\n%q", got, tt.want)
			}
			if n := strings.Count(got, "\n"); n != len(tt.values) {
				t.Errorf("got %d newline(s) for %d value(s): %q", n, len(tt.values), got)
			}
		})
	}
}

func TestHelpersShouldOpen(t *testing.T) {
	tests := []struct {
		name        string
		noOpen      bool
		openBrowser bool
		started     bool
		want        bool
		// needsPipe marks the cases decided by stdout and stderr not
		// being a terminal, which the test cannot arrange.
		needsPipe bool
	}{
		{name: "--no-open after starting the server", noOpen: true, started: true},
		{name: "--no-open beats --open", noOpen: true, openBrowser: true, started: true},
		{name: "--open without having started one", openBrowser: true, want: true},
		{name: "--open when the server was already up", openBrowser: true, started: true, want: true},
		{name: "started, but nobody is looking", started: true, needsPipe: true},
		{name: "already running and nobody is looking", needsPipe: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsPipe && !helpersPiped() {
				t.Skip("stdout or stderr is a terminal; this case is about a pipeline")
			}
			helpersRestoreOpenFlags(t)
			noOpen, openBrowser = tt.noOpen, tt.openBrowser

			if got := shouldOpen(tt.started); got != tt.want {
				t.Errorf("shouldOpen(%v) = %v, want %v", tt.started, got, tt.want)
			}
		})
	}
}

func TestHelpersSummarize(t *testing.T) {
	tests := []struct {
		name string
		res  *server.AddDiffResponse
		want *diffSummary
	}{
		{name: "no response at all", res: nil, want: nil},
		{
			name: "a response that carried no diff",
			res:  &server.AddDiffResponse{Group: "api"},
			want: nil,
		},
		{
			name: "an empty diff still has an id and a title",
			res: &server.AddDiffResponse{Diff: &model.Diff{
				ID: "d1", Title: "nothing",
			}},
			want: &diffSummary{ID: "d1", Title: "nothing"},
		},
		{
			name: "the counts are the sum over the files",
			res: &server.AddDiffResponse{Diff: &model.Diff{
				ID: "d2", Title: "three files",
				Files: []*model.File{
					{NewPath: "a.go", Additions: 3, Deletions: 1},
					{NewPath: "b.go", Additions: 10, Deletions: 0},
					{NewPath: "c.go", Additions: 0, Deletions: 7},
				},
			}},
			want: &diffSummary{
				ID: "d2", Title: "three files",
				Files: 3, Additions: 13, Deletions: 8,
			},
		},
		{
			name: "markdown files are counted separately, and still as files",
			res: &server.AddDiffResponse{Diff: &model.Diff{
				ID: "d3", Title: "docs",
				Files: []*model.File{
					{NewPath: "README.md", Additions: 2, IsMarkdown: true},
					{NewPath: "docs/x.md", Additions: 1, IsMarkdown: true},
					{NewPath: "main.go", Additions: 4},
				},
			}},
			want: &diffSummary{
				ID: "d3", Title: "docs",
				Files: 3, Additions: 7, MarkdownFiles: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(tt.res)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("summarize() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("summarize() = nil, want %+v", tt.want)
			}
			if *got != *tt.want {
				t.Errorf("summarize() = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

func TestHelpersShortDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "no wait at all", d: 0, want: "0s"},
		{name: "under a second rounds down to none", d: 900 * time.Millisecond, want: "0s"},
		{name: "seconds", d: 45 * time.Second, want: "45s"},
		{name: "the last second before a minute", d: 59 * time.Second, want: "59s"},
		{name: "a minute and a half drops the seconds", d: 90 * time.Second, want: "1m"},
		{name: "the last minute before an hour", d: 59 * time.Minute, want: "59m"},
		{name: "an hour pads the minutes", d: time.Hour, want: "1h00m"},
		{name: "an hour and a half", d: 90 * time.Minute, want: "1h30m"},
		{name: "single digit minutes stay padded", d: 2*time.Hour + 5*time.Minute, want: "2h05m"},
		{name: "the last hour before days take over", d: 47 * time.Hour, want: "47h00m"},
		{name: "two days", d: 48 * time.Hour, want: "2d"},
		{name: "days drop the hours", d: 72*time.Hour + 30*time.Minute, want: "3d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortDuration(tt.d); got != tt.want {
				t.Errorf("shortDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestHelpersIndent(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		prefix string
		want   string
	}{
		{name: "one line", text: "a note", prefix: "      ", want: "      a note"},
		{
			name:   "every line of a note is moved, not just the first",
			text:   "a note\nand more of it",
			prefix: "  ",
			want:   "  a note\n  and more of it",
		},
		{
			name:   "a paragraph break keeps its place",
			text:   "one\n\ntwo",
			prefix: "> ",
			want:   "> one\n> \n> two",
		},
		{name: "an empty prefix changes nothing", text: "a\nb", prefix: "", want: "a\nb"},
		{
			name:   "text that is already indented is indented further",
			text:   "  a\n    b",
			prefix: "  ",
			want:   "    a\n      b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := indent(tt.text, tt.prefix); got != tt.want {
				t.Errorf("indent(%q, %q) = %q, want %q", tt.text, tt.prefix, got, tt.want)
			}
		})
	}
}

// lineRangeOf writes the ":12-14" that follows a path everywhere sbnn prints
// a comment, so a single line must not come out as a range of one.
func TestHelpersLineRangeOf(t *testing.T) {
	tests := []struct {
		name string
		c    *model.Comment
		want string
	}{
		{name: "a single line", c: &model.Comment{StartLine: 3, EndLine: 3}, want: ":3"},
		{name: "a range", c: &model.Comment{StartLine: 12, EndLine: 14}, want: ":12-14"},
		{name: "a range of two", c: &model.Comment{StartLine: 1, EndLine: 2}, want: ":1-2"},
		{
			name: "no end line is the start line",
			c:    &model.Comment{StartLine: 7},
			want: ":7",
		},
		{
			name: "an end line before the start is not a range",
			c:    &model.Comment{StartLine: 9, EndLine: 4},
			want: ":9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineRangeOf(tt.c); got != tt.want {
				t.Errorf("lineRangeOf(%+v) = %q, want %q", tt.c, got, tt.want)
			}
		})
	}
}

// waited is the "  waited 1m" tail of a review line. A round whose wait
// cannot be worked out prints nothing rather than a zero.
func TestHelpersWaited(t *testing.T) {
	base := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		firstDiffAt time.Time
		reviewedAt  time.Time
		want        string
	}{
		{
			name:       "a round with no diff time behind it says nothing",
			reviewedAt: base,
		},
		{
			name:        "reviewed in the same instant",
			firstDiffAt: base,
			reviewedAt:  base,
		},
		{
			name:        "clocks out of order say nothing rather than a negative wait",
			firstDiffAt: base,
			reviewedAt:  base.Add(-time.Hour),
		},
		{
			name:        "a minute and a half",
			firstDiffAt: base,
			reviewedAt:  base.Add(90 * time.Second),
			want:        "  waited 1m",
		},
		{
			name:        "two hours",
			firstDiffAt: base,
			reviewedAt:  base.Add(2 * time.Hour),
			want:        "  waited 2h00m",
		},
		{
			name:        "left overnight and then some",
			firstDiffAt: base,
			reviewedAt:  base.Add(50 * time.Hour),
			want:        "  waited 2d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := history.Record{
				Group:       "api",
				FirstDiffAt: tt.firstDiffAt,
				ReviewedAt:  tt.reviewedAt,
			}
			if got := waited(rec); got != tt.want {
				t.Errorf("waited() = %q, want %q", got, tt.want)
			}
		})
	}
}
