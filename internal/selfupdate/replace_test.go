package selfupdate

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeString(s string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	}
}

func TestReplace(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "sbnn")
			if err := os.WriteFile(exe, []byte("old"), 0o751); err != nil {
				t.Fatal(err)
			}
			if err := Replace(exe, goos, writeString("new")); err != nil {
				t.Fatalf("Replace() = %v", err)
			}
			b, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != "new" {
				t.Errorf("exe holds %q, want %q", b, "new")
			}
			if runtime.GOOS != "windows" {
				info, err := os.Stat(exe)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != 0o751 {
					t.Errorf("mode = %v, want the old binary's 0751", got)
				}
			}
			_, err = os.Stat(OldPath(exe))
			if goos == "windows" {
				if err != nil {
					t.Errorf("the old binary was not kept aside: %v", err)
				}
				CleanupOld(exe)
				if _, err := os.Stat(OldPath(exe)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("CleanupOld left %s: %v", OldPath(exe), err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("an old binary was left at %s", OldPath(exe))
			}
			assertOnly(t, dir, "sbnn")
		})
	}
}

// assertOnly checks that dir holds exactly the named files, so that no
// temporary file is left behind.
func assertOnly(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if len(got) != len(names) {
		t.Fatalf("%s holds %v, want %v", dir, got, names)
	}
	for i := range names {
		if got[i] != names[i] {
			t.Fatalf("%s holds %v, want %v", dir, got, names)
		}
	}
}

func TestReplaceFailedWriteLeavesTheOldBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sbnn")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("checksum or archive trouble")
	err := Replace(exe, runtime.GOOS, func(w io.Writer) error {
		io.WriteString(w, "half")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Replace() = %v, want %v", err, boom)
	}
	b, _ := os.ReadFile(exe)
	if string(b) != "old" {
		t.Errorf("exe holds %q after a failed replace, want the old binary", b)
	}
	assertOnly(t, dir, "sbnn")
}

func TestReplaceNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not stop writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "sbnn")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	err := Replace(exe, runtime.GOOS, writeString("new"))
	var nw *NotWritableError
	if !errors.As(err, &nw) {
		t.Fatalf("Replace() = %v, want a NotWritableError", err)
	}
	if nw.Dir != dir {
		t.Errorf("NotWritableError.Dir = %q, want %q", nw.Dir, dir)
	}
}
