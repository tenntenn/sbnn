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
