package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CheckInterval is how often the start-up check asks GitHub, and how often
// it tells the user about a newer release.
const CheckInterval = 24 * time.Hour

// CheckTimeout bounds the start-up check's one request. It runs next to the
// command rather than before it, so this is not a delay anyone waits for; it
// only keeps a hung connection from outliving a long command.
const CheckTimeout = 5 * time.Second

// Cache is what the start-up check remembers between runs, in the state
// directory.
type Cache struct {
	// CheckedAt is when GitHub was last asked, whether or not it answered.
	CheckedAt time.Time `json:"checkedAt"`
	// Latest is the latest release GitHub last named.
	Latest string `json:"latest,omitempty"`
	// NotifiedAt is when the user was last told about a newer release.
	NotifiedAt time.Time `json:"notifiedAt,omitzero"`
}

// LoadCache reads the cache at path. A cache that is missing or unreadable is
// an empty one: the worst that comes of it is one request.
func LoadCache(path string) Cache {
	var c Cache
	b, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Cache{}
	}
	return c
}

// SaveCache writes c to path, readable by its owner only, and atomically, so
// that two sbnn processes checking at once leave one of their caches rather
// than a mix of both.
func SaveCache(path string, c Cache) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Check is the start-up check: the question "is there a newer release?",
// asked at most once per CheckInterval and never in the way of the command
// it runs next to.
type Check struct {
	// Path is the cache file.
	Path string
	// Current is the running version.
	Current string
	// Latest asks GitHub for the latest release.
	Latest func(ctx context.Context) (string, error)
	// Now and Timeout are time.Now and CheckTimeout except in tests.
	Now     func() time.Time
	Timeout time.Duration

	done  chan struct{}
	cache Cache
}

// Start begins the check and returns at once. When the cache is fresh the
// answer is ready immediately and GitHub is not asked; otherwise it is asked
// on a goroutine, which records the attempt in the cache when it finishes.
// The goroutine is not waited for: a command that ends first simply ends,
// and the next run asks again.
func (c *Check) Start(ctx context.Context) {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Timeout == 0 {
		c.Timeout = CheckTimeout
	}
	c.done = make(chan struct{})
	c.cache = LoadCache(c.Path)
	now := c.Now()
	// A CheckedAt in the future is a clock that moved back, and trusting it
	// would put the next check off for as long as the clock was wrong.
	if age := now.Sub(c.cache.CheckedAt); age >= 0 && age < CheckInterval {
		close(c.done)
		return
	}
	go func() {
		defer close(c.done)
		ctx, cancel := context.WithTimeout(ctx, c.Timeout)
		defer cancel()
		latest, err := c.Latest(ctx)
		// A failed attempt is recorded too, keeping the release it last
		// knew of: offline, or out of API rate limit, the answer would be
		// the same on the next command, so asking again on every one only
		// spends the limit.
		c.cache.CheckedAt = now
		if err == nil {
			c.cache.Latest = latest
		}
		_ = SaveCache(c.Path, c.cache)
	}()
}

// Notice returns the newer release to tell the user about, if the check has
// an answer by now, the answer is newer than Current, and the user has not
// already been told within CheckInterval. It never waits.
func (c *Check) Notice() (latest string, ok bool) {
	if c.done == nil {
		return "", false
	}
	select {
	case <-c.done:
	default:
		return "", false
	}
	if !Newer(c.cache.Latest, c.Current) {
		return "", false
	}
	now := c.Now()
	if age := now.Sub(c.cache.NotifiedAt); age >= 0 && age < CheckInterval {
		return "", false
	}
	c.cache.NotifiedAt = now
	_ = SaveCache(c.Path, c.cache)
	return c.cache.Latest, true
}
