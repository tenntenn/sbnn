package server

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

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
