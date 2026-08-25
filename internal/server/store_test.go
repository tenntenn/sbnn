package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// writeSession writes a session file holding one group per name.
func writeSession(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.json")
	groups := make([]*model.Group, 0, len(names))
	for i, name := range names {
		groups = append(groups, &model.Group{
			Name:  name,
			Diffs: []*model.Diff{{ID: "d" + strconv.Itoa(i+1), Title: "round"}},
		})
	}
	b, err := json.Marshal(persisted{Version: persistVersion, Seq: len(names), Groups: groups})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDropsGroupNamesItCouldNotHaveMade(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		keep bool
	}{
		{"the default group", DefaultGroup, true},
		{"an ordinary name", "feature-1.2_x", true},
		{"a path", "../../evil", false},
		{"a slash", "a/b", false},
		{"a space", "with space", false},
		{"no name at all", "", false},
		{"a leading underscore", "_internal", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore(writeSession(t, tt.in))
			if err := s.Load(); err != nil {
				t.Fatal(err)
			}
			names := s.GroupNames()
			if tt.keep {
				if len(names) != 1 || names[0] != tt.in {
					t.Fatalf("groups after load = %v, want [%q]", names, tt.in)
				}
				if g, ok := s.Group(tt.in); !ok || len(g.Diffs) != 1 {
					t.Errorf("group %q = %+v (ok=%v), want its diff kept", tt.in, g, ok)
				}
				return
			}
			if len(names) != 0 {
				t.Errorf("groups after load = %v, want the unusable name dropped", names)
			}
		})
	}
}

// The zombie must be gone from everything that lists groups, and must not be
// written back out on the next save.
func TestADroppedGroupIsNotPersistedAgain(t *testing.T) {
	path := writeSession(t, DefaultGroup, "../../evil")
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	sums := s.Summary("http://localhost:6280")
	if len(sums) != 1 {
		t.Fatalf("Summary() = %+v, want exactly the default group", sums)
	}
	for _, sum := range sums {
		if sum.Name != DefaultGroup {
			t.Errorf("Summary() holds %q, want only the default group", sum.Name)
		}
	}

	s.AddDiff(DefaultGroup, &model.Diff{})
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "evil") {
		t.Errorf("the session file still holds the dropped group:\n%s", b)
	}
}

// A JSON null in the groups array is exactly the kind of damage validGroups
// exists to survive, and it reaches Load on server.New's goroutine: a panic
// there kills the process before it listens, so every later sbnn invocation
// times out against a server that is not coming back.
func TestLoadSurvivesANullGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := []byte(`{"version":` + strconv.Itoa(persistVersion) + `,"seq":1,"groups":[null,` +
		`{"name":"default","diffs":[{"id":"d1","title":"kept"}]},null]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if names := s.GroupNames(); len(names) != 1 || names[0] != DefaultGroup {
		t.Fatalf("groups after load = %v, want [%q]", names, DefaultGroup)
	}
	if g, ok := s.Group(DefaultGroup); !ok || len(g.Diffs) != 1 {
		t.Errorf("group %q = %+v (ok=%v), want its diff kept", DefaultGroup, g, ok)
	}
	if sums := s.Summary("http://localhost:6280"); len(sums) != 1 {
		t.Errorf("Summary() = %+v, want exactly the default group", sums)
	}

	// The nulls must not come back on the next save either.
	s.AddDiff(DefaultGroup, &model.Diff{})
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "null,") || strings.Contains(string(b), ",null") {
		t.Errorf("persisted session = %s, want the null groups gone", b)
	}
}
