package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/server"
)

func TestConfirm(t *testing.T) {
	const question = "Close all 2 review(s)? [y/N]: "

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes short", "y\n", true},
		{"yes short upper", "Y\n", true},
		{"yes long", "yes\n", true},
		{"yes long upper", "YES\n", true},
		{"yes padded", " y \n", true},
		{"yes without newline", "y", true},
		{"no", "n\n", false},
		{"blank line", "\n", false},
		{"eof", "", false},
		{"anything else", "yolo\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			got, err := confirm(strings.NewReader(tt.input), &out, question)
			if err != nil {
				t.Fatalf("confirm(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("confirm(%q) = %v; want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfirmAsks(t *testing.T) {
	const question = "Close all 2 review(s)? [y/N]: "

	var out strings.Builder
	if _, err := confirm(strings.NewReader("n\n"), &out, question); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if out.String() != question {
		t.Errorf("confirm wrote %q; want the question %q", out.String(), question)
	}
}

func TestClearAllQuestion(t *testing.T) {
	tests := []struct {
		name     string
		groups   []server.GroupSummary
		want     string
		contains []string
	}{
		{
			name:   "nothing to lose",
			groups: nil,
			want:   "",
		},
		{
			name: "one review, nothing open",
			groups: []server.GroupSummary{
				{Name: "default", Diffs: 1, Comments: 0, Unresolved: 0},
			},
			contains: []string{"default", "1 diff(s)", "0 comment(s)", "0 open", "Close all 1 review(s)? [y/N]: "},
		},
		{
			name: "several reviews, comments still open",
			groups: []server.GroupSummary{
				{Name: "default", Diffs: 3, Comments: 2, Unresolved: 1},
				{Name: "api", Diffs: 1, Comments: 0, Unresolved: 0},
			},
			contains: []string{
				"this will close every review on the server",
				"default", "3 diff(s)", "2 comment(s)", "1 open",
				"api", "1 diff(s)",
				"Close all 2 review(s)? [y/N]: ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clearAllQuestion(tt.groups)
			if tt.contains == nil {
				if got != tt.want {
					t.Fatalf("clearAllQuestion = %q; want %q", got, tt.want)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("clearAllQuestion = %q; want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestClearGroupQuestion(t *testing.T) {
	groups := []server.GroupSummary{
		{Name: "default", Diffs: 3, Comments: 2, Unresolved: 0},
		{Name: "api", Diffs: 1, Comments: 4, Unresolved: 1},
	}

	tests := []struct {
		name     string
		group    string
		contains []string
	}{
		{name: "nothing open, no question", group: "default"},
		{name: "unknown group is already empty", group: "gone"},
		{
			name:     "open comments are named",
			group:    "api",
			contains: []string{`Close the review of "api"?`, "1 comment(s) are still open", "[y/N]: "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clearGroupQuestion(groups, tt.group)
			if tt.contains == nil {
				if got != "" {
					t.Fatalf("clearGroupQuestion(%q) = %q; want no question", tt.group, got)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("clearGroupQuestion(%q) = %q; want it to contain %q", tt.group, got, want)
				}
			}
		})
	}
}

// A prompt is only worth asking where somebody can answer it. os.ModeCharDevice
// on its own does not say that: the null device is a character device, so
// "sbnn --clear --all < /dev/null" - what a cron job, a systemd unit or a CI
// step writes to say there is nobody here - looked like a terminal, got the
// question, and read the EOF as "no". The three ways of handing sbnn a stdin
// with nobody behind it have to agree.
func TestIsTerminalOnStreamsWithNobodyBehindThem(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	regular, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	tests := map[string]*os.File{
		"the null device": devNull,
		"a regular file":  regular,
		"a pipe":          r,
	}

	for name, f := range tests {
		t.Run(name, func(t *testing.T) {
			if isTerminal(f) {
				t.Errorf("isTerminal(%s) = true; want false - there is nobody to answer a prompt", name)
			}
		})
	}
}

// statusServer answers the one status call runClear makes and records whether
// anything was actually deleted.
func statusServer(t *testing.T, groups []server.GroupSummary) (addr string, deleted *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			seen = append(seen, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"removed":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.Status{App: "sbnn", Groups: groups})
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), &seen
}

// A declined --clear drops nothing, and the caller has to be able to see
// that: the status used to stay 0, so a script could not tell a review that
// was closed from one that was left standing. It is now an error, printed by
// Execute as "sbnn: cancelled".
func TestClearCancelledIsAnError(t *testing.T) {
	groups := []server.GroupSummary{
		{Name: "default", Diffs: 1, Comments: 2, Unresolved: 2},
		{Name: "api", Diffs: 1, Comments: 1, Unresolved: 1},
	}

	tests := map[string]struct {
		answer      string
		all         bool
		wantErr     error
		wantDeletes int
	}{
		"declining --clear --all": {answer: "n\n", all: true, wantErr: errCancelled},
		"eof on --clear --all":    {answer: "", all: true, wantErr: errCancelled},
		"accepting --clear --all": {answer: "y\n", all: true, wantDeletes: 1},
		"declining one review":    {answer: "n\n", wantErr: errCancelled},
		"accepting one review":    {answer: "y\n", wantDeletes: 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			host, deleted := statusServer(t, groups)
			hostname, p, err := net.SplitHostPort(host)
			if err != nil {
				t.Fatal(err)
			}
			n, err := strconv.Atoi(p)
			if err != nil {
				t.Fatal(err)
			}

			answers, err := os.CreateTemp(t.TempDir(), "answer")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := answers.WriteString(tt.answer); err != nil {
				t.Fatal(err)
			}
			if _, err := answers.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			oldBind, oldPort, oldAll, oldYes, oldStdin, oldTTY := bind, port, clearAll, assumeYes, os.Stdin, stdinIsTerminal
			t.Cleanup(func() {
				bind, port, clearAll, assumeYes, os.Stdin, stdinIsTerminal = oldBind, oldPort, oldAll, oldYes, oldStdin, oldTTY
			})
			bind, port, clearAll, assumeYes = hostname, n, tt.all, false
			os.Stdin = answers
			stdinIsTerminal = func() bool { return true }

			err = runClear(context.Background(), "default")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("runClear = %v; want %v", err, tt.wantErr)
			}
			if got := len(*deleted); got != tt.wantDeletes {
				t.Errorf("%d delete(s) sent (%v); want %d", got, *deleted, tt.wantDeletes)
			}
		})
	}
}
