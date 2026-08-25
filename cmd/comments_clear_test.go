package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// clearRecorder is a stub sbnn server that remembers how the comments were
// asked to be cleared.
type clearRecorder struct {
	called bool
	query  string
}

func serveClear(t *testing.T, group string) *clearRecorder {
	t.Helper()
	rec := &clearRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/_/api/status", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"app": "sbnn"})
	})
	mux.HandleFunc("DELETE /_/api/groups/"+group+"/comments", func(w http.ResponseWriter, r *http.Request) {
		rec.called = true
		rec.query = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]int{"removed": 2})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	oldBind, oldPort, oldTarget := bind, port, target
	t.Cleanup(func() { bind, port, target = oldBind, oldPort, oldTarget })
	bind, port, target = host, p, group
	return rec
}

// withClearFlags sets the flag variables runComments reads and puts them
// back afterwards.
func withClearFlags(t *testing.T, clear, resolvedOnly, includeResolved bool) {
	t.Helper()
	old := [6]bool{commentsClear, commentsResolvedOnly, commentsResolved, commentsQuiet, commentsExitCode, commentsJSON}
	t.Cleanup(func() {
		commentsClear, commentsResolvedOnly, commentsResolved = old[0], old[1], old[2]
		commentsQuiet, commentsExitCode, commentsJSON = old[3], old[4], old[5]
	})
	commentsClear, commentsResolvedOnly, commentsResolved = clear, resolvedOnly, includeResolved
	commentsQuiet, commentsExitCode, commentsJSON = false, false, false
}

// The server has always taken ?resolved=true on the delete, and
// client.ClearComments has always had the parameter; the CLI passed a
// hard-coded false, so there was no way to tidy away what had been dealt
// with while keeping what was still open.
func TestCommentsClearPassesResolvedOnly(t *testing.T) {
	tests := []struct {
		name         string
		resolvedOnly bool
		wantQuery    string
	}{
		{name: "everything", resolvedOnly: false, wantQuery: ""},
		{name: "only the resolved ones", resolvedOnly: true, wantQuery: "resolved=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(TargetEnv, "")
			rec := serveClear(t, "default")
			withClearFlags(t, true, tt.resolvedOnly, false)

			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			if err := runComments(cmd, nil); err != nil {
				t.Fatalf("sbnn comments --clear: %v", err)
			}
			if !rec.called {
				t.Fatal("the comments were never cleared")
			}
			if rec.query != tt.wantQuery {
				t.Errorf("cleared with query %q, want %q", rec.query, tt.wantQuery)
			}
		})
	}
}

// The two flags that cannot mean anything together are refused rather than
// ignored: silently deleting a different set of comments from the one that
// was asked for is not a recoverable mistake.
func TestCommentsClearFlagCombinations(t *testing.T) {
	tests := []struct {
		name            string
		clear           bool
		resolvedOnly    bool
		includeResolved bool
		wantErr         string
	}{
		{name: "clear alone", clear: true},
		{name: "clear resolved only", clear: true, resolvedOnly: true},
		{name: "include-resolved without clear", includeResolved: true},
		{name: "resolved-only without clear", resolvedOnly: true, wantErr: "--resolved-only"},
		{name: "include-resolved with clear", clear: true, includeResolved: true, wantErr: "--include-resolved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withClearFlags(t, tt.clear, tt.resolvedOnly, tt.includeResolved)
			err := checkClearFlags()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkClearFlags() = %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatal("checkClearFlags() accepted the combination, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error does not name %s: %v", tt.wantErr, err)
			}
		})
	}
}

// The combination is refused before the server is contacted, so nothing is
// deleted on the way to the error.
func TestCommentsClearRefusesBeforeDeleting(t *testing.T) {
	t.Setenv(TargetEnv, "")
	rec := serveClear(t, "default")
	withClearFlags(t, true, false, true)

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := runComments(cmd, nil); err == nil {
		t.Fatal("runComments accepted --clear --include-resolved, want an error")
	}
	if rec.called {
		t.Error("the comments were cleared despite the error")
	}
}
