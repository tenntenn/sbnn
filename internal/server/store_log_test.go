package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// stateOf is everything a restart has to bring back, as text.
func stateOf(t *testing.T, s *Store) string {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	groups := s.groups
	if len(groups) == 0 {
		// A store that has held groups and one that never did look alike.
		groups = nil
	}
	b, err := json.Marshal(persisted{Seqs: s.seq, Groups: groups, Rounds: s.rounds})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// reloaded reads the session file the way the next server would.
func reloaded(t *testing.T, path string) *Store {
	t.Helper()
	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	return s
}

// firstDiffID and firstCommentID name something a step can act on.
func firstDiffID(t *testing.T, s *Store, group string) string {
	t.Helper()
	g, ok := s.Group(group)
	if !ok || len(g.Diffs) == 0 {
		t.Fatalf("group %q holds no diff", group)
	}
	return g.Diffs[0].ID
}

func firstCommentID(t *testing.T, s *Store, group string) string {
	t.Helper()
	g, ok := s.Group(group)
	if !ok || len(g.Comments) == 0 {
		t.Fatalf("group %q holds no comment", group)
	}
	return g.Comments[0].ID
}

// mutate performs one named change through the store's own API.
func mutate(t *testing.T, s *Store, step string) {
	t.Helper()
	body, resolved := "edited", true
	switch step {
	case "add diff":
		s.AddDiff(DefaultGroup, &model.Diff{Raw: "raw", Files: []*model.File{{ID: "f1", NewPath: "a.go"}}})
	case "add diff to another group":
		s.AddDiff("api", &model.Diff{Title: "other"})
	case "add comment":
		c := &model.Comment{Group: DefaultGroup, DiffID: firstDiffID(t, s, DefaultGroup), FileID: "f1", Body: "first"}
		if _, err := s.AddComment(c); err != nil {
			t.Fatal(err)
		}
	case "edit comment":
		s.UpdateComment(DefaultGroup, firstCommentID(t, s, DefaultGroup), CommentPatch{Body: &body, Resolved: &resolved})
	case "delete comment":
		s.DeleteComment(DefaultGroup, firstCommentID(t, s, DefaultGroup))
	case "clear resolved comments":
		s.ClearComments(DefaultGroup, true)
	case "clear comments":
		s.ClearComments(DefaultGroup, false)
	case "add hook":
		if _, err := s.AddHook(DefaultGroup, &model.Hook{Command: "true"}); err != nil {
			t.Fatal(err)
		}
	case "record hook run":
		s.RecordHookCommandRun(DefaultGroup, "h1", model.HookRun{OK: true, Detail: "ok"})
	case "submit review":
		s.SubmitReview(DefaultGroup, "done", model.VerdictApproved)
	case "delete hooks":
		s.DeleteHooks(DefaultGroup, "")
	case "delete diff":
		s.DeleteDiff(DefaultGroup, firstDiffID(t, s, DefaultGroup))
	case "delete group":
		s.DeleteGroup("api")
	case "delete all groups":
		s.DeleteAllGroups()
	default:
		t.Fatalf("unknown step %q", step)
	}
}

// Every mutation is on disk when it returns, and what is on disk is what the
// server holds. The checks run after each step, not at the end, because a
// log is replayed in order and one record that does not replay the way it
// was applied shows up in the next step's comparison.
func TestEveryMutationIsOnDiskWhenItReturns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	steps := []string{
		"add diff",
		"add comment",
		"add comment",
		"edit comment",
		"add hook",
		"record hook run",
		"submit review",
		"add diff to another group",
		"add diff",
		"delete comment",
		"clear resolved comments",
		"add comment",
		"clear comments",
		"delete hooks",
		"delete diff",
		"add comment",
		"delete group",
		"add diff",
		"delete all groups",
		"add diff",
	}
	for i, step := range steps {
		mutate(t, s, step)
		if got, want := stateOf(t, reloaded(t, path)), stateOf(t, s); got != want {
			t.Fatalf("after step %d (%s) the file holds\n%s\nwant\n%s", i+1, step, got, want)
		}
	}
	if err := s.PersistError(); err != nil {
		t.Errorf("PersistError() = %v, want nil", err)
	}
}

// A mutation appends a small record. It does not write the session again, so
// what it costs does not grow with the size of the review.
func TestAMutationAppendsInsteadOfRewriting(t *testing.T) {
	cases := []struct {
		name string
		step string
	}{
		{"a comment", "add comment"},
		{"an edit", "edit comment"},
		{"a review", "submit review"},
		{"a hook", "add hook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			s := bigStore(t, path)
			mutate(t, s, "add comment")

			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			head, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mutate(t, s, tc.step)

			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) {
				t.Error("the session file was replaced, want a record appended to it")
			}
			grown := after.Size() - before.Size()
			if grown <= 0 || grown > 4<<10 {
				t.Errorf("the file grew by %d bytes (of %d), want a record of a few hundred", grown, before.Size())
			}
			now, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(now), string(head)) {
				t.Error("the bytes already in the file changed")
			}
		})
	}
}

