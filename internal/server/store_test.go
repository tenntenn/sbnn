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
