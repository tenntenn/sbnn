package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/server"
)

// pruneCutoff is the --older-than every test here prunes at.
var pruneCutoff = time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)

// oldGroup and freshGroup are reviews on either side of pruneCutoff. open
// adds a comment nobody resolved, dated with the review.
func oldGroup(name string, open bool) *model.Group {
	at := pruneCutoff.Add(-30 * 24 * time.Hour)
	g := &model.Group{Name: name, Diffs: []*model.Diff{{ID: "d1", Title: "round", CreatedAt: at}}}
	if open {
		g.Comments = []*model.Comment{{ID: "c1", DiffID: "d1", Body: "why?", CreatedAt: at, UpdatedAt: at}}
	}
	return g
}

func freshGroup(name string) *model.Group {
	return &model.Group{Name: name, Diffs: []*model.Diff{{ID: "d1", Title: "round", CreatedAt: pruneCutoff.Add(time.Hour)}}}
}

// writeSessionFile writes a session file in the format the server reads,
// holding the given reviews. It is written by hand rather than through a
// Store because a Store stamps comments with the current time, and an old
// open comment is exactly what these tests need.
func writeSessionFile(t *testing.T, path string, groups ...*model.Group) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"version": 2, "seq": 9, "groups": groups})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakePorts answers a portProbe from a table: a port listed with groups is a
// running server holding them, a port listed with a reason is busy, and any
// other port is free.
type fakePort struct {
	groups []string
	reason string
}

func fakePorts(ports map[int]fakePort) portProbe {
	return func(p int) ([]string, bool, string) {
		fp, ok := ports[p]
		if !ok {
			return nil, false, ""
		}
		return fp.groups, fp.reason == "", fp.reason
	}
}

// fileSummary reduces a list of files to "kind:base" strings, sorted, which
// is what the tables below compare.
func fileSummary(files []pruneFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Kind+":"+filepath.Base(f.Path))
	}
	slices.Sort(out)
	return out
}

