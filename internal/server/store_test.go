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

func TestLoadChecksTheFormatVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version int
		wantErr bool
	}{
		{"the version this build writes", persistVersion, false},
		{"a file from before the field existed", 0, false},
		{"a file from a newer sbnn", persistVersion + 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			b, err := json.Marshal(persisted{
				Version: tt.version,
				Seq:     1,
				Groups:  []*model.Group{{Name: DefaultGroup, Diffs: []*model.Diff{{ID: "d1", Title: "kept"}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, b, 0o600); err != nil {
				t.Fatal(err)
			}

			s := NewStore(path)
			err = s.Load()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Load() = %v, want nil", err)
				}
				if g, ok := s.Group(DefaultGroup); !ok || len(g.Diffs) != 1 {
					t.Fatalf("restored group = %+v (ok=%v), want the stored diff", g, ok)
				}
				return
			}
			if err == nil {
				t.Fatal("Load() = nil for a file from a newer sbnn, want an error")
			}
			// The reader has to be told what to upgrade past.
			for _, want := range []string{"newer sbnn", strconv.Itoa(tt.version)} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Load() error = %q, want it to mention %q", err, want)
				}
			}
			// Nothing from the refused file is in the store.
			if names := s.GroupNames(); len(names) != 0 {
				t.Errorf("groups after a refused load = %v, want none", names)
			}
		})
	}
}

// A refused session file is the only copy of a session this build cannot
// read. The server logs the refusal and carries on, so the store has to make
// sure the next write does not land on top of those bytes.
func TestARefusedSessionFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := []byte(`{"version":` + strconv.Itoa(persistVersion+1) + `,"seq":7,"groups":[` +
		`{"name":"default","diffs":[{"id":"d7","title":"from the future"}],` +
		`"comments":[{"id":"c1","group":"default","diffId":"d7","body":"keep me"}]}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	if err := s.Load(); err == nil {
		t.Fatal("Load() = nil for a file from a newer sbnn, want an error")
	}

	// Everything a running server may do after the refusal was logged.
	s.AddDiff(DefaultGroup, &model.Diff{Title: "new"})
	if _, err := s.AddComment(&model.Comment{Group: DefaultGroup, DiffID: "d1", Body: "new"}); err != nil {
		t.Logf("AddComment: %v", err)
	}
	s.DeleteAllGroups()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Errorf("session file after a refused load =\n%s\nwant it untouched:\n%s", got, raw)
	}
}

// Sealing must be limited to the store that refused its file: a store that
// loaded normally still has to save.
func TestALoadedSessionFileIsStillWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	b, err := json.Marshal(persisted{Version: persistVersion, Seq: 1, Groups: []*model.Group{{Name: DefaultGroup}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	s.AddDiff(DefaultGroup, &model.Diff{Title: "new"})

	var p persisted
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Groups) != 1 || len(p.Groups[0].Diffs) != 1 {
		t.Errorf("persisted session = %s, want the added diff", got)
	}
}
