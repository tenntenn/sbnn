package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/tenntenn/sbnn/internal/model"
)

// The session file is a snapshot followed by a log.
//
// The first JSON value is the whole session, exactly what sbnn used to write
// on every mutation, so a file written by an older sbnn is a snapshot with an
// empty log. Every line after it is one mutation, appended and fsynced before
// the request that made it returns. Rewriting 4.7 MB and syncing it for each
// comment cost about 15 ms at 400 files; appending the comment costs the sync
// of a few hundred bytes. Once the log has outgrown the snapshot it is folded
// back into a new snapshot, so the file stays within about twice the size of
// the session and the rewrite is paid for by the many appends before it.
//
// A record is acknowledged only after it is on disk, so a half-written last
// line was never acknowledged. Load drops it and the next mutation rewrites
// the file, which also clears whatever a failed append left behind.

// defaultCompactMin is the log size below which the snapshot is not rewritten,
// whatever its own size. A small session is cheap to rewrite, but it is also
// cheap to replay, and there is nothing to gain from folding a few kilobytes.
const defaultCompactMin = 1 << 20

// The operations a log record can hold.
const (
	// opDiff appends Diff to Group.
	opDiff = "diff"
	// opDelDiff removes the diff IDs[0] of Group, and its comments.
	opDelDiff = "delDiff"
	// opDelGroup removes Group.
	opDelGroup = "delGroup"
	// opDelAll removes every group.
	opDelAll = "delAll"
	// opComment adds Comment to Group, or replaces the one with its ID.
	opComment = "comment"
	// opDelComments removes the comments IDs of Group.
	opDelComments = "delComments"
	// opMeta sets what a group says about the review itself - when it was
	// submitted, the verdict, the hooks - and leaves its diffs and comments.
	opMeta = "meta"
)

// logRecord is one mutation. Each carries the counters as they stood after it,
// so replaying a record needs no knowledge of how ids and rounds are counted.
type logRecord struct {
	Op      string         `json:"op"`
	Group   string         `json:"group,omitempty"`
	Diff    *model.Diff    `json:"diff,omitempty"`
	Comment *model.Comment `json:"comment,omitempty"`
	IDs     []string       `json:"ids,omitempty"`
	Meta    *model.Group   `json:"meta,omitempty"`
	Seqs    map[string]int `json:"seqs,omitempty"`
	Rounds  map[string]int `json:"rounds,omitempty"`
}

// metaRecord is the record of a change to what g says about its review.
func metaRecord(g *model.Group) *logRecord {
	return &logRecord{Op: opMeta, Group: g.Name, Meta: &model.Group{
		ReviewedAt:    g.ReviewedAt,
		ReviewNote:    g.ReviewNote,
		ReviewVerdict: g.ReviewVerdict,
		Hooks:         g.Hooks,
	}}
}

// session is a session file taken apart.
type session struct {
	snap    persisted
	records []*logRecord
	// snapBytes and logBytes are the sizes of the two halves.
	snapBytes, logBytes int
	// dirty is set when the file is not fit to be appended to.
	dirty bool
}

// parseSession reads a session file. An error means the file is broken; a
// last record that was cut short is not one, because its request never
// returned.
func parseSession(b []byte) (*session, error) {
	var sess session
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&sess.snap); err != nil {
		return nil, err
	}
	if sess.snap.Version > persistVersion {
		// What follows is in a format this build cannot read, and Load says
		// so instead of calling the file broken.
		return &sess, nil
	}
	off := int(dec.InputOffset())
	rest := b[off:]
	sess.snapBytes = off
	if len(bytes.TrimSpace(rest)) == 0 {
		// Nothing but the snapshot. Without a newline after it the first
		// record would be glued to its last brace.
		sess.dirty = !bytes.HasPrefix(rest, []byte("\n"))
		return &sess, nil
	}
	if rest[0] != '\n' {
		return nil, fmt.Errorf("unexpected data after the snapshot: %w", errBroken)
	}
	rest = rest[1:]
	for len(rest) > 0 {
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			sess.dirty = true
			break
		}
		line := rest[:nl]
		rest = rest[nl+1:]
		if len(bytes.TrimSpace(line)) == 0 {
			sess.logBytes += nl + 1
			continue
		}
		var rec logRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			if len(rest) == 0 {
				// The last record, with its newline but not its contents:
				// the block was extended before the data reached it. It was
				// not acknowledged either.
				sess.dirty = true
				break
			}
			return nil, fmt.Errorf("record %d of the log: %w", len(sess.records)+1, err)
		}
		sess.records = append(sess.records, &rec)
		sess.logBytes += nl + 1
	}
	return &sess, nil
}

