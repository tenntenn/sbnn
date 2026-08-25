package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
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

// captureLogs redirects the default logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestPersistReportsFailureAndRecovery(t *testing.T) {
	logs := captureLogs(t)
	// The state directory is gone, which is what a removed or unwritable
	// $XDG_STATE_HOME/sbnn looks like to the running server.
	dir := filepath.Join(t.TempDir(), "state")
	s := NewStore(filepath.Join(dir, "session.json"))

	s.AddDiff(DefaultGroup, &model.Diff{Title: "first"})
	if err := s.PersistError(); err == nil {
		t.Fatal("PersistError() = nil after a write into a missing directory, want an error")
	}
	// A failure that goes on failing is logged once, not once per comment.
	s.AddDiff(DefaultGroup, &model.Diff{Title: "second"})
	if got := strings.Count(logs.String(), "the session is not being saved"); got != 1 {
		t.Errorf("warnings logged = %d, want 1:\n%s", got, logs.String())
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s.AddDiff(DefaultGroup, &model.Diff{Title: "third"})
	if err := s.PersistError(); err != nil {
		t.Fatalf("PersistError() = %v after the directory came back, want nil", err)
	}

	// Everything held in memory is on disk once writing works again.
	restored := NewStore(s.path)
	if err := restored.Load(); err != nil {
		t.Fatal(err)
	}
	g, ok := restored.Group(DefaultGroup)
	if !ok || len(g.Diffs) != 3 {
		t.Fatalf("restored group = %+v (ok=%v), want 3 diffs", g, ok)
	}
}

func TestPersistWritesAPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewStore(path)
	s.AddDiff(DefaultGroup, &model.Diff{Title: "kept"})
	if err := s.PersistError(); err != nil {
		t.Fatalf("PersistError() = %v, want nil", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("session file mode = %v, want 0600", got)
	}
	// The temporary file is renamed, never left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("state directory holds %d entries, want only the session file", len(entries))
	}
}

func TestStatusReportsAnUnsavedSession(t *testing.T) {
	captureLogs(t)
	sessionFile := filepath.Join(t.TempDir(), "state", "session.json")
	ts, _ := newTestServer(t, func(o *Options) { o.SessionFile = sessionFile })

	var st Status
	getJSON(t, ts.URL+"/_/api/status", &st)
	if st.SessionError != "" {
		t.Fatalf("sessionError = %q before any write, want empty", st.SessionError)
	}

	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	getJSON(t, ts.URL+"/_/api/status", &st)
	if st.SessionError == "" {
		t.Fatal("sessionError is empty although the session file cannot be written")
	}
	if !strings.Contains(st.SessionError, "session") {
		t.Errorf("sessionError = %q, want it to name the file it could not write", st.SessionError)
	}
}

func TestBrokenSessionFileIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	// Half a write from a killed server.
	broken := `{"version":1,"seq":2,"groups":[{"name":"default","diffs":[{"id":"d1"`
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	err := s.Load()
	if err == nil {
		t.Fatal("Load() = nil for a broken session file, want an error")
	}
	kept := path + ".broken"
	if !strings.Contains(err.Error(), kept) {
		t.Errorf("Load() error = %q, want it to say the file was kept as %s", err, kept)
	}
	if got, readErr := os.ReadFile(kept); readErr != nil || string(got) != broken {
		t.Fatalf("kept file = %q, %v, want the original bytes", got, readErr)
	}

	// The new session writes a new file and leaves the rescued one alone.
	s.AddDiff(DefaultGroup, &model.Diff{Title: "new"})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the new session was not written: %v", err)
	}
	if got, err := os.ReadFile(kept); err != nil || string(got) != broken {
		t.Fatalf("kept file after a new write = %q, %v, want the original bytes", got, err)
	}
}

func TestBrokenSessionFileIsReportedOnStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = prev }()

	srv, err := New(Options{SessionFile: path, Version: "test"})
	w.Close()
	os.Stderr = prev
	if err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	if _, err := io.Copy(&sb, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "is broken") {
		t.Errorf("stderr = %q, want it to report the broken session file", sb.String())
	}

	// The server starts, with an empty session.
	if names := srv.Store().GroupNames(); len(names) != 0 {
		t.Errorf("groups after a broken session file = %v, want none", names)
	}
}

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
