package cmd

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/repository"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/internal/servermap"
)

// DefaultPruneAge is how long a review has to be left alone before sbnn prune
// closes it. A week is long enough that a review someone is still working
// through - over a weekend, between two meetings - is never taken, and short
// enough that the pile it exists to clear does not build up.
const DefaultPruneAge = "7d"

var (
	pruneOlderThan string
	pruneForce     bool
	pruneDryRun    bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Close reviews nobody has touched for a while, and the files left behind",
	Long: `Close the reviews nobody has touched for a while, and remove the files that
servers which are no longer running left behind.

Nothing else ends a review an agent forgot to close: a group stays on the
server until someone runs sbnn --clear. sbnn prune is the clean-up for that.

  $ sbnn prune --dry-run             # what would go, and what stays and why
  $ sbnn prune                       # close reviews left alone for 7 days
  $ sbnn prune --older-than 30d      # ... for 30 days (36h, 2026-01-31 work too)

A review is as old as the last thing that happened to it: a diff sent, a
comment written or edited, a review submitted, a hook registered or run. An
old review is still kept while it holds a comment nobody resolved, or a hook
waiting for the review to be submitted, and prune says which it kept and why;
--force closes those as well.

The same rule decides the files in the state directory. The session file of
a port no sbnn server is running on is removed once every review in it would
be closed, and the log of that server with it; a session file of a running
server is never touched, and neither is the log of submitted reviews that
sbnn reviews reads. Previews sbnn rebuilt from a diff are removed from the
cache directory once no server and no remaining session file holds their
review.

Before closing anything it asks, listing what it is about to close; --yes, or
a stdin that is not a terminal, skips the question.`,
	Args:         cobra.NoArgs,
	RunE:         runPrune,
	SilenceUsage: true,
}

func init() {
	f := pruneCmd.Flags()
	f.IntVarP(&port, "port", "p", 0,
		"Port of the server whose reviews are closed (default: the port of this repository's server)")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVar(&pruneOlderThan, "older-than", DefaultPruneAge,
		"Close reviews with no activity for this long, or since this date: 7d, 36h, 2026-01-31")
	f.BoolVar(&pruneForce, "force", false, "Close old reviews even when they hold open comments or hooks")
	f.BoolVar(&pruneDryRun, "dry-run", false, "List what would be closed and removed, and change nothing")
	f.BoolVar(&assumeYes, "yes", false, "Skip the confirmation")
	f.BoolVar(&jsonOutput, "json", false, "Print structured JSON on stdout")
}

// The kinds of file prune removes.
const (
	pruneSession = "session"
	pruneLog     = "log"
	prunePreview = "preview"
)

// pruneFile is one file or directory prune removes, or looked at and kept.
type pruneFile struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	// Port is the port a session file or a log belongs to.
	Port int `json:"port,omitempty"`
	// Group is the review a preview directory belongs to.
	Group string `json:"group,omitempty"`
	// Groups are the reviews a session file holds.
	Groups []string `json:"groups,omitempty"`
	// Reason is why the file was kept. It is empty for a file that goes.
	Reason string `json:"reason,omitempty"`
	// running marks a file kept because its server is up. Those are kept on
	// every run, so the text report leaves them out rather than repeat them.
	running bool
}

// pruneReport is everything a prune did, or with --dry-run would do.
type pruneReport struct {
	DryRun bool      `json:"dryRun"`
	Before time.Time `json:"before"`
	// Repository is the repository prune was run in, whose server it
	// closes reviews on: its root, or "" outside any repository.
	Repository string `json:"repository"`
	// Server is the URL of that server, or "" when the repository has
	// never had one.
	Server string `json:"server"`
	// Running is whether a server answered on Server. When none did, only
	// files are pruned.
	Running   bool                 `json:"running"`
	Closed    []server.PrunedGroup `json:"closed"`
	Kept      []server.PrunedGroup `json:"kept"`
	Removed   []pruneFile          `json:"removed"`
	KeptFiles []pruneFile          `json:"keptFiles"`
	// Forgotten are the servers dropped from the server map: their
	// repository is gone, and so is everything they kept.
	Forgotten []servermap.Entry `json:"forgotten"`
}

