package server

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

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
