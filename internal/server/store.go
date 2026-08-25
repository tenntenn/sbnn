package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// DefaultGroup is the group diffs land in when no --target is given. It is
// served at the root path.
const DefaultGroup = "default"

var groupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateGroupName checks a group name coming from the CLI or the URL.
func ValidateGroupName(name string) (string, error) {
	if name == "" {
		return DefaultGroup, nil
	}
	if !groupNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid group name %q: use letters, digits, '.', '-' or '_' (max 64)", name)
	}
	return name, nil
}

// Store keeps the diffs and review comments of every group and persists them
// so that a restarted server comes back with the same session.
type Store struct {
	mu     sync.RWMutex
	path   string
	groups []*model.Group
	seq    int
	// persistErr is why the last write to path failed, if it did.
	persistErr error
}

// NewStore returns a store persisting to path. An empty path disables
// persistence, which is convenient in tests.
func NewStore(path string) *Store {
	return &Store{path: path}
}

type persisted struct {
	Version int            `json:"version"`
	Seq     int            `json:"seq"`
	Groups  []*model.Group `json:"groups"`
}

const persistVersion = 1

// Load restores the session from disk. A missing or unreadable file is not an
// error: sbnn simply starts with an empty session.
func (s *Store) Load() error {
	if s.path == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		// Starting empty on top of the old file means the next diff,
		// comment or hook renames a fresh session over it. A truncated
		// write from a killed server is exactly the case where the old
		// bytes are still worth having, so keep them.
		kept, moveErr := s.setAside()
		if moveErr != nil {
			return fmt.Errorf("session file %s is broken and could not be moved aside (%v): %w", s.path, moveErr, err)
		}
		return fmt.Errorf("session file %s is broken, so sbnn started a new session; "+
			"the old one was kept as %s: %w", s.path, kept, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups = p.Groups
	s.seq = p.Seq
	return nil
}

// setAside renames a session file sbnn refuses to load, so that the new
// session does not overwrite it, and returns where it went.
func (s *Store) setAside() (string, error) {
	kept := s.path + ".broken"
	if err := os.Rename(s.path, kept); err != nil {
		return "", err
	}
	return kept, nil
}

// persist writes the session to disk. The caller must hold the lock.
//
// A failure is not fatal - the server keeps serving the session it holds in
// memory - but it must not pass unnoticed either, because everything written
// after it is lost on the next restart. So the reason is logged and kept for
// PersistError, which the status API reports.
func (s *Store) persist() {
	if s.path == "" {
		return
	}
	err := s.write()
	if err != nil {
		// A full disk or a removed state directory keeps failing on every
		// comment, so only the first failure of a streak is logged; the
		// current reason is always available from PersistError.
		if s.persistErr == nil {
			slog.Warn("the session is not being saved", "file", s.path, "error", err)
		}
		s.persistErr = err
		return
	}
	if s.persistErr != nil {
		slog.Info("the session is being saved again", "file", s.path)
		s.persistErr = nil
	}
}

// write replaces the session file with the current session. The caller must
// hold the lock.
func (s *Store) write() (err error) {
	b, err := json.Marshal(persisted{Version: persistVersion, Seq: s.seq, Groups: s.groups})
	if err != nil {
		return fmt.Errorf("encoding the session: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return fmt.Errorf("creating a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(name)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("setting the mode of %s: %w", name, err)
	}
	// Flush before the rename: a crash right after an unsynced rename leaves
	// the session file in place but empty, which is worse than the old one.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("flushing %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", name, err)
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", name, s.path, err)
	}
	return nil
}

// PersistError reports why the session was last not written to disk, or nil
// when the session file is up to date.
func (s *Store) PersistError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistErr
}

func (s *Store) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s%d", prefix, s.seq)
}

// group returns the named group, creating it when create is set. The caller
// must hold the lock.
func (s *Store) group(name string, create bool) *model.Group {
	for _, g := range s.groups {
		if g.Name == name {
			return g
		}
	}
	if !create {
		return nil
	}
	g := &model.Group{Name: name}
	s.groups = append(s.groups, g)
	return g
}

// GroupNames returns the names of all groups, with the default group first.
func (s *Store) GroupNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.groups))
	for _, g := range s.groups {
		if g.Name == DefaultGroup {
			names = append([]string{g.Name}, names...)
			continue
		}
		names = append(names, g.Name)
	}
	return names
}

