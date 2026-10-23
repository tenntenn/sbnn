package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// Nothing ends a review an agent forgot to close. A group stays on the server
// until someone runs --clear, and the server itself only goes away once it
// holds nothing at all, so a machine that has run sbnn for a few months
// carries dozens of reviews nobody will open again. Pruning closes the ones
// that have been left alone for long enough, by the same rule wherever the
// review lives: on the running server, or in the session file of a server
// that has stopped.

// The reasons a review that is old enough to prune is kept anyway, as the
// JSON of POST /_/api/prune names them. They are words rather than sentences
// so that a script can match them; the command line says them in full.
const (
	// KeepRecent is a review with activity since the cutoff. It is not a
	// reason to keep so much as the absence of one to prune.
	KeepRecent = "recent"
	// KeepOpenComments is a review holding comments nobody has resolved:
	// what a human wrote and nobody has acted on is the one thing in a
	// review that cannot be produced again.
	KeepOpenComments = "open-comments"
	// KeepHooks is a review with a hook registered: something asked to be
	// told when it is submitted, and closing the review would mean it never
	// is.
	KeepHooks = "hooks"
	// KeepNotListed is a review the request did not name. A prune that
	// asked someone first names the reviews they agreed to, and one that
	// became prunable while they were reading the question is not among
	// them.
	KeepNotListed = "not-listed"
)

// PruneKeepReason says why g is not pruned at before, or "" when it is.
//
// A review is old when nothing has happened to it since before (see
// model.Group.LastActivity). An old review is still kept while it holds open
// comments or a hook, unless force is set: age says nobody is looking, but an
// open comment says somebody once wanted something done, and only the person
// running prune can decide that it no longer matters.
func PruneKeepReason(g *model.Group, before time.Time, force bool) string {
	if !g.LastActivity().Before(before) {
		return KeepRecent
	}
	if force {
		return ""
	}
	if model.OpenThreads(g.Comments) > 0 {
		return KeepOpenComments
	}
	if len(g.Hooks) > 0 {
		return KeepHooks
	}
	return ""
}

// PruneRequest is the body of POST /_/api/prune.
type PruneRequest struct {
	// Before is the cutoff: a review whose last activity is older than this
	// is pruned. It is required, because the zero time would quietly mean
	// "nothing is old enough" and a prune that does nothing looks like one
	// that found nothing.
	Before time.Time `json:"before"`
	// Force prunes old reviews even when they hold open comments or hooks.
	Force bool `json:"force,omitempty"`
	// DryRun answers what would be pruned and changes nothing.
	DryRun bool `json:"dryRun,omitempty"`
	// Groups, when not empty, limits the prune to the reviews it names: those
	// a dry run listed and the user then agreed to. A review it does not name
	// is kept whatever its age.
	Groups []string `json:"groups,omitempty"`
}

// PrunedGroup describes one review a prune looked at.
type PrunedGroup struct {
	Name         string    `json:"name"`
	LastActivity time.Time `json:"lastActivity,omitzero"`
	Diffs        int       `json:"diffs"`
	Comments     int       `json:"comments"`
	Unresolved   int       `json:"unresolved"`
	Hooks        int       `json:"hooks"`
	// Reason is why the review was kept, one of the Keep constants. It is
	// empty for a review that was pruned.
	Reason string `json:"reason,omitempty"`
}

// PruneResponse is the answer to POST /_/api/prune. With DryRun set, Closed is
// what would have been closed.
type PruneResponse struct {
	Closed []PrunedGroup `json:"closed"`
	Kept   []PrunedGroup `json:"kept"`
}

// describePruned summarises g for a prune report.
func describePruned(g *model.Group, reason string) PrunedGroup {
	return PrunedGroup{
		Name:         g.Name,
		LastActivity: g.LastActivity(),
		Diffs:        len(g.Diffs),
		Comments:     len(model.Threads(g.Comments)),
		Unresolved:   model.OpenThreads(g.Comments),
		Hooks:        len(g.Hooks),
		Reason:       reason,
	}
}

// PruneGroups sorts the given groups into those a prune at before closes and
// those it keeps. It is the rule the server applies to its own groups, and
// the command line applies to the groups in the session file of a server
// that is no longer running.
func PruneGroups(groups []*model.Group, before time.Time, force bool) PruneResponse {
	res := PruneResponse{Closed: []PrunedGroup{}, Kept: []PrunedGroup{}}
	for _, g := range groups {
		reason := PruneKeepReason(g, before, force)
		if reason == "" {
			res.Closed = append(res.Closed, describePruned(g, ""))
			continue
		}
		res.Kept = append(res.Kept, describePruned(g, reason))
	}
	return res
}