var errBroken = errors.New("the session file is broken")

// apply replays one record. The caller must hold the lock.
func (s *Store) apply(rec *logRecord) error {
	s.seq = rec.Seqs
	s.rounds = rec.Rounds
	g := s.group(rec.Group, false)
	switch rec.Op {
	case opDiff:
		if rec.Diff == nil {
			return fmt.Errorf("a %q record without a diff", rec.Op)
		}
		g = s.group(rec.Group, true)
		g.Diffs = append(g.Diffs, rec.Diff)
	case opDelDiff:
		if g == nil {
			return nil
		}
		g.Diffs = slices.DeleteFunc(g.Diffs, func(d *model.Diff) bool { return slices.Contains(rec.IDs, d.ID) })
		g.Comments = slices.DeleteFunc(g.Comments, func(c *model.Comment) bool { return slices.Contains(rec.IDs, c.DiffID) })
	case opDelGroup:
		s.groups = slices.DeleteFunc(s.groups, func(g *model.Group) bool { return g.Name == rec.Group })
	case opDelAll:
		s.groups = nil
	case opComment:
		if g == nil || rec.Comment == nil {
			return fmt.Errorf("a %q record for the unknown group %q", rec.Op, rec.Group)
		}
		i := slices.IndexFunc(g.Comments, func(c *model.Comment) bool { return c.ID == rec.Comment.ID })
		if i < 0 {
			g.Comments = append(g.Comments, rec.Comment)
		} else {
			g.Comments[i] = rec.Comment
		}
	case opDelComments:
		if g == nil {
			return nil
		}
		g.Comments = slices.DeleteFunc(g.Comments, func(c *model.Comment) bool { return slices.Contains(rec.IDs, c.ID) })
	case opMeta:
		if rec.Meta == nil {
			return fmt.Errorf("a %q record without a group", rec.Op)
		}
		g = s.group(rec.Group, true)
		g.ReviewedAt, g.ReviewNote, g.ReviewVerdict, g.Hooks = rec.Meta.ReviewedAt, rec.Meta.ReviewNote, rec.Meta.ReviewVerdict, rec.Meta.Hooks
	default:
		return fmt.Errorf("a record of the unknown kind %q: %w", rec.Op, errBroken)
	}
	return nil
}

// persist makes a mutation durable before it returns. rec says what changed;
// nil means the whole session has to be written. The caller must hold the lock.
//
// A failure is not fatal - the server keeps serving the session it holds in
// memory - but it must not pass unnoticed either, because everything written
// after it is lost on the next restart. So the reason is logged and kept for
// PersistError, which the status API reports.
func (s *Store) persist(rec *logRecord) {
	if s.path == "" || s.sealed {
		return
	}
	err := s.save(rec)
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

// save appends rec to the log, or writes a new snapshot when the file cannot
// be appended to or the log has outgrown the snapshot.
func (s *Store) save(rec *logRecord) error {
	if rec == nil || s.dirty || s.snapBytes == 0 || s.logBytes > max(s.snapBytes, s.compactMin) {
		return s.write()
	}
	if err := s.appendRecord(rec); err != nil {
		// A snapshot replaces the file whole, so it also gets rid of a
		// half-written record the failed append may have left.
		s.dirty = true
		return s.write()
	}
	return nil
}

// appendRecord adds rec to the end of the session file and syncs it.
func (s *Store) appendRecord(rec *logRecord) error {
	rec.Seqs, rec.Rounds = s.seq, s.rounds
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encoding the session: %w", err)
	}
	b = append(b, '\n')
	// No O_CREATE: a file that is gone is a snapshot's job, not an empty log.
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return fmt.Errorf("appending to %s: %w", s.path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("flushing %s: %w", s.path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", s.path, err)
	}
	s.logBytes += len(b)
	return nil
}

// write replaces the session file with a snapshot of the current session. The
// caller must hold the lock.
func (s *Store) write() (err error) {
	b, err := json.Marshal(persisted{Version: persistVersion, Seq: s.sharedSeq(), Seqs: s.seq, Groups: s.groups, Rounds: s.rounds})
	if err != nil {
		return fmt.Errorf("encoding the session: %w", err)
	}
	b = append(b, '\n')
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
	s.snapBytes, s.logBytes, s.dirty = len(b), 0, false
	return nil
}