// GroupSummary is the lightweight view of a group used by --status and by
// the sidebar of the web UI.
type GroupSummary struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Diffs      int       `json:"diffs"`
	Files      int       `json:"files"`
	Comments   int       `json:"comments"`
	Unresolved int       `json:"unresolved"`
	ReviewedAt time.Time `json:"reviewedAt,omitzero"`
	// Reviewed is false once a diff arrives after the last submission.
	Reviewed bool `json:"reviewed"`
	Hooks    int  `json:"hooks"`
}

// Summary returns a summary of every group.
func (s *Store) Summary(baseURL string) []GroupSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupSummary, 0, len(s.groups))
	for _, g := range s.groups {
		sum := GroupSummary{
			Name:       g.Name,
			URL:        GroupURL(baseURL, g.Name),
			Diffs:      len(g.Diffs),
			ReviewedAt: g.ReviewedAt,
			Reviewed:   g.Reviewed(),
			Hooks:      len(g.Hooks),
		}
		for _, d := range g.Diffs {
			sum.Files += len(d.Files)
		}
		for _, c := range g.Comments {
			sum.Comments++
			if !c.Resolved {
				sum.Unresolved++
			}
		}
		if g.Name == DefaultGroup {
			out = append([]GroupSummary{sum}, out...)
			continue
		}
		out = append(out, sum)
	}
	return out
}

// GroupURL returns the URL a group is served at.
func GroupURL(baseURL, group string) string {
	if group == DefaultGroup || group == "" {
		return baseURL + "/"
	}
	return baseURL + "/" + group
}

// Group returns a copy of the named group.
func (s *Store) Group(name string) (*model.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.group(name, false)
	if g == nil {
		return nil, false
	}
	return clone(g), true
}

// AddDiff stores a parsed diff in a group and returns it with its ID filled
// in.
func (s *Store) AddDiff(group string, d *model.Diff) *model.Diff {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, true)
	d.ID = s.nextID("d")
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	if d.Title == "" {
		d.Title = fmt.Sprintf("diff %d", len(g.Diffs)+1)
	}
	g.Diffs = append(g.Diffs, d)
	s.persist()
	return clone(d)
}

// DeleteDiff removes a diff and the comments attached to it.
func (s *Store) DeleteDiff(group, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return false
	}
	found := false
	diffs := g.Diffs[:0]
	for _, d := range g.Diffs {
		if d.ID == id {
			found = true
			continue
		}
		diffs = append(diffs, d)
	}
	if !found {
		return false
	}
	g.Diffs = diffs
	comments := g.Comments[:0]
	for _, c := range g.Comments {
		if c.DiffID != id {
			comments = append(comments, c)
		}
	}
	g.Comments = comments
	s.persist()
	return true
}

// DeleteGroup removes a whole group.
func (s *Store) DeleteGroup(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	groups := s.groups[:0]
	found := false
	for _, g := range s.groups {
		if g.Name == name {
			found = true
			continue
		}
		groups = append(groups, g)
	}
	s.groups = groups
	if found {
		s.persist()
	}
	return found
}

// FileContext returns the diff and file identified by the given IDs.
func (s *Store) FileContext(group, diffID, fileID string) (*model.Diff, *model.File, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.group(group, false)
	if g == nil {
		return nil, nil, false
	}
	d := g.FindDiff(diffID)
	if d == nil {
		return nil, nil, false
	}
	f := d.FindFile(fileID)
	if f == nil {
		return nil, nil, false
	}
	return clone(d), clone(f), true
}

// SubmitReview records that the human is done looking, which is the event
// an agent waits for.
func (s *Store) SubmitReview(group, note string, verdict model.Verdict) (*model.Group, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return nil, false
	}
	g.ReviewedAt = time.Now()
	g.ReviewNote = note
	g.ReviewVerdict = verdict
	s.persist()
	return clone(g), true
}

// AddHook registers something to do when a review is submitted.
func (s *Store) AddHook(group string, h *model.Hook) (*model.Hook, error) {
	if h.Command == "" && h.URL == "" {
		return nil, fmt.Errorf("a hook needs a command or a url")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, true)
	for _, existing := range g.Hooks {
		// Re-registering the same thing (a second `sbnn --on-review ...` for
		// the same group) should not pile up duplicates.
		if existing.Command == h.Command && existing.URL == h.URL {
			return clone(existing), nil
		}
	}
	h.ID = s.nextID("h")
	h.CreatedAt = time.Now()
	g.Hooks = append(g.Hooks, h)
	s.persist()
	return clone(h), nil
}

