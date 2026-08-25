//go:build !windows

package history

import (
	"os"
	"syscall"
	"testing"
)

// fsLocksAreEnforced reports whether the filesystem holding path hands out
// locks that are held against a second opener.
//
// It asks the platform directly rather than through lockForAppend, so that
// a lockForAppend which quietly stopped locking fails the tests below
// instead of skipping them.
func fsLocksAreEnforced(t *testing.T, path string) bool {
	t.Helper()
	first, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the log = %v", err)
	}
	defer first.Close()
	if err := syscall.Flock(int(first.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false
	}
	defer syscall.Flock(int(first.Fd()), syscall.LOCK_UN)

	second, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the log = %v", err)
	}
	defer second.Close()
	if err := syscall.Flock(int(second.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	syscall.Flock(int(second.Fd()), syscall.LOCK_UN)
	return false
}
