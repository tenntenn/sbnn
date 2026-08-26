package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tenntenn/sbnn/internal/model"
)

// DefaultGroup is the group diffs land in when no --target is given. It is
// served at the root path.
const DefaultGroup = "default"

var groupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// roundTitlePattern matches a default round title, so a session file written
// before rounds were counted can say how far its groups had got.
var roundTitlePattern = regexp.MustCompile(`^diff (\d+)$`)

// maxTitleLen bounds a round title, in runes.
const maxTitleLen = 120

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
	// seq counts the ids handed out, one counter per prefix.
	seq map[string]int
	// rounds counts the rounds a group has held, per group name. It only
	// goes up, so two rounds of one review never share a default title.
	rounds map[string]int
	// persistErr is why the last write to path failed, if it did.
	persistErr error
	// sealed is set when Load refused the session file. The bytes on disk
	// are the only copy of a session this build cannot read, so nothing may
	// write over them - not even the write() that reports through
	// persistErr, because a sealed store must not touch the file at all.
	sealed bool
}

// NewStore returns a store persisting to path. An empty path disables
// persistence, which is convenient in tests.
func NewStore(path string) *Store {
	return &Store{path: path}
}

type persisted struct {
	Version int `json:"version"`
	// Seq is the single counter sbnn used before the counters were split
	// per prefix. It is still written, high enough that an older sbnn
	// reading this file cannot hand out an id that is already taken.
	Seq int `json:"seq"`
	// Seqs holds one counter per id prefix.
	Seqs   map[string]int `json:"seqs,omitempty"`
	Groups []*model.Group `json:"groups"`
	// Rounds is how many rounds each group has held.
	Rounds map[string]int `json:"rounds,omitempty"`
}

const persistVersion = 1

// The id prefixes. Every kind of object counts on its own, so the first diff
// of a session is d1 even when a hook was registered before it - which is
// what `git diff | sbnn --on-review ...` does.
const (
	diffPrefix    = "d"
	commentPrefix = "c"
	hookPrefix    = "h"
)

// idPrefixes is every prefix nextID is called with.
var idPrefixes = []string{diffPrefix, commentPrefix, hookPrefix}

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
	// A file from a newer sbnn may hold fields this build knows nothing
	// about. Loading it as far as the JSON tags happen to line up would turn
	// a format change into silently missing diffs and comments, so refuse it
	// and say which version wrote it.
	if p.Version > persistVersion {
		// Refusing the file is only half the job: the server logs the error
		// and keeps running, so the very first diff would otherwise persist
		// an empty session over the file we just declined to read. Seal the
		// store instead, which keeps the bytes on disk intact and makes the
		// advice below something the reader can still act on.
		//
		// Moving the file aside is advice for a stopped sbnn. A running one
		// stays sealed whatever happens to the path, because it still cannot
		// read what it refused and has nothing to merge a new session into,
		// so the sentence says to restart rather than leaving the reader to
		// find out that the diffs went nowhere.
		refused := fmt.Errorf("session file %s was written by a newer sbnn (format version %d, this one understands %d): "+
			"this session is not saved and the file is left untouched; "+
			"upgrade sbnn, or move the file aside and restart sbnn to start a new session", s.path, p.Version, persistVersion)
		s.mu.Lock()
		s.sealed = true
		// persist() returns before write() for a sealed store, so nothing
		// after this ever sets persistErr. Status.SessionError is why the
		// session is not on disk and is empty only while the file is up to
		// date, so leaving it empty here would tell a reader the session was
		// saved when it is in memory and nowhere else.
		s.persistErr = refused
		s.mu.Unlock()
		return refused
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups = validGroups(s.path, p.Groups)
	s.seq = p.Seqs
	if s.seq == nil {
		// A file written before the counters were split apart carries one
		// shared counter. Starting every prefix at that value skips a few
		// numbers, but no id the old session handed out can come back.
		s.seq = make(map[string]int, len(idPrefixes))
		for _, prefix := range idPrefixes {
			s.seq[prefix] = p.Seq
		}
	}
	s.rounds = p.Rounds
	if s.rounds == nil {
		// A file written before the rounds were counted has to carry on
		// from the highest number it already used, or the next round
		// repeats a title.
		s.rounds = make(map[string]int, len(s.groups))
		for _, g := range s.groups {
			s.rounds[g.Name] = roundsSoFar(g)
		}
	}
	return nil
}

