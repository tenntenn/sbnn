package server

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

func TestPruneKeepReason(t *testing.T) {
	cutoff := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	old, recent := cutoff.Add(-48*time.Hour), cutoff.Add(time.Hour)

	open := []*model.Comment{{CreatedAt: old, UpdatedAt: old}}
	settled := []*model.Comment{{CreatedAt: old, UpdatedAt: old, Resolved: true}}
	hooks := []*model.Hook{{CreatedAt: old}}
	diffs := func(at time.Time) []*model.Diff { return []*model.Diff{{CreatedAt: at}} }

	for _, tt := range []struct {
		name  string
		group model.Group
		force bool
		want  string
	}{
		{"an old review is pruned", model.Group{Diffs: diffs(old)}, false, ""},
		{"a review with nothing in it is pruned", model.Group{}, false, ""},
		{"a recent review is kept", model.Group{Diffs: diffs(recent)}, false, KeepRecent},
		{"force does not take a recent review", model.Group{Diffs: diffs(recent)}, true, KeepRecent},
		{"exactly at the cutoff is not older than it", model.Group{Diffs: diffs(cutoff)}, false, KeepRecent},
		{"an old comment resolved recently makes it recent", model.Group{
			Diffs:    diffs(old),
			Comments: []*model.Comment{{CreatedAt: old, UpdatedAt: recent, Resolved: true}},
		}, false, KeepRecent},
		{"an open comment keeps an old review", model.Group{Diffs: diffs(old), Comments: open}, false, KeepOpenComments},
		{"resolved comments do not", model.Group{Diffs: diffs(old), Comments: settled}, false, ""},
		{"a hook keeps an old review", model.Group{Diffs: diffs(old), Hooks: hooks}, false, KeepHooks},
		{"open comments are named before hooks", model.Group{Diffs: diffs(old), Comments: open, Hooks: hooks}, false, KeepOpenComments},
		{"force closes a review with open comments", model.Group{Diffs: diffs(old), Comments: open}, true, ""},
		{"force closes a review with a hook", model.Group{Diffs: diffs(old), Hooks: hooks}, true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := PruneKeepReason(&tt.group, cutoff, tt.force); got != tt.want {
				t.Errorf("PruneKeepReason = %q, want %q", got, tt.want)
			}
		})
	}
}

