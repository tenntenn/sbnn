package history

import (
	"os"

	"github.com/tenntenn/sbnn/internal/filelock"
)

// lockForAppend takes an exclusive advisory lock on the whole log and
// returns the release. It is filelock.Lock under the name Append and its
// tests have always used.
func lockForAppend(f *os.File) (func(), error) {
	return filelock.Lock(f)
}
