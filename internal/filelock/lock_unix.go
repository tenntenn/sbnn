//go:build !windows

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// Lock takes an exclusive advisory lock on the whole file and returns the
// release. A flock belongs to the open file description, so two processes -
// or two opens inside one - wait for each other the way a writer needs them
// to.
func Lock(f *os.File) (func(), error) {
	fd := int(f.Fd())
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX)
		switch {
		case err == nil:
			return func() { syscall.Flock(fd, syscall.LOCK_UN) }, nil
		case errors.Is(err, syscall.EINTR):
			// A signal arrived while waiting. Ask again.
		case noLocksHere(err):
			// Some filesystems have no locks to hand out. A write
			// made without one beats a write refused.
			return func() {}, nil
		default:
			return nil, err
		}
	}
}

// noLocksHere reports whether the failure means the filesystem does not do
// locking, rather than that this lock could not be had.
func noLocksHere(err error) bool {
	return errors.Is(err, syscall.ENOLCK) ||
		errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EINVAL)
}