func runPrune(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if strings.TrimSpace(pruneOlderThan) == "" {
		return errors.New("--older-than needs an age or a date, such as 7d, 36h or 2026-01-31")
	}
	before, err := history.ParseSince(pruneOlderThan, time.Now())
	if err != nil {
		return fmt.Errorf("--older-than: %w", err)
	}
	stateDir, err := paths.StateDir()
	if err != nil {
		return err
	}
	cacheDir, err := paths.CacheDir()
	if err != nil {
		return err
	}

	ref, err := resolveServer(cmd, false)
	if err != nil {
		return err
	}
	rep := pruneReport{
		DryRun:     pruneDryRun,
		Before:     before,
		Repository: ref.repo,
		Closed:     []server.PrunedGroup{},
		Kept:       []server.PrunedGroup{},
		Removed:    []pruneFile{},
		KeptFiles:  []pruneFile{},
		Forgotten:  []servermap.Entry{},
	}
	req := server.PruneRequest{Before: before, Force: pruneForce, DryRun: true}
	// The plan is always worked out first, even when it is carried out at
	// once: the files depend on which reviews the server keeps, and the
	// question, when there is one, is built from it.
	var (
		c    *client.Client
		plan *server.PruneResponse
	)
	if ref.port != 0 {
		c = client.New(addr(), 5*time.Second)
		rep.Server = c.BaseURL()
		if st, err := probe(ctx, c, 2*time.Second); err == nil {
			// Closing reviews is the one thing prune does to a server, and
			// it must be this repository's reviews it closes.
			if err := ref.check(st); err != nil {
				return err
			}
			rep.Running = true
			if plan, err = c.Prune(ctx, req); err != nil {
				return olderServer(err)
			}
		}
	}
	files := planPruneFiles(stateDir, cacheDir, before, pruneForce, runningServers(ctx, plan))

	if !pruneDryRun {
		ok, err := askBeforeClear(pruneQuestion(plan, files.remove, before))
		if err != nil {
			return err
		}
		if !ok {
			return errCancelled
		}
		if plan != nil && len(plan.Closed) > 0 {
			// Only what the user was shown: a review that became old enough
			// while they read the question was not part of what they agreed
			// to.
			req.DryRun = false
			for _, g := range plan.Closed {
				req.Groups = append(req.Groups, g.Name)
			}
			if plan, err = c.Prune(ctx, req); err != nil {
				return err
			}
			// A review the server kept after all - a comment arrived in
			// between - keeps its previews too.
			files = files.keepingPreviews(prunedNames(plan.Kept))
		}
		files = removePruneFiles(files, runningServers(ctx, nil))
	}
	forgotten, err := forgetServers(stateDir, files.remove, runningServers(ctx, nil), pruneDryRun)
	if err != nil {
		return err
	}
	rep.Forgotten = append(rep.Forgotten, forgotten...)
	if plan != nil {
		rep.Closed, rep.Kept = plan.Closed, plan.Kept
	}
	rep.Removed = append(rep.Removed, files.remove...)
	rep.KeptFiles = append(rep.KeptFiles, files.keep...)
	if jsonOutput {
		return writeJSON(rep)
	}
	printPruneReport(os.Stdout, rep)
	return nil
}

// olderServer explains a prune the server did not know how to answer. The
// server is detached and outlives upgrades, so the one answering may well be
// from before prune existed, and "405 Method Not Allowed" says nothing about
// what to do next.
func olderServer(err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not allowed") || strings.Contains(msg, "not found") {
		return fmt.Errorf("%w (the running server is older than sbnn prune: sbnn --restart starts it again from this binary, keeping the session)", err)
	}
	return err
}

// portProbe reports whether an sbnn server is running on a port and, when one
// is, the reviews it holds. reason is set when there is no sbnn server to ask
// but the port cannot be called free either, which counts as running as far
// as deleting its files goes.
type portProbe func(port int) (groups []string, running bool, reason string)

