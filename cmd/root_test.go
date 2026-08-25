package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestReadDiffStatFailure pins the bug this file was added for: a stat that
// fails used to be answered with ("", nil), which is the same answer as "the
// user piped nothing in", so sbnn printed a review URL and exited 0 without
// sending the diff.
func TestReadDiffStatFailure(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("os.Open(%q): %v", os.DevNull, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := readDiff(f)
	if err == nil {
		t.Fatalf("readDiff(closed file) = %q, nil; want an error", got)
	}
	if got != "" {
		t.Errorf("readDiff(closed file) content = %q; want empty", got)
	}
	if !strings.Contains(err.Error(), "cannot inspect stdin") {
		t.Errorf("readDiff(closed file) error = %v; want it to mention stdin", err)
	}
}

func TestReadDiffPipe(t *testing.T) {
	const content = "diff text"

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	if _, err := io.WriteString(w, content); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	got, err := readDiff(r)
	if err != nil {
		t.Fatalf("readDiff: %v", err)
	}
	if got != content {
		t.Errorf("readDiff = %q; want %q", got, content)
	}
}

func TestReadDiffTooLarge(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	// A pipe holds far less than maxDiffSize, so the writer has to run
	// alongside the read.
	go func() {
		defer w.Close()
		io.Copy(w, io.LimitReader(zeroReader{}, maxDiffSize+1))
	}()

	got, err := readDiff(r)
	if err == nil {
		t.Fatalf("readDiff(%d bytes) = %d bytes, nil; want an error", maxDiffSize+1, len(got))
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("readDiff error = %v; want it to say the diff is too large", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }
