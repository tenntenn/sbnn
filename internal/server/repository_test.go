package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// newWithRepository opens the session at path on a server of repo, adds a
// diff when add is set, and returns the server, or New's error.
func newWithRepository(t *testing.T, path string, repo *string, add bool) (*Server, error) {
	t.Helper()
	s, err := New(Options{SessionFile: path, Repository: repo})
	if err != nil {
		return nil, err
	}
	if add {
		s.Store().AddDiff(DefaultGroup, &model.Diff{Title: "d"})
	}
	return s, nil
}

func TestServerRepository(t *testing.T) {
	a, b, none := "/repo/a", "/repo/b", ""
	tests := []struct {
		name string
		// first opens the session and writes to it; second opens it again.
		first, second *string
		wantErr       string
		// want is what the second server reports, nil for nothing.
		want *string
	}{
		{name: "the same repository again", first: &a, second: &a, want: &a},
		{name: "another repository is refused", first: &a, second: &b,
			wantErr: "belongs to the server of /repo/a, not of /repo/b"},
		{name: "outside any repository is a repository of its own", first: &none, second: &a,
			wantErr: "belongs to the server of the diffs from outside any repository, not of /repo/a"},
		{name: "a session from before is taken over", first: nil, second: &b, want: &b},
		{name: "a server that names none reports none", first: &a, second: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session-7000.json")
			if _, err := newWithRepository(t, path, tt.first, true); err != nil {
				t.Fatal(err)
			}
			s, err := newWithRepository(t, path, tt.second, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("New = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := s.status().Repository
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("status().Repository = %v, want %v", got, tt.want)
			}
			if len(s.store.Summary("")) != 1 {
				t.Error("the session was not restored")
			}
		})
	}
}

// A session taken over from before records whose it now is with the next
// write, so that a later server of another repository is turned away.
func TestServerRepositoryIsWrittenDown(t *testing.T) {
	a, b := "/repo/a", "/repo/b"
	path := filepath.Join(t.TempDir(), "session-7000.json")
	if _, err := newWithRepository(t, path, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := newWithRepository(t, path, &a, true); err != nil {
		t.Fatal(err)
	}
	if _, err := newWithRepository(t, path, &b, false); err == nil {
		t.Error("a server of /repo/b loaded the session /repo/a took over")
	}
}
