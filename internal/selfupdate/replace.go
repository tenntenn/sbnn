package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// NotWritableError says that the executable cannot be replaced because the
// directory it is in cannot be written to by this user.
type NotWritableError struct {
	Dir string
	Err error
}

func (e *NotWritableError) Error() string {
	return fmt.Sprintf("cannot write to %s, where sbnn is installed (%v); run sbnn update as a user who can write there, or download the release by hand from https://github.com/%s/releases",
		e.Dir, e.Err, Repo)
}

func (e *NotWritableError) Unwrap() error { return e.Err }

// OldPath is where Replace moves the executable it replaces on Windows.
func OldPath(exe string) string {
	return exe + ".old"
}

// CleanupOld removes what a replace on Windows left behind. Windows does not
// let a running executable be deleted, only renamed, so the old binary
// outlives the sbnn update that replaced it and is removed by the next sbnn
// to run. A failure is ignored: the file is in the way of nothing, and the
// next run tries again.
func CleanupOld(exe string) {
	_ = os.Remove(OldPath(exe))
}

// Replace puts the binary write produces in place of exe, which must be the
// real path of the running executable with its symlinks resolved.
//
// The new binary is written to a temporary file in the same directory, so
// that the final rename stays on one file system and is atomic: whoever runs
// exe at any moment gets either the old binary or the whole new one, never a
// half-written file. It gets exe's permissions. On Windows the running
// executable cannot be overwritten, so it is renamed to OldPath first and put
// back if the new one cannot take its place. goos is runtime.GOOS except in
// tests, which exercise the Windows path everywhere.
func Replace(exe, goos string, write func(w io.Writer) error) (err error) {
	info, err := os.Stat(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(exe)+".new-*")
	if err != nil {
		return notWritable(dir, err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	if goos != "windows" {
		if err := os.Rename(tmp.Name(), exe); err != nil {
			return notWritable(dir, err)
		}
		return nil
	}
	old := OldPath(exe)
	if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the binary an earlier update left at %s: %w", old, err)
	}
	if err := os.Rename(exe, old); err != nil {
		return notWritable(dir, err)
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		if rerr := os.Rename(old, exe); rerr != nil {
			return fmt.Errorf("installing the new binary: %w; and putting the old one back from %s: %v", err, old, rerr)
		}
		return notWritable(dir, err)
	}
	return nil
}

func notWritable(dir string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return &NotWritableError{Dir: dir, Err: err}
	}
	return err
}