// Prune closes the reviews that have been left alone since req.Before,
// keeping those PruneKeepReason keeps. The decision and the deletion are made under
// one lock, so a comment written while a prune runs either lands before it
// and keeps the review, or finds the review already gone - never a review
// judged idle and then closed under a comment that had just arrived.
//
// Each closed review is written to the session log as a deletion, exactly as
// DeleteGroup writes one.
func (s *Store) Prune(req PruneRequest) PruneResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := PruneGroups(s.groups, req.Before, req.Force)
	if len(req.Groups) > 0 {
		closed := res.Closed[:0]
		for _, p := range res.Closed {
			if slices.Contains(req.Groups, p.Name) {
				closed = append(closed, p)
				continue
			}
			p.Reason = KeepNotListed
			res.Kept = append(res.Kept, p)
		}
		res.Closed = closed
	}
	if req.DryRun || len(res.Closed) == 0 {
		return res
	}
	for _, p := range res.Closed {
		s.groups = slices.DeleteFunc(s.groups, func(g *model.Group) bool { return g.Name == p.Name })
		// A group that comes back is a new review, and starts at round 1.
		delete(s.rounds, p.Name)
		s.persist(&logRecord{Op: opDelGroup, Group: p.Name})
	}
	return res
}

// handlePrune closes the reviews nobody has touched since a cutoff.
//
// The command line could close them one DELETE at a time, but it would have
// to judge each review from a status it fetched earlier, and a comment
// written in between would be closed with the review it was meant to keep.
func (s *Server) handlePrune(w http.ResponseWriter, r *http.Request) {
	var req PruneRequest
	if !decodeBody(w, r, maxBodySize, "the prune request", &req) {
		return
	}
	if req.Before.IsZero() {
		http.Error(w, "before is required: the time a review must have been left alone since", http.StatusBadRequest)
		return
	}
	res := s.store.Prune(req)
	if !req.DryRun {
		for _, g := range res.Closed {
			s.forgetGroup(g.Name)
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// forgetGroup lets go of what the server keeps about a group outside the
// store, once the group itself is gone, and tells the pages that show it.
func (s *Server) forgetGroup(name string) {
	// The stored review notice outlives the group otherwise, and would be
	// replayed to a reconnecting browser for a group that no longer exists.
	s.broker.forgetReviews(name)
	s.prev.forget(name)
	s.notify(name)
}

// previewDir is the directory the reconstructed previews of a group, or of
// one diff of it, are written to. It is "" when there is no cache directory,
// and when a name would not stay one level down: the result is handed to
// os.RemoveAll, and a diff id of ".." out of a hand-edited session file would
// otherwise name every preview of every review.
func (p *previewer) previewDir(group string, diffID ...string) string {
	if p.cacheDir == "" {
		return ""
	}
	parts := []string{p.cacheDir, "preview"}
	for _, name := range append([]string{group}, diffID...) {
		seg := safeSegment(name)
		if seg == "" || seg == "." || seg == ".." {
			return ""
		}
		parts = append(parts, seg)
	}
	return filepath.Join(parts...)
}

// forget removes the previews reconstructed for a group, or for the given
// diffs of it, once they can no longer be asked for. Without it the cache
// directory only ever grew: every round of every review left its Markdown
// behind for good.
//
// The cache directory is shared by the servers on every port, so another
// server holding a group of the same name loses the preview files of it too.
// That costs nothing lasting: a preview is rebuilt from the diff whenever it
// is asked for and the file is missing.
func (p *previewer) forget(group string, diffIDs ...string) {
	if len(diffIDs) == 0 {
		removePreviewDir(p.previewDir(group))
		return
	}
	for _, id := range diffIDs {
		removePreviewDir(p.previewDir(group, id))
	}
	// The group directory itself goes once its last diff has.
	if dir := p.previewDir(group); dir != "" {
		os.Remove(dir)
	}
}

func removePreviewDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("could not remove the previews of a closed review", "dir", dir, "error", err)
	}
}

// ReadSession returns the reviews held in a session file, without touching
// the file.
//
// It is what lets prune judge the session of a server that is not running.
// Load is no use for that: it is written for the server that owns the file,
// and sets a file it cannot read aside, which is not something a command
// looking at another port's session has any business doing. A file this
// build cannot read is an error here, and the caller leaves it alone.
func ReadSession(path string) ([]*model.Group, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sess, err := parseSession(b)
	if err != nil {
		return nil, fmt.Errorf("session file %s is broken: %w", path, err)
	}
	if sess.snap.Version > persistVersion {
		return nil, fmt.Errorf("session file %s was written by a newer sbnn (format version %d, this one understands %d)",
			path, sess.snap.Version, persistVersion)
	}
	tmp := &Store{groups: slices.DeleteFunc(slices.Clone(sess.snap.Groups), func(g *model.Group) bool { return g == nil })}
	for _, rec := range sess.records {
		if err := tmp.apply(rec); err != nil {
			return nil, fmt.Errorf("session file %s is broken: %w", path, err)
		}
	}
	return tmp.groups, nil
}
