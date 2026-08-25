//go:build windows

package history

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// LOCKFILE_EXCLUSIVE_LOCK. Without LOCKFILE_FAIL_IMMEDIATELY the call
// waits for whoever holds the range to let go, which is what an append
// wants.
const lockfileExclusiveLock = 0x0002

// The whole range a file could ever cover, so that what is locked does not
// depend on how long the log already is.
const (
	allBytesLow  = uintptr(0xffffffff)
	allBytesHigh = uintptr(0xffffffff)
)

// lockForAppend takes an exclusive lock on the whole file and returns the
// release. Windows has no flock; LockFileEx over the entire range is the
// same promise, and it is held against other processes as well.
func lockForAppend(f *os.File) (func(), error) {
	var ol syscall.Overlapped
	r, _, err := procLockFileEx.Call(f.Fd(), lockfileExclusiveLock, 0,
		allBytesLow, allBytesHigh, uintptr(unsafe.Pointer(&ol)))
	if r == 0 {
		return nil, err
	}
	return func() {
		var ol syscall.Overlapped
		procUnlockFileEx.Call(f.Fd(), 0,
			allBytesLow, allBytesHigh, uintptr(unsafe.Pointer(&ol)))
	}, nil
}