// Hooks returns the hooks of a group.
func (s *Store) Hooks(group string) []*model.Hook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.group(group, false)
	if g == nil {
		return nil
	}
	out := make([]*model.Hook, 0, len(g.Hooks))
	for _, h := range g.Hooks {
		out = append(out, clone(h))
	}
	return out
}

// DeleteHooks drops the hooks of a group, or the one with the given ID, and
// returns how many were removed.
func (s *Store) DeleteHooks(group, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return 0
	}
	removed := 0
	kept := g.Hooks[:0]
	for _, h := range g.Hooks {
		if id == "" || h.ID == id {
			removed++
			continue
		}
		kept = append(kept, h)
	}
	g.Hooks = kept
	if removed > 0 {
		s.persist()
	}
	return removed
}

// DeleteAllGroups closes every review and returns how many went.
func (s *Store) DeleteAllGroups() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.groups)
	s.groups = nil
	if n > 0 {
		s.persist()
	}
	return n
}

// FindFileByPath locates a file inside a group by its path. diffID narrows
// the search to one diff; empty means the newest diff carrying that path,
// which is the one an agent just sent.
func (s *Store) FindFileByPath(group, diffID, path string) (*model.Diff, *model.File, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.group(group, false)
	if g == nil {
		return nil, nil, false
	}
	for i := len(g.Diffs) - 1; i >= 0; i-- {
		d := g.Diffs[i]
		if diffID != "" && d.ID != diffID {
			continue
		}
		for _, f := range d.Files {
			if f.Path() == path || f.OldPath == path {
				return clone(d), clone(f), true
			}
		}
	}
	return nil, nil, false
}

// AddComment stores a review comment.
func (s *Store) AddComment(c *model.Comment) (*model.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(c.Group, false)
	if g == nil {
		return nil, fmt.Errorf("unknown group %q", c.Group)
	}
	if d := g.FindDiff(c.DiffID); d == nil {
		return nil, fmt.Errorf("unknown diff %q", c.DiffID)
	}
	now := time.Now()
	c.ID = s.nextID("c")
	c.CreatedAt, c.UpdatedAt = now, now
	g.Comments = append(g.Comments, c)
	s.persist()
	return clone(c), nil
}

// CommentPatch carries the fields of a comment that can be edited. A nil
// field is left alone.
type CommentPatch struct {
	Body     *string
	Resolved *bool
	Question *bool
}

// UpdateComment edits a comment in place.
func (s *Store) UpdateComment(group, id string, patch CommentPatch) (*model.Comment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return nil, false
	}
	for _, c := range g.Comments {
		if c.ID != id {
			continue
		}
		if patch.Body != nil {
			c.Body = *patch.Body
		}
		if patch.Resolved != nil {
			c.Resolved = *patch.Resolved
		}
		if patch.Question != nil {
			c.Question = *patch.Question
		}
		c.UpdatedAt = time.Now()
		s.persist()
		return clone(c), true
	}
	return nil, false
}

// DeleteComment removes a single comment.
func (s *Store) DeleteComment(group, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return false
	}
	found := false
	comments := g.Comments[:0]
	for _, c := range g.Comments {
		if c.ID == id {
			found = true
			continue
		}
		comments = append(comments, c)
	}
	g.Comments = comments
	if found {
		s.persist()
	}
	return found
}

// ClearComments drops the comments of a group and returns how many were
// removed. When resolvedOnly is set, unresolved comments are kept.
func (s *Store) ClearComments(group string, resolvedOnly bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(group, false)
	if g == nil {
		return 0
	}
	removed := 0
	comments := g.Comments[:0]
	for _, c := range g.Comments {
		if !resolvedOnly || c.Resolved {
			removed++
			continue
		}
		comments = append(comments, c)
	}
	g.Comments = comments
	if removed > 0 {
		s.persist()
	}
	return removed
}

// Comments returns a copy of the comments of a group.
func (s *Store) Comments(group string) ([]*model.Comment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.group(group, false)
	if g == nil {
		return nil, false
	}
	out := make([]*model.Comment, 0, len(g.Comments))
	for _, c := range g.Comments {
		out = append(out, clone(c))
	}
	return out, true
}

// clone deep copies a value through JSON, which keeps callers from mutating
// stored state by accident.
func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}
