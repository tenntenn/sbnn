package server

import (
	"bytes"
	"io"
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
