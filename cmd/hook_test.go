package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/client"
)

// DeleteHook is what makes "sbnn hook --remove <id>" possible, so what it
// puts on the wire is a contract with the server's by-ID route:
//
//	DELETE /_/api/groups/{group}/hooks/{id}
//
// The group and the ID have to stay one path segment each, whatever is in
// them, and a count of 0 has to come back as a count - the server answers an
// unknown ID that way, and it is the CLI that turns it into an error.
func TestDeleteHookRequestAndCount(t *testing.T) {
	tests := map[string]struct {
		group       string
		id          string
		respond     string
		wantPath    string
		wantEscaped string
		want        int
	}{
		"removes one hook by id": {
			group:       "api",
			id:          "h2",
			respond:     `{"removed":1}`,
			wantPath:    "/_/api/groups/api/hooks/h2",
			wantEscaped: "/_/api/groups/api/hooks/h2",
			want:        1,
		},
		"unknown id is a count of zero, not an error": {
			group:       "api",
			id:          "nosuchid",
			respond:     `{"removed":0}`,
			wantPath:    "/_/api/groups/api/hooks/nosuchid",
			wantEscaped: "/_/api/groups/api/hooks/nosuchid",
			want:        0,
		},
		// A slash would split the segment in two and address a different
		// route; a space is not legal in a request target unescaped.
		"slashes and spaces stay inside their segment": {
			group:       "team/api",
			id:          "h 1",
			respond:     `{"removed":1}`,
			wantPath:    "/_/api/groups/team/api/hooks/h 1",
			wantEscaped: "/_/api/groups/team%2Fapi/hooks/h%201",
			want:        1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var gotMethod, gotPath, gotEscaped string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotEscaped = r.Method, r.URL.Path, r.URL.EscapedPath()
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.respond))
			}))
			defer srv.Close()

			c := client.New(strings.TrimPrefix(srv.URL, "http://"), time.Second)
			got, err := c.DeleteHook(context.Background(), tt.group, tt.id)
			if err != nil {
				t.Fatalf("DeleteHook(%q, %q): %v", tt.group, tt.id, err)
			}
			if got != tt.want {
				t.Errorf("removed = %d, want %d", got, tt.want)
			}
			if gotMethod != http.MethodDelete {
				t.Errorf("method = %s, want %s", gotMethod, http.MethodDelete)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotEscaped != tt.wantEscaped {
				t.Errorf("escaped path = %q, want %q", gotEscaped, tt.wantEscaped)
			}
		})
	}
}

// DeleteHooks is what --clear uses and what every other caller of the
// group-wide route depends on. --remove was added next to it, not on top of
// it, so it still has to address the route without an ID.
func TestDeleteHooksStillClearsTheWholeGroup(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"removed":2}`))
	}))
	defer srv.Close()

	c := client.New(strings.TrimPrefix(srv.URL, "http://"), time.Second)
	got, err := c.DeleteHooks(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if want := 2; got != want {
		t.Errorf("removed = %d, want %d", got, want)
	}
	if want := "/_/api/groups/api/hooks"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

// An HTTP error has to stay an error: --remove must not report a hook gone
// because the server said 500.
func TestDeleteHookReportsServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := client.New(strings.TrimPrefix(srv.URL, "http://"), time.Second)
	got, err := c.DeleteHook(context.Background(), "api", "h2")
	if err == nil {
		t.Fatalf("DeleteHook returned %d and no error on a 500", got)
	}
	if got != 0 {
		t.Errorf("removed = %d on an error, want 0", got)
	}
}

// validateHookFlags parses args the way cobra does before RunE and returns
// what the flag rules make of them. Flag state on hookCmd is package-wide,
// so every value it touches is put back before the next case runs.
func validateHookFlags(t *testing.T, args []string) error {
	t.Helper()
	fs := hookCmd.Flags()
	t.Cleanup(func() {
		for _, name := range []string{"remove", "clear", "on-review", "on-review-url"} {
			f := fs.Lookup(name)
			f.Changed = false
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Fatalf("restoring --%s: %v", name, err)
			}
		}
	})
	if err := hookCmd.ParseFlags(args); err != nil {
		return err
	}
	return hookCmd.ValidateFlagGroups()
}

// Asking to remove a hook and to register one in the same command used to be
// settled by the order of the switch in runHook: the removal happened, the
// hook the user asked for was dropped without a word, and the command still
// exited 0. The pairs have to be refused before anything is touched, and the
// combinations that mean something - a removal on its own, a registration on
// its own, a hook carrying both a command and a URL - have to stay legal.
func TestHookRemoveRefusesToBeMixedWithRegistration(t *testing.T) {
	tests := map[string]struct {
		args    []string
		wantErr bool
	}{
		"remove with a command":     {args: []string{"--remove", "h2", "--on-review", "echo NEW"}, wantErr: true},
		"remove with a url":         {args: []string{"--remove", "h2", "--on-review-url", "http://localhost:9000/reviews"}, wantErr: true},
		"remove with clear":         {args: []string{"--remove", "h2", "--clear"}, wantErr: true},
		"remove alone":              {args: []string{"--remove", "h2"}},
		"command alone":             {args: []string{"--on-review", "echo NEW"}},
		"url alone":                 {args: []string{"--on-review-url", "http://localhost:9000/reviews"}},
		"one hook with both halves": {args: []string{"--on-review", "echo NEW", "--on-review-url", "http://localhost:9000/reviews"}},
		"listing takes no flags":    {args: nil},
		"clear alone":               {args: []string{"--clear"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateHookFlags(t, tt.args)
			if tt.wantErr && err == nil {
				t.Fatalf("sbnn hook %s was accepted, want it refused", strings.Join(tt.args, " "))
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("sbnn hook %s was refused: %v", strings.Join(tt.args, " "), err)
			}
		})
	}
}
