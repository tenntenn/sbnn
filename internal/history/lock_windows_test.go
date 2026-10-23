//go:build windows

package history

import (
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// The calls the probe below makes, declared here because the lock itself
// lives in internal/filelock now and the probe must not go through it.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock = 0x0002
	allBytesLow           = uintptr(0xffffffff)
	allBytesHigh          = uintptr(0xffffffff)
)

// LOCKFILE_FAIL_IMMEDIATELY, so that the probe is turned away rather than
// left waiting for the lock the first handle is holding.
const lockfileFailImmediately = 0x0001

// fsLocksAreEnforced reports whether the filesystem holding path hands out
// locks that are held against a second opener.
//
// It asks the platform directly rather than through lockForAppend, so that
// a lockForAppend which quietly stopped locking fails the tests below
// instead of skipping them.
func fsLocksAreEnforced(t *testing.T, path string) bool {
	t.Helper()
	lock := func(f *os.File, flags uintptr) bool {
		var ol syscall.Overlapped
		r, _, _ := procLockFileEx.Call(f.Fd(), flags, 0,
			allBytesLow, allBytesHigh, uintptr(unsafe.Pointer(&ol)))
		return r != 0
	}
	unlock := func(f *os.File) {
		var ol syscall.Overlapped
		procUnlockFileEx.Call(f.Fd(), 0,
			allBytesLow, allBytesHigh, uintptr(unsafe.Pointer(&ol)))
	}

	first, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the log = %v", err)
	}
	defer first.Close()
	if !lock(first, lockfileExclusiveLock|lockfileFailImmediately) {
		return false
	}
	defer unlock(first)

	second, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the log = %v", err)
	}
	defer second.Close()
	if !lock(second, lockfileExclusiveLock|lockfileFailImmediately) {
		return true
	}
	unlock(second)
	return false
}