// runningServers asks each port the way the rest of sbnn finds a server: by
// its status endpoint, which also tells an sbnn server from anything else on
// the port. The server of --port is described by plan, when there is one,
// because what it will hold once the prune is done is what matters, not what
// it holds now.
//
// It asks on localhost and on --bind. A server bound to some other address
// answers on neither, so a port that refuses a listener is not called free
// either.
func runningServers(ctx context.Context, plan *server.PruneResponse) portProbe {
	hosts := []string{"localhost"}
	if bind != "" && bind != "localhost" {
		hosts = append(hosts, bind)
	}
	return func(p int) ([]string, bool, string) {
		for _, host := range hosts {
			st, err := client.New(net.JoinHostPort(host, strconv.Itoa(p)), time.Second).Status(ctx)
			if err != nil {
				continue
			}
			if p == port && plan != nil {
				return prunedNames(plan.Kept), true, ""
			}
			names := make([]string, 0, len(st.Groups))
			for _, g := range st.Groups {
				names = append(names, g.Name)
			}
			return names, true, ""
		}
		if err := checkAddrFree(net.JoinHostPort("localhost", strconv.Itoa(p))); err != nil {
			return nil, false, fmt.Sprintf("port %d is in use by something that does not answer as sbnn", p)
		}
		return nil, false, ""
	}
}

func prunedNames(groups []server.PrunedGroup) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	return names
}

// pruneFilePlan is what prune removes, and what it looked at and kept.
type pruneFilePlan struct {
	remove, keep []pruneFile
}

// keepingPreviews moves the previews of the given reviews from remove to
// keep.
func (fp pruneFilePlan) keepingPreviews(groups []string) pruneFilePlan {
	out := pruneFilePlan{keep: fp.keep}
	for _, f := range fp.remove {
		if f.Kind == prunePreview && slices.Contains(groups, f.Group) {
			f.Reason = "its review was kept"
			out.keep = append(out.keep, f)
			continue
		}
		out.remove = append(out.remove, f)
	}
	return out
}

var (
	sessionFileName = regexp.MustCompile(`^session-([0-9]+)\.json$`)
	serverLogName   = regexp.MustCompile(`^server-([0-9]+)\.log$`)
)

// portFiles are the files the state directory keeps for one port.
type portFiles struct {
	session, log string
}

// list returns the files of a port that exist.
func (pf *portFiles) list(p int) []pruneFile {
	var out []pruneFile
	if pf.session != "" {
		out = append(out, pruneFile{Kind: pruneSession, Path: pf.session, Port: p})
	}
	if pf.log != "" {
		out = append(out, pruneFile{Kind: pruneLog, Path: pf.log, Port: p})
	}
	return out
}

// stateFiles finds the session files and server logs in the state directory,
// by port.
func stateFiles(stateDir string) map[int]*portFiles {
	ports := map[int]*portFiles{}
	entries, _ := os.ReadDir(stateDir)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		for _, re := range []*regexp.Regexp{sessionFileName, serverLogName} {
			m := re.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			p, err := strconv.Atoi(m[1])
			if err != nil || p <= 0 || p > 65535 {
				continue
			}
			pf := ports[p]
			if pf == nil {
				pf = &portFiles{}
				ports[p] = pf
			}
			path := filepath.Join(stateDir, e.Name())
			if re == sessionFileName {
				pf.session = path
			} else {
				pf.log = path
			}
		}
	}
	return ports
}

