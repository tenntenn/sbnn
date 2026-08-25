package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// titles returns the titles a group holds, in order.
func titles(t *testing.T, s *Store, group string) []string {
	t.Helper()
	g, ok := s.Group(group)
	if !ok {
		t.Fatalf("no group %q", group)
	}
	out := make([]string, 0, len(g.Diffs))
	for _, d := range g.Diffs {
		out = append(out, d.Title)
	}
	return out
}

func TestRoundTitlesDoNotRepeatAfterADelete(t *testing.T) {
	s := NewStore("")
	for range 3 {
		s.AddDiff(DefaultGroup, &model.Diff{})
	}
	got := titles(t, s, DefaultGroup)
	want := []string{"diff 1", "diff 2", "diff 3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("titles = %v, want %v", got, want)
	}

	// Deleting a round is a normal action: it has a x in the sidebar and a
	// DELETE endpoint. The round after it must not reuse a title.
	g, _ := s.Group(DefaultGroup)
	if !s.DeleteDiff(DefaultGroup, g.Diffs[1].ID) {
		t.Fatal("DeleteDiff() = false")
	}
	s.AddDiff(DefaultGroup, &model.Diff{})
	got = titles(t, s, DefaultGroup)
	want = []string{"diff 1", "diff 3", "diff 4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("titles after deleting the 2nd and adding one = %v, want %v", got, want)
	}
}

func TestRoundsAreCountedPerGroup(t *testing.T) {
	s := NewStore("")
	s.AddDiff(DefaultGroup, &model.Diff{})
	s.AddDiff(DefaultGroup, &model.Diff{})
	s.AddDiff("api", &model.Diff{})
	if got := titles(t, s, "api"); got[0] != "diff 1" {
		t.Errorf("first title of a second group = %q, want %q", got[0], "diff 1")
	}

	// A group that is closed and comes back is a new review.
	if !s.DeleteGroup(DefaultGroup) {
		t.Fatal("DeleteGroup() = false")
	}
	s.AddDiff(DefaultGroup, &model.Diff{})
	if got := titles(t, s, DefaultGroup); got[0] != "diff 1" {
		t.Errorf("first title after the group was closed = %q, want %q", got[0], "diff 1")
	}
}

func TestRoundNumbersSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	s.AddDiff(DefaultGroup, &model.Diff{})
	s.AddDiff(DefaultGroup, &model.Diff{})

	restarted := NewStore(path)
	if err := restarted.Load(); err != nil {
		t.Fatal(err)
	}
	restarted.AddDiff(DefaultGroup, &model.Diff{})
	if got := titles(t, restarted, DefaultGroup); got[2] != "diff 3" {
		t.Errorf("title after a restart = %q, want %q", got[2], "diff 3")
	}
}

// A session written before the rounds were counted has to carry on from the
// highest number its titles show, not from how many diffs it holds.
func TestLegacySessionCarriesOnFromItsHighestRound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	legacy := `{"version":1,"seq":3,"groups":[{"name":"default","diffs":[` +
		`{"id":"d1","title":"diff 1"},{"id":"d3","title":"diff 3"}]}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.AddDiff(DefaultGroup, &model.Diff{})
	got := titles(t, s, DefaultGroup)
	if got[2] != "diff 4" {
		t.Errorf("title after a legacy session = %q, want %q (past every title it held)", got[2], "diff 4")
	}
}

func TestTitlesAreCleanedUp(t *testing.T) {
	long := strings.Repeat("x", 200)
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"kept as is", "round 2: the parser", "round 2: the parser"},
		{"surrounding space", "  spaced  ", "spaced"},
		{"a line break", "first\nsecond", "first second"},
		{"a control character", "bell\aend", "bell end"},
		{"empty after trimming", " \n\t ", "diff 1"},
		{"too long", long, strings.Repeat("x", maxTitleLen-1) + "…"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore("")
			d := s.AddDiff(DefaultGroup, &model.Diff{Title: tt.in})
			if d.Title != tt.want {
				t.Errorf("title = %q, want %q", d.Title, tt.want)
			}
			if n := len([]rune(d.Title)); n > maxTitleLen {
				t.Errorf("title is %d runes, want at most %d", n, maxTitleLen)
			}
		})
	}
}

func TestRoundsArePersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	s.AddDiff("api", &model.Diff{})
	s.AddDiff("api", &model.Diff{})

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Rounds["api"] != 2 {
		t.Errorf(`"rounds" = %v, want api=2`, p.Rounds)
	}
}