// Once the log is longer than the snapshot it is folded into a new one, so
// the file stays within about twice the session and a restart does not replay
// an unbounded history.
func TestTheLogIsFoldedIntoTheSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	s.compactMin = 1
	mutate(t, s, "add diff")
	mutate(t, s, "add comment")

	replaced := 0
	for range 200 {
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		s.AddComment(&model.Comment{Group: DefaultGroup, DiffID: firstDiffID(t, s, DefaultGroup), FileID: "f1", Body: strings.Repeat("x", 100)})
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) {
			replaced++
		}
		if got, want := stateOf(t, reloaded(t, path)), stateOf(t, s); got != want {
			t.Fatalf("the file holds\n%s\nwant\n%s", got, want)
		}
	}
	if replaced == 0 {
		t.Error("the snapshot was never rewritten")
	}
	if replaced > 20 {
		t.Errorf("the snapshot was rewritten %d times in 200 mutations, want it amortised", replaced)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := parseSession(b)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.logBytes > 2*snapshot.snapBytes+1024 {
		t.Errorf("the log is %d bytes beside a snapshot of %d, want it folded in", snapshot.logBytes, snapshot.snapBytes)
	}
}

// A server killed in the middle of an append leaves a last line that was never
// acknowledged. It is dropped, everything before it is kept, and the next
// mutation does not glue its record onto the torn one.
func TestATornLastRecordIsDropped(t *testing.T) {
	cases := []struct {
		name string
		tail string
	}{
		{"cut in the middle", `{"op":"comment","group":"default","comment":{"id":"c9","gro`},
		{"complete but unterminated", `{"op":"delAll"}`},
		{"a lone brace", `{`},
		{"terminated but not a record", "{\"op\":\"comm\x00\x00\x00\x00\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			s := NewStore(path)
			mutate(t, s, "add diff")
			mutate(t, s, "add comment")
			mutate(t, s, "add comment")
			want := stateOf(t, s)

			f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.tail); err != nil {
				t.Fatal(err)
			}
			f.Close()

			restarted := reloaded(t, path)
			if got := stateOf(t, restarted); got != want {
				t.Fatalf("after the restart the session is\n%s\nwant\n%s", got, want)
			}
			mutate(t, restarted, "add comment")
			if got, want := stateOf(t, reloaded(t, path)), stateOf(t, restarted); got != want {
				t.Errorf("the next mutation was lost behind the torn record:\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// A record in the middle of the log that cannot be read is damage, not a torn
// write, and is treated as the broken session file it is.
func TestADamagedRecordInTheMiddleIsBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	mutate(t, s, "add diff")
	mutate(t, s, "add comment")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	damaged := string(b) + "{not json}\n" + `{"op":"delAll"}` + "\n"
	if err := os.WriteFile(path, []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := NewStore(path).Load(); err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if kept, err := os.ReadFile(path + ".broken"); err != nil || string(kept) != damaged {
		t.Errorf("the damaged file was not kept: %v", err)
	}
}

// A failed write is reported by the request that caused it and not later, and
// a log that could not be appended to does not leave the session unsaved once
// the disk is back.
func TestAFailedAppendIsReportedAtOnce(t *testing.T) {
	logs := captureLogs(t)
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.json")
	s := NewStore(path)
	mutate(t, s, "add diff")
	mutate(t, s, "add comment")
	if err := s.PersistError(); err != nil {
		t.Fatalf("PersistError() = %v, want nil", err)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	mutate(t, s, "add comment")
	if err := s.PersistError(); err == nil {
		t.Fatal("PersistError() = nil right after a mutation that could not be saved")
	}
	if !strings.Contains(logs.String(), "the session is not being saved") {
		t.Errorf("the failure was not logged:\n%s", logs)
	}

	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mutate(t, s, "add comment")
	if err := s.PersistError(); err != nil {
		t.Fatalf("PersistError() = %v, want nil once the directory is back", err)
	}
	if got, want := stateOf(t, reloaded(t, path)), stateOf(t, s); got != want {
		t.Errorf("the file holds\n%s\nwant everything, the comments lost while the disk was gone included\n%s", got, want)
	}
}

// A file written by an sbnn from before the log is a snapshot with nothing
// after it, and no newline. It loads, and the first mutation after it does not
// glue a record onto the closing brace.
func TestAnOlderSessionFileGainsALog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	old := `{"version":1,"seq":2,"seqs":{"d":1},"groups":[{"name":"default","diffs":[{"id":"d1","title":"diff 1","files":[{"id":"f1"}]}],"comments":[]}],"rounds":{"default":1}}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := reloaded(t, path)
	for _, step := range []string{"add comment", "add comment", "edit comment"} {
		mutate(t, s, step)
		if got, want := stateOf(t, reloaded(t, path)), stateOf(t, s); got != want {
			t.Fatalf("after %s the file holds\n%s\nwant\n%s", step, got, want)
		}
	}
}

func BenchmarkMutation400Files(b *testing.B) {
	path := filepath.Join(b.TempDir(), "session.json")
	s := bigStore(b, path)
	st, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("session file: %d bytes", st.Size())
	g, _ := s.Group(DefaultGroup)
	c := &model.Comment{Group: DefaultGroup, DiffID: g.Diffs[0].ID, FileID: g.Diffs[0].Files[0].ID, Side: "new", StartLine: 1, EndLine: 1, Body: "x"}

	b.Run("AddComment", func(b *testing.B) {
		for b.Loop() {
			if _, err := s.AddComment(c); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("snapshot", func(b *testing.B) {
		for b.Loop() {
			s.mu.Lock()
			err := s.write()
			s.mu.Unlock()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