// planPruneFiles works out which files of the state and cache directories can
// go. It changes nothing.
//
// Only names sbnn itself writes are looked at: session-<port>.json and
// server-<port>.log in the state directory, and one directory per review
// under preview/ in the cache directory. Everything else is left alone - the
// log of submitted reviews above all, and the .broken copy of a session file
// a server refused to read, which is kept on purpose.
func planPruneFiles(stateDir, cacheDir string, before time.Time, force bool, ask portProbe) pruneFilePlan {
	var fp pruneFilePlan
	ports := stateFiles(stateDir)
	// The reviews something still holds. Their previews stay.
	live := map[string]bool{}
	for _, p := range slices.Sorted(maps.Keys(ports)) {
		pf := ports[p]
		groups, running, reason := ask(p)
		if running || reason != "" {
			if reason == "" {
				reason = fmt.Sprintf("an sbnn server is running on port %d", p)
			}
			for _, g := range groups {
				live[g] = true
			}
			for _, f := range pf.list(p) {
				f.Reason, f.running = reason, running
				fp.keep = append(fp.keep, f)
			}
			continue
		}
		sessionGone := true
		if pf.session != "" {
			f := pruneFile{Kind: pruneSession, Path: pf.session, Port: p}
			var why string
			f.Groups, why = judgeSession(pf.session, before, force)
			if why != "" {
				sessionGone = false
				f.Reason = why
				for _, g := range f.Groups {
					live[g] = true
				}
				fp.keep = append(fp.keep, f)
			} else {
				fp.remove = append(fp.remove, f)
			}
		}
		if pf.log != "" {
			f := pruneFile{Kind: pruneLog, Path: pf.log, Port: p}
			if sessionGone {
				fp.remove = append(fp.remove, f)
			} else {
				f.Reason = fmt.Sprintf("the session file of port %d is kept", p)
				fp.keep = append(fp.keep, f)
			}
		}
	}

	if cacheDir == "" {
		return fp
	}
	previews, _ := os.ReadDir(filepath.Join(cacheDir, "preview"))
	for _, e := range previews {
		name := e.Name()
		if !e.IsDir() || live[name] {
			continue
		}
		// A name sbnn could not have given a review is not sbnn's to remove.
		if valid, err := server.ValidateGroupName(name); err != nil || valid != name {
			continue
		}
		fp.remove = append(fp.remove, pruneFile{
			Kind:  prunePreview,
			Path:  filepath.Join(cacheDir, "preview", name),
			Group: name,
		})
	}
	return fp
}

// judgeSession decides about the session file of a server that is not
// running, by the rule the server applies to its own reviews, and returns the
// reviews it holds. The file goes only when every review in it would be
// closed: it is one file, and taking some reviews out of it would mean
// rewriting the session of a server that may yet be started on it.
//
// why is the reason the file is kept, or "" when it can go. A file that
// cannot be read is kept: it may be a session from a newer sbnn, and the
// bytes on disk are the only copy of it.
func judgeSession(path string, before time.Time, force bool) (names []string, why string) {
	groups, err := server.ReadSession(path)
	if err != nil {
		return nil, fmt.Sprintf("it cannot be read (%v)", err)
	}
	for _, g := range groups {
		names = append(names, g.Name)
	}
	res := server.PruneGroups(groups, before, force)
	if len(res.Kept) == 0 {
		return names, ""
	}
	// The most telling reason first: an open comment says more than a
	// recent diff about why the file has to stay.
	k := slices.MinFunc(res.Kept, func(a, b server.PrunedGroup) int {
		return cmp.Compare(keepRank(a.Reason), keepRank(b.Reason))
	})
	return names, fmt.Sprintf("no server is running on port %d, but it holds %q, which is kept: %s",
		sessionPort(path), k.Name, keepReasonText(k, before))
}

// sessionPort reads the port back out of a session file name.
func sessionPort(path string) int {
	m := sessionFileName.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return 0
	}
	p, _ := strconv.Atoi(m[1])
	return p
}

func keepRank(reason string) int {
	switch reason {
	case server.KeepOpenComments:
		return 0
	case server.KeepHooks:
		return 1
	default:
		return 2
	}
}