// seedPruneStore fills s with one review of each kind prune tells apart, the
// old ones dated long before cutoff. Times the store stamps itself are moved
// back by hand, which is what weeks of leaving a review alone would do.
func seedPruneStore(t *testing.T, s *Store, cutoff time.Time) {
	t.Helper()
	old := cutoff.Add(-30 * 24 * time.Hour)
	raw := sampleDiff
	add := func(group string, at time.Time) *model.Diff {
		return s.AddDiff(group, &model.Diff{Raw: raw, Files: diff.Parse(raw), CreatedAt: at})
	}
	comment := func(group string, d *model.Diff, resolved bool) {
		c, err := s.AddComment(&model.Comment{
			Group: group, DiffID: d.ID, FileID: d.Files[0].ID, Path: d.Files[0].Path(),
			Side: "new", StartLine: 1, EndLine: 1, Body: "look", Resolved: resolved,
		})
		if err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, sc := range s.group(group, false).Comments {
			if sc.ID == c.ID {
				sc.CreatedAt, sc.UpdatedAt = old, old
			}
		}
	}

	add("stale", old)
	comment("settled", add("settled", old), true)
	comment("open", add("open", old), false)
	add("hooked", old)
	if _, err := s.AddHook("hooked", &model.Hook{Command: "true"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.group("hooked", false).Hooks[0].CreatedAt = old
	s.mu.Unlock()
	add("fresh", cutoff.Add(time.Hour))
}

func prunedGroupNames(groups []PrunedGroup) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	slices.Sort(names)
	return names
}

func keptReasons(groups []PrunedGroup) map[string]string {
	out := map[string]string{}
	for _, g := range groups {
		out[g.Name] = g.Reason
	}
	return out
}

func TestPruneEndpoint(t *testing.T) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, tt := range []struct {
		name       string
		req        PruneRequest
		wantClosed []string
		wantKept   map[string]string
	}{
		{
			name:       "a dry run closes nothing",
			req:        PruneRequest{DryRun: true},
			wantClosed: []string{"settled", "stale"},
			wantKept:   map[string]string{"open": KeepOpenComments, "hooked": KeepHooks, "fresh": KeepRecent},
		},
		{
			name:       "old reviews without open comments or hooks are closed",
			wantClosed: []string{"settled", "stale"},
			wantKept:   map[string]string{"open": KeepOpenComments, "hooked": KeepHooks, "fresh": KeepRecent},
		},
		{
			name:       "force closes every old review",
			req:        PruneRequest{Force: true},
			wantClosed: []string{"hooked", "open", "settled", "stale"},
			wantKept:   map[string]string{"fresh": KeepRecent},
		},
		{
			name:       "only the reviews the request names",
			req:        PruneRequest{Groups: []string{"stale"}},
			wantClosed: []string{"stale"},
			wantKept: map[string]string{
				"settled": KeepNotListed, "open": KeepOpenComments, "hooked": KeepHooks, "fresh": KeepRecent,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, srv := newTestServer(t)
			seedPruneStore(t, srv.store, cutoff)
			for _, g := range []string{"stale", "fresh"} {
				if err := os.MkdirAll(filepath.Join(srv.opts.CacheDir, "preview", g, "d1"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ch, ok := srv.broker.subscribe()
			if !ok {
				t.Fatal("cannot subscribe")
			}
			defer srv.broker.unsubscribe(ch)

			tt.req.Before = cutoff
			var res PruneResponse
			resp := postJSON(t, ts.URL+"/_/api/prune", tt.req, &res)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %s", resp.Status)
			}
			if got := prunedGroupNames(res.Closed); !slices.Equal(got, tt.wantClosed) {
				t.Errorf("closed = %v, want %v", got, tt.wantClosed)
			}
			if got := keptReasons(res.Kept); !maps.Equal(got, tt.wantKept) {
				t.Errorf("kept = %v, want %v", got, tt.wantKept)
			}

			// What the store holds, and what a restarted server reads back
			// from the session file, is what the answer said.
			wantLeft := slices.Sorted(maps.Keys(tt.wantKept))
			if tt.req.DryRun {
				wantLeft = slices.Sorted(slices.Values(append(slices.Clone(tt.wantClosed), wantLeft...)))
			}
			got := slices.Sorted(slices.Values(srv.store.GroupNames()))
			if !slices.Equal(got, wantLeft) {
				t.Errorf("groups left = %v, want %v", got, wantLeft)
			}
			reloaded := NewStore(srv.opts.SessionFile)
			if err := reloaded.Load(); err != nil {
				t.Fatal(err)
			}
			if got := slices.Sorted(slices.Values(reloaded.GroupNames())); !slices.Equal(got, wantLeft) {
				t.Errorf("groups after a restart = %v, want %v", got, wantLeft)
			}

			// A closed review tells its pages and lets go of its previews;
			// a dry run does neither.
			_, staleErr := os.Stat(filepath.Join(srv.opts.CacheDir, "preview", "stale"))
			if closed := !tt.req.DryRun; closed == (staleErr == nil) {
				t.Errorf("previews of a closed review present = %v, want %v", staleErr == nil, !closed)
			}
			if _, err := os.Stat(filepath.Join(srv.opts.CacheDir, "preview", "fresh")); err != nil {
				t.Errorf("previews of a kept review: %v", err)
			}
			events := drain(ch)
			for _, name := range tt.wantClosed {
				told := slices.ContainsFunc(events, func(b []byte) bool {
					return bytes.Contains(b, []byte(`"group":"`+name+`"`))
				})
				if told == tt.req.DryRun {
					t.Errorf("pages told about %q = %v, want %v", name, told, !tt.req.DryRun)
				}
			}
		})
	}
}

// drain returns the events already queued on ch.
func drain(ch chan event) [][]byte {
	var out [][]byte
	for {
		select {
		case ev := <-ch:
			out = append(out, ev.data)
		default:
			return out
		}
	}
}

func TestPruneEndpointWantsACutoff(t *testing.T) {
	ts, srv := newTestServer(t)
	seedPruneStore(t, srv.store, time.Now())
	resp := postJSON(t, ts.URL+"/_/api/prune", PruneRequest{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %s, want 400", resp.Status)
	}
	if n := len(srv.store.GroupNames()); n != 5 {
		t.Errorf("%d groups left, want all 5", n)
	}
}

// Closing a review, one round of it or every review lets go of the previews
// sbnn rebuilt for it, so the cache directory stops growing for good.
func TestClosingAReviewRemovesItsPreviews(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		gone    []string
		present []string
	}{
		{"one round", "/_/api/groups/api/diffs/d1", []string{"api/d1"}, []string{"api/d2", "other"}},
		{"one review", "/_/api/groups/api", []string{"api"}, []string{"other"}},
		{"every review", "/_/api/groups", []string{"api", "other"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, srv := newTestServer(t)
			for _, g := range []string{"api", "api", "other"} {
				srv.store.AddDiff(g, &model.Diff{Raw: sampleDiff, Files: diff.Parse(sampleDiff)})
			}
			preview := filepath.Join(srv.opts.CacheDir, "preview")
			for _, dir := range []string{"api/d1", "api/d2", "other/d3"} {
				if err := os.MkdirAll(filepath.Join(preview, dir), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(preview, dir, "README.md"), []byte("# x\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			req, err := http.NewRequest(http.MethodDelete, ts.URL+tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				t.Fatalf("status = %s", resp.Status)
			}
			for _, dir := range tt.gone {
				if _, err := os.Stat(filepath.Join(preview, dir)); err == nil {
					t.Errorf("%s is still in the cache", dir)
				}
			}
			for _, dir := range tt.present {
				if _, err := os.Stat(filepath.Join(preview, dir)); err != nil {
					t.Errorf("%s: %v", dir, err)
				}
			}
		})
	}
}

func TestPreviewDirStaysInsideTheCache(t *testing.T) {
	p := &previewer{cacheDir: t.TempDir()}
	for _, tt := range []struct {
		group string
		diffs []string
		want  string
	}{
		{"api", nil, filepath.Join(p.cacheDir, "preview", "api")},
		{"api", []string{"d1"}, filepath.Join(p.cacheDir, "preview", "api", "d1")},
		{"api", []string{".."}, ""},
		{"api", []string{"."}, ""},
		{"api", []string{""}, ""},
		{"..", nil, ""},
	} {
		if got := p.previewDir(tt.group, tt.diffs...); got != tt.want {
			t.Errorf("previewDir(%q, %q) = %q, want %q", tt.group, tt.diffs, got, tt.want)
		}
	}
	if got := (&previewer{}).previewDir("api"); got != "" {
		t.Errorf("previewDir without a cache directory = %q, want \"\"", got)
	}
}

func TestReadSession(t *testing.T) {
	t.Run("the snapshot and the log after it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "session-1.json")
		s := NewStore(path)
		s.AddDiff("kept", &model.Diff{Raw: sampleDiff, Files: diff.Parse(sampleDiff)})
		s.AddDiff("gone", &model.Diff{Raw: sampleDiff, Files: diff.Parse(sampleDiff)})
		s.DeleteGroup("gone")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		groups, err := ReadSession(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].Name != "kept" || len(groups[0].Diffs) != 1 {
			t.Errorf("groups = %+v, want the one review the log left", groups)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Error("reading the session changed the file")
		}
	})

	for _, tt := range []struct {
		name, content, want string
	}{
		{"a broken file", "{not json", "broken"},
		{"a file from a newer sbnn", `{"version":99,"groups":[]}` + "\n", "newer sbnn"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session-1.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadSession(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one saying %q", err, tt.want)
			}
			// Load would have set the file aside; a reader must not.
			if b, err := os.ReadFile(path); err != nil || string(b) != tt.content {
				t.Errorf("the file was touched: %q, %v", b, err)
			}
		})
	}
}

// The JSON of the answer is what the command line prints with --json, so the
// keys are part of what a script reads.
func TestPruneResponseJSON(t *testing.T) {
	b, err := json.Marshal(PruneResponse{
		Closed: []PrunedGroup{{Name: "a"}},
		Kept:   []PrunedGroup{{Name: "b", Reason: KeepHooks, Hooks: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"closed"`, `"kept"`, `"reason":"hooks"`, `"unresolved"`} {
		if !bytes.Contains(b, []byte(key)) {
			t.Errorf("%s lacks %s", b, key)
		}
	}
}
