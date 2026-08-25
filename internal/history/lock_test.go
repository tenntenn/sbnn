package history

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// openLog opens the log the way Append does, so that a test holds its lock
// against a distinct open file description - which is what one server
// holds against another.
func openLog(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("opening the log = %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// TestLockForAppendIsExclusive holds lockForAppend to the one thing Append
// leans on: that no two writers are inside it at once. A lock that is
// taken but hands the file to everyone at the same time reads as a fix and
// is none, so this counts holders rather than looking at the log.
func TestLockForAppendIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	if !fsLocksAreEnforced(t, path) {
		t.Skip("the filesystem under the test directory hands out no locks")
	}

	const writers = 6
	var (
		mu     sync.Mutex
		inside int
		most   int
	)
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				t.Errorf("opening the log = %v", err)
				return
			}
			defer f.Close()
			unlock, err := lockForAppend(f)
			if err != nil {
				t.Errorf("lockForAppend = %v", err)
				return
			}
			defer unlock()

			mu.Lock()
			inside++
			if inside > most {
				most = inside
			}
			mu.Unlock()

			// Long enough that anyone let in alongside is seen.
			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			inside--
			mu.Unlock()
		})
	}
	wg.Wait()

	if most != 1 {
		t.Errorf("%d writers held the append lock at once, want 1", most)
	}
}

// TestAppendWaitsForTheAppendLock is the regression test for the bug
// itself: that Append takes the log's lock before it writes.
//
// It cannot be caught by writing from several goroutines and reading the
// log back. On Linux a write(2) to a regular file is serialised by the
// inode lock, so records stay whole with no lock of ours at all, and the
// log parses either way - the corruption the lock is against wants a
// filesystem, or a page cache, that this test does not get to choose. So
// the test asks the question directly: someone else is holding the lock,
// and Append must not get past them.
func TestAppendWaitsForTheAppendLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviews.jsonl")
	if !fsLocksAreEnforced(t, path) {
		t.Skip("the filesystem under the test directory hands out no locks")
	}

	held := openLog(t, path)
	unlock, err := lockForAppend(held)
	if err != nil {
		t.Fatalf("lockForAppend = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- Append(path, Record{Group: "api", ReviewedAt: time.Unix(0, 0).UTC()})
	}()

	select {
	case err := <-done:
		unlock()
		t.Fatalf("Append returned (%v) while another writer held the log's lock: it writes without taking it", err)
	case <-time.After(300 * time.Millisecond):
		// Waiting, which is the whole point.
	}

	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Append = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Append never returned once the lock was let go")
	}

	records, err := Load(path, Filter{})
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if len(records) != 1 || records[0].Group != "api" {
		t.Errorf("log holds %v, want the one record Append was given", records)
	}
}