// keepReasonText says in words why a review was kept.
func keepReasonText(g server.PrunedGroup, before time.Time) string {
	switch g.Reason {
	case server.KeepOpenComments:
		return fmt.Sprintf("%d open comment(s); --force closes it anyway", g.Unresolved)
	case server.KeepHooks:
		return fmt.Sprintf("%d hook(s) waiting for the review; --force closes it anyway", g.Hooks)
	case server.KeepNotListed:
		return "it became old enough while the question was being answered"
	case server.KeepRecent:
		return "active since " + pruneTime(before)
	default:
		return g.Reason
	}
}

// removePruneFiles removes what fp says can go and returns what it removed
// and what it kept. A session file and a log are only removed after asking
// once more whether a server is running on their port: the plan is a
// question old, and a server started in between has just read that session
// and is going to write to it.
func removePruneFiles(fp pruneFilePlan, ask portProbe) pruneFilePlan {
	out := pruneFilePlan{keep: fp.keep}
	asked := map[int]string{}
	for _, f := range fp.remove {
		if f.Kind != prunePreview {
			reason, ok := asked[f.Port]
			if !ok {
				_, running, why := ask(f.Port)
				if running && why == "" {
					why = fmt.Sprintf("an sbnn server started on port %d", f.Port)
				}
				reason = why
				asked[f.Port] = why
			}
			if reason != "" {
				f.Reason = reason
				out.keep = append(out.keep, f)
				continue
			}
		}
		var err error
		if f.Kind == prunePreview {
			err = os.RemoveAll(f.Path)
		} else if err = os.Remove(f.Path); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			f.Reason = fmt.Sprintf("it could not be removed (%v)", err)
			out.keep = append(out.keep, f)
			continue
		}
		out.remove = append(out.remove, f)
	}
	return out
}

// pruneQuestion lists every review a prune is about to close, on the server
// and in the session files of servers that are not running, so that the user
// sees what goes before answering. It returns "" when no review is closed:
// what is left then is a stopped server's log and previews that are rebuilt
// on demand.
func pruneQuestion(plan *server.PruneResponse, remove []pruneFile, before time.Time) string {
	var b strings.Builder
	n := 0
	if plan != nil && len(plan.Closed) > 0 {
		fmt.Fprintf(&b, "sbnn: this will close the reviews nobody has touched since %s:\n", pruneTime(before))
		for _, g := range plan.Closed {
			fmt.Fprintf(&b, "  %-16s %s\n", g.Name, prunedGroupLine(g))
		}
		n += len(plan.Closed)
	}
	for _, f := range remove {
		if f.Kind != pruneSession || len(f.Groups) == 0 {
			continue
		}
		fmt.Fprintf(&b, "sbnn: and remove the session of port %d, where no server is running, with %s\n",
			f.Port, strings.Join(f.Groups, ", "))
		n += len(f.Groups)
	}
	if n == 0 {
		return ""
	}
	fmt.Fprintf(&b, "Close %d review(s)? [y/N]: ", n)
	return b.String()
}

// prunedGroupLine describes a review in one line.
func prunedGroupLine(g server.PrunedGroup) string {
	last := "no activity recorded"
	if !g.LastActivity.IsZero() {
		last = "last active " + pruneTime(g.LastActivity)
	}
	return fmt.Sprintf("%s, %d diff(s), %d comment(s), %d open", last, g.Diffs, g.Comments, g.Unresolved)
}

func pruneTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