func TestPlanPruneFiles(t *testing.T) {
	for _, tt := range []struct {
		name string
		// sessions are the session files to write, by port.
		sessions map[int][]*model.Group
		// logs are the ports with a server log.
		logs []int
		// raw are session files with this content instead.
		raw      map[int]string
		previews []string
		ports    map[int]fakePort
		force    bool

		wantRemove []string
		wantKeep   []string
	}{
		{
			name:       "a stopped server whose reviews are all old",
			sessions:   map[int][]*model.Group{16280: {oldGroup("a", false), oldGroup("b", false)}},
			logs:       []int{16280},
			previews:   []string{"a", "b"},
			wantRemove: []string{"log:server-16280.log", "preview:a", "preview:b", "session:session-16280.json"},
		},
		{
			name:       "an empty session file",
			sessions:   map[int][]*model.Group{29417: {}},
			wantRemove: []string{"session:session-29417.json"},
		},
		{
			name:       "a log without a session file",
			logs:       []int{6299},
			wantRemove: []string{"log:server-6299.log"},
		},
		{
			name:       "a stopped server holding an open comment",
			sessions:   map[int][]*model.Group{6281: {oldGroup("a", false), oldGroup("asked", true)}},
			logs:       []int{6281},
			previews:   []string{"a", "asked"},
			wantKeep:   []string{"log:server-6281.log", "session:session-6281.json"},
			wantRemove: nil,
		},
		{
			name:       "force takes the open comment as well",
			sessions:   map[int][]*model.Group{6281: {oldGroup("a", false), oldGroup("asked", true)}},
			logs:       []int{6281},
			previews:   []string{"a", "asked"},
			force:      true,
			wantRemove: []string{"log:server-6281.log", "preview:a", "preview:asked", "session:session-6281.json"},
		},
		{
			name:     "a stopped server with a recent review",
			sessions: map[int][]*model.Group{17280: {freshGroup("new")}},
			previews: []string{"new"},
			force:    true,
			wantKeep: []string{"session:session-17280.json"},
		},
		{
			name:       "a running server's files are never touched",
			sessions:   map[int][]*model.Group{6280: {oldGroup("a", false)}},
			logs:       []int{6280},
			previews:   []string{"a", "held", "orphan"},
			ports:      map[int]fakePort{6280: {groups: []string{"held"}}},
			wantKeep:   []string{"log:server-6280.log", "session:session-6280.json"},
			wantRemove: []string{"preview:a", "preview:orphan"},
		},
		{
			name:     "a port something else is listening on",
			sessions: map[int][]*model.Group{8080: {oldGroup("a", false)}},
			logs:     []int{8080},
			ports:    map[int]fakePort{8080: {reason: "busy"}},
			wantKeep: []string{"log:server-8080.log", "session:session-8080.json"},
		},
		{
			name:     "a session file this sbnn cannot read",
			raw:      map[int]string{7000: `{"version":99}`},
			logs:     []int{7000},
			wantKeep: []string{"log:server-7000.log", "session:session-7000.json"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state, cache := t.TempDir(), t.TempDir()
			for p, groups := range tt.sessions {
				writeSessionFile(t, filepath.Join(state, "session-"+strconv.Itoa(p)+".json"), groups...)
			}
			for p, content := range tt.raw {
				if err := os.WriteFile(filepath.Join(state, "session-"+strconv.Itoa(p)+".json"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, p := range tt.logs {
				touch(t, filepath.Join(state, "server-"+strconv.Itoa(p)+".log"))
			}
			for _, g := range tt.previews {
				touch(t, filepath.Join(cache, "preview", g, "d1", "README.md"))
			}
			// What prune must never look at, whatever else is going on.
			touch(t, filepath.Join(state, "reviews.jsonl"))
			touch(t, filepath.Join(state, "session-6280.json.broken"))
			touch(t, filepath.Join(state, ".session-123456"))
			touch(t, filepath.Join(cache, "preview", "_not-a-review", "x.md"))
			touch(t, filepath.Join(cache, "preview", "loose-file.md"))

			fp := planPruneFiles(state, cache, pruneCutoff, tt.force, fakePorts(tt.ports))
			gotRemove := fileSummary(fp.remove)
			if want := slices.Sorted(slices.Values(tt.wantRemove)); !slices.Equal(gotRemove, want) {
				t.Errorf("remove = %v, want %v", gotRemove, want)
			}
			gotKeep := fileSummary(fp.keep)
			if want := slices.Sorted(slices.Values(tt.wantKeep)); !slices.Equal(gotKeep, want) {
				t.Errorf("keep = %v, want %v", gotKeep, want)
			}
			for _, f := range fp.keep {
				if f.Reason == "" {
					t.Errorf("%s is kept without a reason", f.Path)
				}
			}
		})
	}
}

// The reason a stopped server's session file is kept names the review that
// keeps it, and the most telling reason when there are several.
func TestJudgeSessionNamesTheReviewThatKeepsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-6281.json")
	writeSessionFile(t, path, freshGroup("recent"), oldGroup("asked", true), oldGroup("done", false))
	names, why := judgeSession(path, pruneCutoff, false)
	if want := []string{"recent", "asked", "done"}; !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	for _, want := range []string{"6281", `"asked"`, "1 open comment(s)"} {
		if !strings.Contains(why, want) {
			t.Errorf("why = %q, want it to mention %s", why, want)
		}
	}
}

// A server can start on a port between the plan and the removal, and it has
// just read the session file prune was about to delete.
func TestRemovePruneFilesAsksAgain(t *testing.T) {
	state, cache := t.TempDir(), t.TempDir()
	started := filepath.Join(state, "session-16280.json")
	stopped := filepath.Join(state, "session-17280.json")
	preview := filepath.Join(cache, "preview", "a")
	for _, p := range []string{started, stopped, filepath.Join(preview, "d1", "README.md")} {
		touch(t, p)
	}
	fp := pruneFilePlan{remove: []pruneFile{
		{Kind: pruneSession, Path: started, Port: 16280},
		{Kind: pruneSession, Path: stopped, Port: 17280},
		{Kind: prunePreview, Path: preview, Group: "a"},
		// Gone already, which is what was wanted.
		{Kind: pruneLog, Path: filepath.Join(state, "server-17280.log"), Port: 17280},
	}}
	out := removePruneFiles(fp, fakePorts(map[int]fakePort{16280: {}}))

	if got, want := fileSummary(out.remove), []string{"log:server-17280.log", "preview:a", "session:session-17280.json"}; !slices.Equal(got, want) {
		t.Errorf("removed = %v, want %v", got, want)
	}
	if got, want := fileSummary(out.keep), []string{"session:session-16280.json"}; !slices.Equal(got, want) {
		t.Errorf("kept = %v, want %v", got, want)
	}
	for path, want := range map[string]bool{started: true, stopped: false, preview: false} {
		if _, err := os.Stat(path); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", path, err == nil, want)
		}
	}
}

func TestPruneQuestion(t *testing.T) {
	plan := &server.PruneResponse{Closed: []server.PrunedGroup{{Name: "stale", Diffs: 1}}}
	session := pruneFile{Kind: pruneSession, Port: 16280, Groups: []string{"x", "y"}}
	log := pruneFile{Kind: pruneLog, Port: 16280}
	for _, tt := range []struct {
		name   string
		plan   *server.PruneResponse
		remove []pruneFile
		want   []string // "" means no question at all
	}{
		{"nothing to close", &server.PruneResponse{}, []pruneFile{log}, nil},
		{"no server and only a log", nil, []pruneFile{log}, nil},
		{"reviews on the server", plan, nil, []string{"stale", "Close 1 review(s)?"}},
		{"reviews in a stopped server's session", nil, []pruneFile{session, log}, []string{"16280", "x, y", "Close 2 review(s)?"}},
		{"both", plan, []pruneFile{session}, []string{"stale", "16280", "Close 3 review(s)?"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := pruneQuestion(tt.plan, tt.remove, pruneCutoff)
			if len(tt.want) == 0 {
				if q != "" {
					t.Errorf("question = %q, want none", q)
				}
				return
			}
			for _, want := range tt.want {
				if !strings.Contains(q, want) {
					t.Errorf("question = %q, want it to contain %q", q, want)
				}
			}
		})
	}
}

// withPruneState points sbnn at scratch state and cache directories and sets
// the flag variables runPrune reads, putting everything back afterwards.
func withPruneState(t *testing.T) (state, cache string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	state, err := paths.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	cache, err = paths.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(state, home) || !strings.HasPrefix(cache, home) {
		t.Fatalf("state %s and cache %s must be under %s", state, cache, home)
	}
	oldPort, oldBind, oldJSON, oldYes := port, bind, jsonOutput, assumeYes
	oldAge, oldForce, oldDry := pruneOlderThan, pruneForce, pruneDryRun
	t.Cleanup(func() {
		port, bind, jsonOutput, assumeYes = oldPort, oldBind, oldJSON, oldYes
		pruneOlderThan, pruneForce, pruneDryRun = oldAge, oldForce, oldDry
	})
	bind, jsonOutput, assumeYes = "localhost", true, true
	pruneOlderThan, pruneForce, pruneDryRun = pruneCutoff.Format(time.RFC3339), false, false
	return state, cache
}

// runPrune end to end: a real server on a scratch port, the session file of a
// server that is gone, and the previews of both.
func TestRunPrune(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		t.Run("dry run "+strconv.FormatBool(dryRun), func(t *testing.T) {
			state, cache := withPruneState(t)
			pruneDryRun = dryRun
			port = freePort(t)
			gone := freePort(t)

			own := filepath.Join(state, "session-"+strconv.Itoa(port)+".json")
			writeSessionFile(t, own, oldGroup("stale", false), oldGroup("asked", true), freshGroup("fresh"))
			touch(t, filepath.Join(state, "server-"+strconv.Itoa(port)+".log"))
			other := filepath.Join(state, "session-"+strconv.Itoa(gone)+".json")
			writeSessionFile(t, other, oldGroup("elsewhere", false))
			otherLog := filepath.Join(state, "server-"+strconv.Itoa(gone)+".log")
			touch(t, otherLog)
			history := filepath.Join(state, "reviews.jsonl")
			touch(t, history)
			for _, g := range []string{"stale", "asked", "fresh", "elsewhere", "orphan"} {
				touch(t, filepath.Join(cache, "preview", g, "d1", "README.md"))
			}

			srv, err := server.New(server.Options{
				Port: port, SessionFile: own, CacheDir: cache, Version: "test",
				Mo: mo.New("mo-not-installed-for-tests", 0, ""),
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- srv.Run(ctx) }()
			t.Cleanup(func() {
				cancel()
				<-done
			})
			if _, ok := waitFor(port, true, 5*time.Second); !ok {
				t.Fatal("the server never came up")
			}

			out := captureStdout(t)
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			if err := runPrune(cmd, nil); err != nil {
				out()
				t.Fatal(err)
			}
			var rep pruneReport
			if err := json.Unmarshal([]byte(out()), &rep); err != nil {
				t.Fatal(err)
			}

			if !rep.Running || rep.DryRun != dryRun {
				t.Errorf("running = %v, dry run = %v", rep.Running, rep.DryRun)
			}
			if got := prunedGroupNamesOf(rep.Closed); !slices.Equal(got, []string{"stale"}) {
				t.Errorf("closed = %v, want [stale]", got)
			}
			if got := prunedGroupNamesOf(rep.Kept); !slices.Equal(got, []string{"asked", "fresh"}) {
				t.Errorf("kept = %v, want [asked fresh]", got)
			}
			wantRemoved := []string{
				"log:" + filepath.Base(otherLog),
				"preview:elsewhere", "preview:orphan", "preview:stale",
				"session:" + filepath.Base(other),
			}
			slices.Sort(wantRemoved)
			if got := fileSummary(rep.Removed); !slices.Equal(got, wantRemoved) {
				t.Errorf("removed = %v, want %v", got, wantRemoved)
			}

			names := slices.Sorted(slices.Values(srv.Store().GroupNames()))
			wantNames := []string{"asked", "fresh"}
			if dryRun {
				wantNames = []string{"asked", "fresh", "stale"}
			}
			if !slices.Equal(names, wantNames) {
				t.Errorf("the server holds %v, want %v", names, wantNames)
			}
			exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
			for _, p := range []string{own, history, filepath.Join(cache, "preview", "asked"), filepath.Join(cache, "preview", "fresh")} {
				if !exists(p) {
					t.Errorf("%s is gone; it must never be pruned here", p)
				}
			}
			for _, p := range []string{other, otherLog, filepath.Join(cache, "preview", "stale"), filepath.Join(cache, "preview", "orphan")} {
				if exists(p) == !dryRun {
					t.Errorf("%s exists = %v, want %v", p, exists(p), dryRun)
				}
			}
		})
	}
}

func prunedGroupNamesOf(groups []server.PrunedGroup) []string {
	names := prunedNames(groups)
	slices.Sort(names)
	return names
}

func TestPrintPruneReport(t *testing.T) {
	running := pruneFile{Kind: pruneSession, Path: "/s/session-6280.json", Reason: "an sbnn server is running on port 6280", running: true}
	held := pruneFile{Kind: pruneSession, Path: "/s/session-6281.json", Reason: `it holds "asked"`}
	for _, tt := range []struct {
		name    string
		rep     pruneReport
		want    []string
		notWant []string
	}{
		{
			name: "a dry run says what it would do",
			rep: pruneReport{
				DryRun: true, Running: true, Server: "http://localhost:6280",
				Closed: []server.PrunedGroup{{Name: "stale"}},
				Kept: []server.PrunedGroup{
					{Name: "asked", Reason: server.KeepOpenComments, Unresolved: 2},
					{Name: "fresh", Reason: server.KeepRecent},
				},
				Removed: []pruneFile{{Kind: pruneSession, Path: "/s/session-16280.json", Groups: []string{"x"}}},
			},
			want: []string{"Would close 1 review(s)", "stale", "asked", "2 open comment(s)", "--force",
				"Left 1 review(s)", "Would remove 1 file(s)", "session-16280.json (with x)"},
			notWant: []string{"Nothing to prune"},
		},
		{
			name:    "a running server's own files are not repeated",
			rep:     pruneReport{Running: true, KeptFiles: []pruneFile{running, held}},
			want:    []string{"Kept 1 file(s)", "session-6281.json", "Nothing to prune"},
			notWant: []string{"session-6280.json"},
		},
		{
			name: "no server",
			rep:  pruneReport{Server: "http://localhost:6280"},
			want: []string{"No sbnn server on http://localhost:6280", "Nothing to prune"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			printPruneReport(&b, tt.rep)
			for _, want := range tt.want {
				if !strings.Contains(b.String(), want) {
					t.Errorf("report lacks %q:\n%s", want, b.String())
				}
			}
			for _, not := range tt.notWant {
				if strings.Contains(b.String(), not) {
					t.Errorf("report says %q:\n%s", not, b.String())
				}
			}
		})
	}
}

// A server started by an sbnn from before prune answers the request with the
// router's refusal, which is worth translating into what to do about it.
func TestOlderServer(t *testing.T) {
	for _, tt := range []struct {
		err  string
		hint bool
	}{
		{"POST http://localhost:6280/_/api/prune: Method Not Allowed", true},
		{"POST http://localhost:6280/_/api/prune: 404 page not found", true},
		{"POST http://localhost:6280/_/api/prune: before is required", false},
		{"dial tcp: connection refused", false},
	} {
		got := olderServer(errors.New(tt.err)).Error()
		if hint := strings.Contains(got, "sbnn --restart"); hint != tt.hint {
			t.Errorf("olderServer(%q) = %q, hint = %v, want %v", tt.err, got, hint, tt.hint)
		}
	}
}

func TestRunPruneRefusesABadAge(t *testing.T) {
	for _, age := range []string{"", "  ", "soon", "-3d"} {
		t.Run(age, func(t *testing.T) {
			withPruneState(t)
			pruneOlderThan = age
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			if err := runPrune(cmd, nil); err == nil {
				t.Errorf("--older-than %q was accepted", age)
			}
		})
	}
}
