package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

func TestIDsAreCountedPerKind(t *testing.T) {
	s := NewStore("")
	// `git diff | sbnn --on-review ...` registers the hook before it sends
	// the diff, which used to make that first diff d2.
	h, err := s.AddHook(DefaultGroup, &model.Hook{Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	d := s.AddDiff(DefaultGroup, &model.Diff{Title: "first"})
	c, err := s.AddComment(&model.Comment{Group: DefaultGroup, DiffID: d.ID, Body: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		what string
		got  string
		want string
	}{
		{"first hook", h.ID, "h1"},
		{"first diff", d.ID, "d1"},
		{"first comment", c.ID, "c1"},
		{"second diff", s.AddDiff(DefaultGroup, &model.Diff{}).ID, "d2"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s id = %q, want %q", tt.what, tt.got, tt.want)
		}
	}
}

func TestIDsCarryOnAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	first := s.AddDiff("api", &model.Diff{}).ID

	restarted := NewStore(path)
	if err := restarted.Load(); err != nil {
		t.Fatal(err)
	}
	second := restarted.AddDiff("api", &model.Diff{}).ID
	if first != "d1" || second != "d2" {
		t.Errorf("diff ids across a restart = %q, %q, want d1, d2", first, second)
	}
}

// A session file written by an sbnn that shared one counter must keep
// working: the ids it already handed out must not be handed out again.
func TestLegacySessionKeepsItsIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	legacy := `{"version":1,"seq":4,"groups":[{"name":"default","diffs":[{"id":"d2","title":"old"}],` +
		`"comments":[{"id":"c3","group":"default","diffId":"d2","body":"old"}],` +
		`"hooks":[{"id":"h1","command":"true"}]}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	g, ok := s.Group(DefaultGroup)
	if !ok || len(g.Diffs) != 1 || g.Diffs[0].ID != "d2" {
		t.Fatalf("restored group = %+v (ok=%v), want the d2 diff", g, ok)
	}

	d := s.AddDiff(DefaultGroup, &model.Diff{})
	c, err := s.AddComment(&model.Comment{Group: DefaultGroup, DiffID: "d2", Body: "new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		what  string
		got   string
		taken string
	}{
		{"diff", d.ID, "d2"},
		{"comment", c.ID, "c3"},
	} {
		if tt.got == tt.taken {
			t.Errorf("new %s reuses the id %q of the loaded session", tt.what, tt.taken)
		}
	}
	if d.ID != "d5" {
		t.Errorf("diff id after a legacy session = %q, want d5 (past the shared counter)", d.ID)
	}
}

// The pre-split "seq" field is still written, so an sbnn old enough to read
// only that field does not reuse ids either.
func TestSharedSeqIsWrittenForOlderSbnn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	s.AddDiff(DefaultGroup, &model.Diff{})
	s.AddDiff(DefaultGroup, &model.Diff{})
	if _, err := s.AddHook(DefaultGroup, &model.Hook{Command: "true"}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Seq != 2 {
		t.Errorf(`"seq" = %d, want 2 (the highest per-prefix counter)`, p.Seq)
	}
	if p.Seqs["d"] != 2 || p.Seqs["h"] != 1 {
		t.Errorf(`"seqs" = %v, want d=2 h=1`, p.Seqs)
	}
}
