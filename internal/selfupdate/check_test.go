package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// waitDone waits for the check's goroutine, which only a test does: sbnn
// itself never waits for it.
func waitDone(t *testing.T, c *Check) {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the check never finished")
	}
}

func TestCheck(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name       string
		cache      *Cache
		latest     string
		latestErr  error
		wantAsk    bool
		wantNotice string
		wantCache  Cache
	}{
		{
			name:       "no cache asks and tells",
			latest:     "v1.1.0",
			wantAsk:    true,
			wantNotice: "v1.1.0",
			wantCache:  Cache{CheckedAt: now, Latest: "v1.1.0", NotifiedAt: now},
		},
		{
			name:      "no newer release says nothing",
			latest:    "v1.0.0",
			wantAsk:   true,
			wantCache: Cache{CheckedAt: now, Latest: "v1.0.0"},
		},
		{
			name:       "a fresh cache is not asked again but still tells",
			cache:      &Cache{CheckedAt: now.Add(-time.Hour), Latest: "v1.1.0"},
			wantNotice: "v1.1.0",
			wantCache:  Cache{CheckedAt: now.Add(-time.Hour), Latest: "v1.1.0", NotifiedAt: now},
		},
		{
			name:      "told within the interval says nothing",
			cache:     &Cache{CheckedAt: now.Add(-time.Hour), Latest: "v1.1.0", NotifiedAt: now.Add(-time.Hour)},
			wantCache: Cache{CheckedAt: now.Add(-time.Hour), Latest: "v1.1.0", NotifiedAt: now.Add(-time.Hour)},
		},
		{
			name:       "a stale cache asks again",
			cache:      &Cache{CheckedAt: now.Add(-25 * time.Hour), Latest: "v1.0.0"},
			latest:     "v1.2.0",
			wantAsk:    true,
			wantNotice: "v1.2.0",
			wantCache:  Cache{CheckedAt: now, Latest: "v1.2.0", NotifiedAt: now},
		},
		{
			name:      "a check in the future is not trusted",
			cache:     &Cache{CheckedAt: now.Add(48 * time.Hour), Latest: "v1.0.0"},
			latest:    "v1.0.0",
			wantAsk:   true,
			wantCache: Cache{CheckedAt: now, Latest: "v1.0.0"},
		},
		{
			name:       "a failure is recorded and keeps what was known",
			cache:      &Cache{CheckedAt: now.Add(-25 * time.Hour), Latest: "v1.1.0"},
			latestErr:  errors.New("offline"),
			wantAsk:    true,
			wantNotice: "v1.1.0",
			wantCache:  Cache{CheckedAt: now, Latest: "v1.1.0", NotifiedAt: now},
		},
		{
			name:      "a failure with nothing known says nothing",
			latestErr: errors.New("rate limited"),
			wantAsk:   true,
			wantCache: Cache{CheckedAt: now},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "update-check.json")
			if tt.cache != nil {
				if err := SaveCache(path, *tt.cache); err != nil {
					t.Fatal(err)
				}
			}
			var asked atomic.Bool
			c := &Check{
				Path:    path,
				Current: "1.0.0",
				Now:     func() time.Time { return now },
				Latest: func(context.Context) (string, error) {
					asked.Store(true)
					return tt.latest, tt.latestErr
				},
			}
			c.Start(t.Context())
			waitDone(t, c)
			if asked.Load() != tt.wantAsk {
				t.Errorf("asked GitHub = %v, want %v", asked.Load(), tt.wantAsk)
			}
			got, ok := c.Notice()
			if ok != (tt.wantNotice != "") || got != tt.wantNotice {
				t.Errorf("Notice() = %q, %v, want %q", got, ok, tt.wantNotice)
			}
			if _, ok := c.Notice(); ok {
				t.Error("a second Notice() told again")
			}
			if cache := LoadCache(path); !cache.CheckedAt.Equal(tt.wantCache.CheckedAt) ||
				cache.Latest != tt.wantCache.Latest || !cache.NotifiedAt.Equal(tt.wantCache.NotifiedAt) {
				t.Errorf("cache = %+v, want %+v", cache, tt.wantCache)
			}
		})
	}
}

func TestCheckNeverWaits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	release := make(chan struct{})
	c := &Check{
		Path:    path,
		Current: "1.0.0",
		Latest: func(ctx context.Context) (string, error) {
			select {
			case <-release:
				return "v2.0.0", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	}
	c.Start(t.Context())
	start := time.Now()
	if _, ok := c.Notice(); ok {
		t.Error("Notice() told before the check answered")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Notice() took %v; it must not wait", d)
	}
	close(release)
	waitDone(t, c)
	if got, ok := c.Notice(); !ok || got != "v2.0.0" {
		t.Errorf("Notice() once answered = %q, %v", got, ok)
	}
}

func TestCheckTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	c := &Check{
		Path:    path,
		Current: "1.0.0",
		Timeout: 10 * time.Millisecond,
		Latest: func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	c.Start(t.Context())
	waitDone(t, c)
	if _, ok := c.Notice(); ok {
		t.Error("a check that timed out told about a release")
	}
	if cache := LoadCache(path); cache.CheckedAt.IsZero() {
		t.Error("the timed-out attempt was not recorded")
	}
}

func TestSaveCachePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permissions on Windows")
	}
	path := filepath.Join(t.TempDir(), "update-check.json")
	if err := SaveCache(path, Cache{Latest: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("cache mode = %v, want 0600", got)
	}
}

func TestLoadCacheIgnoresGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := LoadCache(path); !c.CheckedAt.IsZero() || c.Latest != "" {
		t.Errorf("LoadCache() of garbage = %+v", c)
	}
}