// validGroups drops the groups of a session file whose name sbnn would never
// have accepted from the CLI or the URL.
//
// The session file is a plain JSON file in a user-writable directory, and a
// hand edit, a partial write or a format change can put anything in it. A
// name that fails ValidateGroupName cannot be read, deleted or linked to
// afterwards - the router normalises the path, the handlers validate, and
// GroupURL builds a broken link - so it would sit in every listing with no
// way to get rid of it short of --clear --all.
func validGroups(path string, groups []*model.Group) []*model.Group {
	kept := make([]*model.Group, 0, len(groups))
	for _, g := range groups {
		// A JSON null in the groups array unmarshals to a nil element, and
		// the same hand edits and partial writes this function exists for
		// are what produce one. Reading g.Name would panic in Load, which
		// runs on server.New's goroutine and would take the process down
		// before it ever listened.
		if g == nil {
			continue
		}
		// ValidateGroupName maps the empty name to the default group, but a
		// stored group with no name is as unreachable as an invalid one.
		if _, err := ValidateGroupName(g.Name); err != nil || g.Name == "" {
			slog.Warn("dropping a group the session file should not contain",
				"file", path, "group", g.Name, "reason", "the name cannot be used")
			continue
		}
		kept = append(kept, g)
	}
	return kept
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
	if s.path == "" || s.sealed {
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
	b, err := json.Marshal(persisted{Version: persistVersion, Seq: s.sharedSeq(), Seqs: s.seq, Groups: s.groups, Rounds: s.rounds})
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

// roundsSoFar guesses how many rounds a group loaded from an older session
// file has held: the diffs it holds, or the highest number a default title
// carries when rounds have been deleted since.
func roundsSoFar(g *model.Group) int {
	n := len(g.Diffs)
	for _, d := range g.Diffs {
		m := roundTitlePattern.FindStringSubmatch(d.Title)
		if m == nil {
			continue
		}
		if v, err := strconv.Atoi(m[1]); err == nil && v > n {
			n = v
		}
	}
	return n
}

// nextRound counts one more round of a group. The caller must hold the lock.
func (s *Store) nextRound(group string) int {
	if s.rounds == nil {
		s.rounds = make(map[string]int)
	}
	s.rounds[group]++
	return s.rounds[group]
}

// cleanTitle makes a title given from outside fit to show: no surrounding
// space, no line breaks or other control characters, and no longer than
// maxTitleLen runes. Titles land in the sidebar, in the tab strip and under
// every comment in the prompt an agent reads.
func cleanTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if r := []rune(title); len(r) > maxTitleLen {
		title = strings.TrimRight(string(r[:maxTitleLen-1]), " ") + "\u2026"
	}
	return title
}

func (s *Store) nextID(prefix string) string {
	if s.seq == nil {
		s.seq = make(map[string]int, len(idPrefixes))
	}
	s.seq[prefix]++
	return fmt.Sprintf("%s%d", prefix, s.seq[prefix])
}

// sharedSeq is what the pre-split "seq" field is written as: the highest of
// the per-prefix counters, so an sbnn old enough to read only that field
// carries on without reusing an id. The caller must hold the lock.
func (s *Store) sharedSeq() int {
	highest := 0
	for _, n := range s.seq {
		if n > highest {
			highest = n
		}
	}
	return highest
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
	d.ID = s.nextID(diffPrefix)
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	// The round number counts every round the group has held, deleted ones
	// included: "diff 3" means the third round of this review, which is what
	// a reader takes it to mean, and two rounds never share a title.
	round := s.nextRound(group)
	d.Title = cleanTitle(d.Title)
	if d.Title == "" {
		d.Title = fmt.Sprintf("diff %d", round)
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
		// A group that comes back is a new review, and starts at round 1.
		delete(s.rounds, name)
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
	h.ID = s.nextID(hookPrefix)
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
	s.rounds = nil
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
	for _, d := range slices.Backward(g.Diffs) {
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
	c.ID = s.nextID(commentPrefix)
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