// printPruneReport writes what a prune did, or would do, for a person.
func printPruneReport(w io.Writer, rep pruneReport) {
	verb := func(done, planned string) string {
		if rep.DryRun {
			return planned
		}
		return done
	}
	switch {
	case rep.Server == "":
		fmt.Fprintf(w, "No sbnn server was ever started for %s, so only the files of stopped servers are pruned.\n",
			repository.Describe(rep.Repository))
	case !rep.Running:
		fmt.Fprintf(w, "No sbnn server on %s, so only the files of stopped servers are pruned.\n", rep.Server)
	}
	if len(rep.Closed) > 0 {
		fmt.Fprintf(w, "%s %d review(s) on %s untouched since %s:\n",
			verb("Closed", "Would close"), len(rep.Closed), rep.Server, pruneTime(rep.Before))
		for _, g := range rep.Closed {
			fmt.Fprintf(w, "  %-16s %s\n", g.Name, prunedGroupLine(g))
		}
	}
	var old []server.PrunedGroup
	recent := 0
	for _, g := range rep.Kept {
		if g.Reason == server.KeepRecent {
			recent++
			continue
		}
		old = append(old, g)
	}
	if len(old) > 0 {
		fmt.Fprintf(w, "Kept %d old review(s):\n", len(old))
		for _, g := range old {
			fmt.Fprintf(w, "  %-16s %s\n", g.Name, keepReasonText(g, rep.Before))
		}
	}
	if recent > 0 {
		fmt.Fprintf(w, "Left %d review(s) with activity since %s.\n", recent, pruneTime(rep.Before))
	}
	if len(rep.Removed) > 0 {
		fmt.Fprintf(w, "%s %d file(s):\n", verb("Removed", "Would remove"), len(rep.Removed))
		for _, f := range rep.Removed {
			fmt.Fprintf(w, "  %-8s %s%s\n", f.Kind, f.Path, removedNote(f))
		}
	}
	kept := slices.DeleteFunc(slices.Clone(rep.KeptFiles), func(f pruneFile) bool { return f.running })
	if len(kept) > 0 {
		fmt.Fprintf(w, "Kept %d file(s):\n", len(kept))
		for _, f := range kept {
			fmt.Fprintf(w, "  %-8s %s: %s\n", f.Kind, f.Path, f.Reason)
		}
	}
	if len(rep.Forgotten) > 0 {
		fmt.Fprintf(w, "%s %d server(s) of repositories that no longer exist:\n",
			verb("Forgot", "Would forget"), len(rep.Forgotten))
		for _, e := range rep.Forgotten {
			fmt.Fprintf(w, "  port %-6d %s\n", e.Port, repository.Describe(e.Repository))
		}
	}
	if len(rep.Closed) == 0 && len(old) == 0 && len(rep.Removed) == 0 && len(rep.Forgotten) == 0 {
		fmt.Fprintln(w, "Nothing to prune.")
	}
}

// forgetServers drops from the server map the servers whose repository no
// longer exists and that left nothing behind: no server is running on their
// port, and their session file is gone or is one of removed. Without this
// the map only ever grows, and every deleted checkout keeps a port nobody
// will be given again.
//
// The server of the diffs from outside any repository has no directory to
// lose, so it is never forgotten this way. With dryRun the map is left as it
// is and the servers that would go are returned.
func forgetServers(stateDir string, removed []pruneFile, ask portProbe, dryRun bool) ([]servermap.Entry, error) {
	mapPath := filepath.Join(stateDir, "servers.json")
	gone := func(e servermap.Entry) bool {
		if e.Repository == repository.None {
			return false
		}
		if _, err := os.Stat(e.Repository); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		session := sessionPath(stateDir, e.Port)
		if _, err := os.Stat(session); err == nil && !slices.ContainsFunc(removed, func(f pruneFile) bool {
			return f.Kind == pruneSession && f.Path == session
		}) {
			return false
		}
		_, running, reason := ask(e.Port)
		return !running && reason == ""
	}
	if dryRun {
		m, err := servermap.Load(mapPath)
		if err != nil {
			return nil, err
		}
		var out []servermap.Entry
		for _, e := range m.Servers {
			if gone(e) {
				out = append(out, e)
			}
		}
		return out, nil
	}
	var out []servermap.Entry
	err := servermap.Update(mapPath, func(m *servermap.Map) error {
		m.Servers = slices.DeleteFunc(m.Servers, func(e servermap.Entry) bool {
			if gone(e) {
				out = append(out, e)
				return true
			}
			return false
		})
		return nil
	})
	return out, err
}

// removedNote names the reviews a removed session file held.
func removedNote(f pruneFile) string {
	if f.Kind != pruneSession || len(f.Groups) == 0 {
		return ""
	}
	return " (with " + strings.Join(f.Groups, ", ") + ")"
}
