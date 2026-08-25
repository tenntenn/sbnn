// Package server implements the resident sbnn server: it holds the diffs sent
// from stdin, serves the review UI, and keeps the review comments.
package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/model"
)

// MaxDiffSize bounds a single diff sent to the server. It is exported so that
// cmd can bound stdin by this number rather than by a copy of it; cmd still
// keeps its own maxDiffSize today, and TestMaxDiffSizeMatchesTheCommandLine
// fails if the two ever stop agreeing.
const MaxDiffSize = 32 << 20 // 32MB

// maxDiffBodySize bounds the request that carries a diff.
//
// The diff is bounded by MaxDiffSize, but it reaches the server inside a JSON
// object, and JSON only ever makes a string longer: a quote or a backslash
// doubles, a newline becomes two bytes, and the object itself adds its keys.
// Bounding the body by MaxDiffSize therefore rejected diffs that were inside
// the limit while telling them they were not - a 32,999,998-byte diff, well
// under 32MB, arrived as a 33,804,914-byte body and came back as "the diff is
// too large (max 32MB)".
//
// So the body gets its own, larger bound, and the diff is measured after
// decoding, where its real size is known and the message can be true. Twice
// MaxDiffSize covers a diff that is half quotes and backslashes, which no
// diff of source text is; a body past that is not a diff sbnn can help with
// and is refused as a body, in those words.
const maxDiffBodySize = 2*MaxDiffSize + maxBodySize

// maxBodySize bounds the small JSON bodies: a comment, a review note, a hook.
const maxBodySize = 1 << 20 // 1MB

// Options configures a Server.
type Options struct {
	// Bind and Port are the address the sbnn server listens on.
	Bind string
	Port int
	// SessionFile is where the session is persisted. Empty disables it.
	SessionFile string
	// Mo drives the Markdown preview.
	Mo *mo.Runner
	// CacheDir holds Markdown reconstructed from diffs.
	CacheDir string
	// HistoryFile is the log of submitted reviews. Empty keeps none.
	HistoryFile string
	// Version and Revision are reported by /_/api/status.
	Version  string
	Revision string
	// AllowRemote must be set to bind to a non-loopback address.
	AllowRemote bool
	// IdleTimeout ends the server once it has held nothing to review for this
	// long. Zero, the default, keeps it resident until it is told to stop.
	IdleTimeout time.Duration
}

// Server is the resident sbnn server.
type Server struct {
	opts   Options
	store  *Store
	broker *broker
	proxy  *moProxy
	prev   *previewer

	shutdown chan struct{}
	once     sync.Once
}

// New creates a server and restores the previous session.
func New(opts Options) (*Server, error) {
	if opts.Bind == "" {
		opts.Bind = "localhost"
	}
	if opts.Mo == nil {
		opts.Mo = mo.New("", 0, "")
	}
	if !opts.AllowRemote && !isLoopback(opts.Bind) {
		return nil, fmt.Errorf("refusing to bind to %s: sbnn serves local diffs and comments without authentication "+
			"(pass --dangerously-allow-remote-access if you really mean it)", opts.Bind)
	}
	s := &Server{
		opts:     opts,
		store:    NewStore(opts.SessionFile),
		broker:   newBroker(),
		shutdown: make(chan struct{}),
	}
	if err := s.store.Load(); err != nil {
		// The background server's log lands in the state directory, where
		// nobody is looking, so this also goes to stderr: losing a session
		// is worth one line wherever the human is.
		slog.Warn("could not restore session", "error", err)
		fmt.Fprintf(os.Stderr, "sbnn: %v\n", err)
	}
	// Run replaces this with a previewer that knows the frame proxy.
	s.prev = &previewer{mo: opts.Mo, cacheDir: opts.CacheDir}
	return s, nil
}

// Addr returns the host:port the server listens on.
func (s *Server) Addr() string {
	return net.JoinHostPort(s.opts.Bind, strconv.Itoa(s.opts.Port))
}

// BaseURL returns the URL of the server.
func (s *Server) BaseURL() string {
	return "http://" + s.Addr()
}

// Store exposes the session store, mainly for tests.
func (s *Server) Store() *Store { return s.store }

// Run serves until ctx is cancelled or a shutdown is requested.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", s.Addr(), err)
	}

	// The Markdown preview is mo, embedded in the review page. mo answers
	// with "frame-ancestors 'none'", so it is reached through a loopback
	// proxy that allows exactly this server's origin to frame it.
	proxy, err := newMoProxy(s.opts.Mo.BaseURL(), s.BaseURL())
	if err != nil {
		slog.Warn("Markdown preview will open in a separate window", "error", err)
	} else {
		s.proxy = proxy
		go proxy.serve()
		defer proxy.close()
	}
	s.prev = &previewer{mo: s.opts.Mo, proxy: s.proxy, cacheDir: s.opts.CacheDir}

	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	if s.opts.IdleTimeout > 0 {
		go s.watchIdle(ctx, idleCheckInterval(s.opts.IdleTimeout))
	}

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	case <-s.shutdown:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// idleCheckInterval picks how often to test for idleness: often enough that
// the timeout is honoured with reasonable precision, rarely enough that a
// long timeout does not mean a busy ticker.
func idleCheckInterval(timeout time.Duration) time.Duration {
	check := min(timeout/4, 30*time.Second)
	if check <= 0 {
		check = time.Millisecond
	}
	return check
}

// watchIdle ends the server once it has had nothing to do for IdleTimeout.
//
// The first `git diff | sbnn` starts a resident server and detaches it, and
// nothing has ever ended it: no idle timeout, no lifetime bound, no cleanup on
// logout. The only ways out are `sbnn --shutdown`, POST /_/api/shutdown, or
// killing the process, so a review from three months ago could still be
// holding a port, a session file and its parsed diffs. It is invisible too -
// the process reads like something running in a terminal, and nothing tells
// the user it is still there.
//
// The timeout must be continuous: any sign of life resets it, so a server is
// only collected after being useless for the whole stretch.
func (s *Server) watchIdle(ctx context.Context, check time.Duration) {
	t := time.NewTicker(check)
	defer t.Stop()
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case now := <-t.C:
			switch {
			case !s.idle():
				since = time.Time{}
			case since.IsZero():
				since = now
			case now.Sub(since) >= s.opts.IdleTimeout:
				slog.Info("shutting down: nothing left to review",
					"idle", s.opts.IdleTimeout)
				s.requestShutdown()
				return
			}
		}
	}
}

// idle reports whether the server is holding nothing worth staying alive for.
//
// Conservative on purpose. A review waiting for a human must never be
// collected - that is the whole point of the hooks - so this asks "nothing
// here at all" rather than "no recent activity". A group still holding a diff
// is a review someone may come back to, an open event stream is a page
// watching, and a registered hook is something waiting to fire. A server that
// has none of the three has nothing to lose.
func (s *Server) idle() bool {
	if s.broker.count() > 0 {
		return false
	}
	for _, g := range s.store.Summary("") {
		if g.Diffs > 0 || g.Hooks > 0 {
			return false
		}
	}
	return true
}

func (s *Server) requestShutdown() {
	s.once.Do(func() { close(s.shutdown) })
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /_/api/status", s.handleStatus)
	mux.HandleFunc("GET /_/api/reviews", s.handleReviews)
	mux.HandleFunc("GET /_/api/groups", s.handleGroups)
	mux.HandleFunc("DELETE /_/api/groups", s.handleDeleteAllGroups)
	mux.HandleFunc("GET /_/api/groups/{group}", s.handleGroup)
	mux.HandleFunc("DELETE /_/api/groups/{group}", s.handleDeleteGroup)
	mux.HandleFunc("POST /_/api/groups/{group}/diffs", s.handleAddDiff)
	mux.HandleFunc("DELETE /_/api/groups/{group}/diffs/{diff}", s.handleDeleteDiff)
	mux.HandleFunc("GET /_/api/groups/{group}/diffs/{diff}/files/{file}/preview", s.handlePreview)
	mux.HandleFunc("GET /_/api/groups/{group}/diffs/{diff}/files/{file}/content", s.handleFileContent)
	mux.HandleFunc("GET /_/api/groups/{group}/diffs/{diff}/files/{file}/image", s.handleFileImage)
	mux.HandleFunc("GET /_/api/groups/{group}/comments", s.handleComments)
	mux.HandleFunc("POST /_/api/groups/{group}/comments", s.handleAddComment)
	mux.HandleFunc("PATCH /_/api/groups/{group}/comments/{id}", s.handleUpdateComment)
	mux.HandleFunc("DELETE /_/api/groups/{group}/comments/{id}", s.handleDeleteComment)
	mux.HandleFunc("DELETE /_/api/groups/{group}/comments", s.handleClearComments)
	mux.HandleFunc("GET /_/api/groups/{group}/prompt", s.handlePrompt)
	mux.HandleFunc("POST /_/api/groups/{group}/review", s.handleSubmitReview)
	mux.HandleFunc("GET /_/api/groups/{group}/hooks", s.handleHooks)
	mux.HandleFunc("POST /_/api/groups/{group}/hooks", s.handleAddHook)
	mux.HandleFunc("DELETE /_/api/groups/{group}/hooks", s.handleDeleteHooks)
	mux.HandleFunc("DELETE /_/api/groups/{group}/hooks/{id}", s.handleDeleteHooks)
	mux.HandleFunc("POST /_/api/shutdown", s.handleShutdown)
	mux.HandleFunc("GET /_/events", s.handleEvents)
	mux.Handle("GET /", s.spaHandler())

	return s.withSecurityHeaders(mux)
}

// withSecurityHeaders sets a CSP for sbnn's own pages. The preview iframe is
// the one cross origin the page is allowed to load.
func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	frameSrc := "'none'"
	if s.proxy != nil {
		frameSrc = s.proxy.baseURL
	}
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"frame-src " + frameSrc,
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if reason, ok := s.crossOrigin(r); ok {
			// A page on some other site asked sbnn to change something. It
			// is not the person sitting here, whatever it says.
			slog.Warn("refused a cross-origin request", "reason", reason,
				"method", r.Method, "path", r.URL.Path)
			http.Error(w, "sbnn only takes changes from its own page or from the command line",
				http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossOrigin reports whether a state-changing request came from somewhere
// other than sbnn's own page, and why.
//
// sbnn listens on loopback with no authentication, which any website the user
// visits can reach: a POST from a page on evil.example would otherwise
// register a hook, and a hook is a shell command sbnn runs. Browsers name
// their sender - Origin, and Sec-Fetch-Site on top of it - and the command
// line sends neither, which is the whole distinction.
func (s *Server) crossOrigin(r *http.Request) (string, bool) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		// Reading is guarded by the browser: without CORS headers no other
		// site gets to see the answer.
		return "", false
	}
	// Sec-Fetch-Site is sent by every current browser and by nothing else.
	// "none" is the address bar, "same-origin" is sbnn's own page.
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "none", "same-origin":
		// The browser computed this against the page's real origin, and a
		// page cannot forge it: Sec-Fetch-* are forbidden header names, so a
		// request from anywhere else arrives as "cross-site" whatever it
		// wants to claim. Answering here instead of falling through to the
		// Origin check is what keeps writes working under --bind, where the
		// address the user typed is one sbnn was never told.
		return "", false
	case "":
		// Too old to send it, or not a browser at all. Origin decides.
	default:
		return "Sec-Fetch-Site: " + site, true
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		// No browser is claiming this request. curl, the sbnn command and the
		// hooks it runs all land here.
		return "", origin == "null"
	}
	if !s.ownOrigin(origin, r.Host) {
		return "Origin: " + origin, true
	}
	return "", false
}

// ownOrigin reports whether an Origin header names this server. The page is
// reached by whichever loopback name the user typed, so all of them count,
// as long as the port is the one sbnn listens on.
//
// host is the request's own Host header - the authority the client actually
// dialled. An Origin that matches it is by definition same-origin, which is
// the only thing that identifies sbnn's page when it is served on an address
// opts.Bind does not name: under --bind 0.0.0.0 the user reaches the page at
// the machine's LAN address, and the browser reports that back. A page on
// another site cannot borrow this: the browser sets Host from the URL it is
// dialling and Origin from the page it is dialling out of, so for a genuine
// cross-origin request the two differ.
func (s *Server) ownOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	if host != "" && u.Host == host {
		return true
	}
	if u.Port() != strconv.Itoa(s.opts.Port) {
		return false
	}
	hostname := u.Hostname()
	if hostname == s.opts.Bind {
		return true
	}
	if ip, err := netip.ParseAddr(hostname); err == nil {
		return ip.IsLoopback()
	}
	return hostname == "localhost"
}

// Status is the payload of GET /_/api/status.
type Status struct {
	App         string         `json:"app"`
	Version     string         `json:"version"`
	Revision    string         `json:"revision,omitempty"`
	PID         int            `json:"pid"`
	URL         string         `json:"url"`
	MoURL       string         `json:"moUrl"`
	MoProxyURL  string         `json:"moProxyUrl,omitempty"`
	MoAvailable bool           `json:"moAvailable"`
	MoError     string         `json:"moError,omitempty"`
	Groups      []GroupSummary `json:"groups"`
	// SessionError says why the session is not being written to disk. It is
	// empty while the session file is up to date.
	SessionError string `json:"sessionError,omitempty"`
}

func (s *Server) status() Status {
	st := Status{
		App:      "sbnn",
		Version:  s.opts.Version,
		Revision: s.opts.Revision,
		PID:      os.Getpid(),
		URL:      s.BaseURL(),
		MoURL:    s.opts.Mo.BaseURL(),
		Groups:   s.store.Summary(s.BaseURL()),
	}
	if s.proxy != nil {
		st.MoProxyURL = s.proxy.baseURL
	}
	if err := s.store.PersistError(); err != nil {
		st.SessionError = err.Error()
	}
	if err := s.opts.Mo.Available(); err != nil {
		st.MoError = err.Error()
	} else {
		st.MoAvailable = true
	}
	return st
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.status())
}

// ReviewsResponse is the payload of GET /_/api/reviews.
type ReviewsResponse struct {
	Reviews []history.Record `json:"reviews"`
	Stats   history.Stats    `json:"stats"`
}

// handleReviews serves the reviews that were submitted, so that a pile of
// them can be read together.
func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := history.Filter{Group: q.Get("group")}
	if since := q.Get("since"); since != "" {
		t, err := history.ParseSince(since, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		filter.Since = t
	}
	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 0 {
			http.Error(w, "limit must be a number", http.StatusBadRequest)
			return
		}
		filter.Limit = n
	}
	records, err := history.Load(s.opts.HistoryFile, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []history.Record{}
	}
	writeJSON(w, http.StatusOK, ReviewsResponse{Reviews: records, Stats: history.Summarize(records)})
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Summary(s.BaseURL()))
}

func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	g, found := s.store.Group(name)
	if !found {
		// An empty group is a valid state: the UI shows "waiting for a diff".
		g = &model.Group{Name: name, Diffs: []*model.Diff{}, Comments: []*model.Comment{}}
	}
	// A group that exists but holds nothing has nil slices, and those
	// marshal as null. Answering [] or null for the same state, depending
	// only on how the group came to be, is a difference every consumer of
	// the API would otherwise have to know about.
	if g.Diffs == nil {
		g.Diffs = []*model.Diff{}
	}
	if g.Comments == nil {
		g.Comments = []*model.Comment{}
	}
	writeJSON(w, http.StatusOK, withoutRawDiffs(g))
}

// withoutRawDiffs drops the original diff text from a group about to be sent
// to a client.
//
// This endpoint returns the whole group - every diff, file, hunk and line -
// and the page refetches it on every change event, so its size is paid again
// on each keystroke-sized edit anyone has open. Diff.Raw is the original diff
// text, and it is dead weight here: nothing reads it. Dropping it measured
// 7.7 KB of 52 KB at 4 files and 1.00 MB of 6.61 MB at 500. export.Build
// already drops it for the same reason.
//
// The store keeps Raw - an export or a re-parse still wants it. g is the
// store's own clone, so clearing the field here cannot reach it.
func withoutRawDiffs(g *model.Group) *model.Group {
	for _, d := range g.Diffs {
		d.Raw = ""
	}
	return g
}

// handleDeleteAllGroups closes every review at once.
func (s *Server) handleDeleteAllGroups(w http.ResponseWriter, r *http.Request) {
	removed := s.store.DeleteAllGroups()
	s.notify("")
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	if !s.store.DeleteGroup(name) {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	// The stored review notice outlives the group otherwise, and would be
	// replayed to a reconnecting browser for a group that no longer exists.
	s.broker.forgetReviews(name)
	s.notify(name)
	w.WriteHeader(http.StatusNoContent)
}

// AddDiffRequest is the body of POST /_/api/groups/{group}/diffs.
type AddDiffRequest struct {
	Title   string `json:"title"`
	BaseDir string `json:"baseDir"`
	Content string `json:"content"`
	// Labels are carried through to the review record untouched.
	Labels map[string]string `json:"labels,omitempty"`
	// Collapse names files the sender wants folded away - its generated
	// ones, whatever those are called here. sbnn matches the patterns and
	// reads nothing into them.
	Collapse []string `json:"collapse,omitempty"`
}

// AddDiffResponse is the answer to a stored diff.
type AddDiffResponse struct {
	Group string      `json:"group"`
	URL   string      `json:"url"`
	Diff  *model.Diff `json:"diff"`
}

func (s *Server) handleAddDiff(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	var req AddDiffRequest
	if !decodeBody(w, r, maxDiffBodySize, "the request body", &req) {
		return
	}
	// Now that the diff is out of its envelope, its size is the one the
	// sender knows and the one the command line prints.
	if len(req.Content) > MaxDiffSize {
		http.Error(w, fmt.Sprintf("the diff is too large (max %s)", byteLimit(MaxDiffSize)),
			http.StatusRequestEntityTooLarge)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "empty diff", http.StatusBadRequest)
		return
	}
	files := diff.Parse(req.Content)
	if len(files) == 0 {
		http.Error(w, "no file diff found in the input", http.StatusBadRequest)
		return
	}
	incoming := &model.Diff{
		Title:   req.Title,
		BaseDir: req.BaseDir,
		Labels:  req.Labels,
		Raw:     req.Content,
		Files:   files,
	}
	foldFiles(incoming, req.Collapse)
	d := s.store.AddDiff(name, incoming)
	s.notify(name)
	writeJSON(w, http.StatusOK, AddDiffResponse{
		Group: name,
		URL:   GroupURL(s.BaseURL(), name),
		Diff:  d,
	})
}

func (s *Server) handleDeleteDiff(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	if !s.store.DeleteDiff(name, r.PathValue("diff")) {
		http.Error(w, "no such diff", http.StatusNotFound)
		return
	}
	s.notify(name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	d, f, found := s.store.FileContext(name, r.PathValue("diff"), r.PathValue("file"))
	if !found {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	res, err := s.prev.preview(r.Context(), name, d, f)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, mo.ErrNotInstalled) {
			status = http.StatusFailedDependency
		} else if errors.Is(err, errNotPreviewable) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleFileContent hands out the Markdown of a file so that a client too
// narrow for mo's own layout can render it itself.
func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	d, f, found := s.store.FileContext(name, r.PathValue("diff"), r.PathValue("file"))
	if !found {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	res, err := s.prev.content(d, f)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNotPreviewable) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleFileImage hands out the raw bytes of an image file, for the <img>
// tag sbnn's own preview points at. It never involves mo, which cannot show
// images at all.
func (s *Server) handleFileImage(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	d, f, found := s.store.FileContext(name, r.PathValue("diff"), r.PathValue("file"))
	if !found {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	data, contentType, err := s.prev.image(d, f)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNotPreviewable) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(data); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	comments, found := s.store.Comments(name)
	if !found {
		comments = []*model.Comment{}
	}
	writeJSON(w, http.StatusOK, comments)
}

// AddCommentRequest is the body of POST /_/api/groups/{group}/comments.
type AddCommentRequest struct {
	// DiffID and FileID identify the commented file. A client that only
	// knows the path - an agent on the command line - may leave FileID
	// empty and let the server resolve Path against the newest diff.
	DiffID string `json:"diffId"`
	FileID string `json:"fileId"`
	// Author names who is commenting, empty for the reviewer in the browser.
	Author    string `json:"author"`
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Body      string `json:"body"`
	Snippet   string `json:"snippet"`
	// Question marks a comment that wants an answer, not a change.
	Question bool `json:"question,omitempty"`
	// Suggestion is a convenience for clients that only have the
	// replacement text: it is appended to Body as a fenced suggestion
	// block, which is where a suggestion actually lives.
	Suggestion string `json:"suggestion"`
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	var req AddCommentRequest
	if !decodeBody(w, r, maxBodySize, "the comment", &req) {
		return
	}
	body := model.WithSuggestion(req.Body, req.Suggestion)
	if strings.TrimSpace(body) == "" {
		http.Error(w, "a comment needs a body or a suggestion", http.StatusBadRequest)
		return
	}
	// The side is folded and trimmed so the API agrees with the CLI:
	// new, old, or empty (meaning new). Anything else is a caller's
	// mistake and has to be reported, not guessed at -- guessing put
	// comments on lines nobody asked about.
	switch side := strings.ToLower(strings.TrimSpace(req.Side)); side {
	case "":
		req.Side = "new"
	case "new", "old":
		req.Side = side
	default:
		http.Error(w, fmt.Sprintf("unknown side %q: use new or old", req.Side), http.StatusBadRequest)
		return
	}
	if len(model.Suggestions(body)) > 0 && req.Side == "old" {
		http.Error(w, "a suggestion replaces lines of the new file, not of the old one", http.StatusBadRequest)
		return
	}
	// Line numbers are 1-based; 0 means "not on this side" and is not a
	// place a comment can point at. The CLI already refuses these, and a
	// stored comment with a non-positive range anchors to nothing.
	if req.StartLine < 1 {
		http.Error(w, fmt.Sprintf("startLine must be 1 or more, got %d", req.StartLine), http.StatusBadRequest)
		return
	}
	if req.EndLine == 0 {
		// A client that comments on a single line may send startLine alone.
		req.EndLine = req.StartLine
	}
	if req.EndLine < req.StartLine {
		http.Error(w, fmt.Sprintf("endLine %d is before startLine %d", req.EndLine, req.StartLine), http.StatusBadRequest)
		return
	}
	// Which file the comment is on. Both shapes of the request end up
	// here: the page sends diffId and fileId, while a client that only
	// knows the path - an agent on the command line - lets the server
	// resolve it against the newest diff. The stored range is measured
	// against that file, so the file has to be found for both shapes and
	// not, as it once was, only for the path one.
	var f *model.File
	byPath := req.FileID == ""
	if byPath {
		if req.Path == "" {
			http.Error(w, "a comment needs a fileId or a path", http.StatusBadRequest)
			return
		}
		d, found, ok := s.store.FindFileByPath(name, req.DiffID, req.Path)
		if !ok {
			http.Error(w, fmt.Sprintf("no diff in group %q contains %s", name, req.Path), http.StatusNotFound)
			return
		}
		f = found
		req.DiffID, req.FileID = d.ID, f.ID
	} else {
		var ok bool
		if _, f, ok = s.store.FileContext(name, req.DiffID, req.FileID); !ok {
			// A fileId that names no file of this diff anchors the
			// comment to nothing: the page keys its sections on
			// diffId:fileId, so such a comment is counted in every total
			// and shown on no line. An unknown diffId is left alone here,
			// because AddComment below already reports that one.
			if g, found := s.store.Group(name); found && g.FindDiff(req.DiffID) != nil {
				http.Error(w, fmt.Sprintf("no file %q in diff %q", req.FileID, req.DiffID), http.StatusBadRequest)
				return
			}
		}
	}

	if f != nil {
		if req.Snippet == "" {
			req.Snippet = diff.Snippet(f, req.Side, req.StartLine, req.EndLine)
		}
		if byPath && req.Snippet == "" {
			http.Error(w, fmt.Sprintf("%s has no line %s in this diff", req.Path, lineSpec(req.StartLine, req.EndLine)),
				http.StatusBadRequest)
			return
		}
		// A snippet only has to be non-empty to be accepted, so a range
		// whose start is inside a hunk but whose end runs past it used to
		// be stored exactly as asked. The page draws a comment on the row
		// matching its endLine, so an endLine the diff never showed put
		// the comment on no row at all. Clamp instead of refusing: the
		// range selection in the page overshoots by a line at the edges of
		// a hunk, and a 400 there would break a gesture that works today.
		if last := lastCoveredLine(f, req.Side, req.StartLine, req.EndLine); last > 0 && last < req.EndLine {
			req.EndLine = last
		}
	}
	c, err := s.store.AddComment(&model.Comment{
		Group:     name,
		DiffID:    req.DiffID,
		FileID:    req.FileID,
		Author:    req.Author,
		Path:      req.Path,
		Side:      req.Side,
		StartLine: req.StartLine,
		EndLine:   req.EndLine,
		Body:      body,
		Question:  req.Question,
		Snippet:   req.Snippet,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.notify(name)
	writeJSON(w, http.StatusOK, c)
}

// UpdateCommentRequest is the body of PATCH .../comments/{id}.
type UpdateCommentRequest struct {
	Body     *string `json:"body,omitempty"`
	Resolved *bool   `json:"resolved,omitempty"`
	Question *bool   `json:"question,omitempty"`
}

// lastCoveredLine reports the highest line number on this side that the
// file really has within [start, end]: the last line a snippet taken over
// that range covered. It picks lines by the same rule Snippet does, so the
// two always agree on where a range stopped. It returns 0 when the range
// covers nothing at all.
func lastCoveredLine(f *model.File, side string, start, end int) int {
	last := 0
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			num := l.NewNumber
			if side == "old" {
				num = l.OldNumber
			}
			if num < start || num > end {
				continue
			}
			if side == "old" && l.Kind == model.LineAdd {
				continue
			}
			if side != "old" && l.Kind == model.LineDelete {
				continue
			}
			if num > last {
				last = num
			}
		}
	}
	return last
}

// lineSpec formats a line range for an error message.
func lineSpec(start, end int) string {
	if end > start {
		return fmt.Sprintf("%d-%d", start, end)
	}
	return strconv.Itoa(start)
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	var req UpdateCommentRequest
	if !decodeBody(w, r, maxBodySize, "the comment", &req) {
		return
	}
	c, found := s.store.UpdateComment(name, r.PathValue("id"), CommentPatch{
		Body:     req.Body,
		Resolved: req.Resolved,
		Question: req.Question,
	})
	if !found {
		http.Error(w, "no such comment", http.StatusNotFound)
		return
	}
	s.notify(name)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	if !s.store.DeleteComment(name, r.PathValue("id")) {
		http.Error(w, "no such comment", http.StatusNotFound)
		return
	}
	s.notify(name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearComments(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	removed := s.store.ClearComments(name, r.URL.Query().Get("resolved") == "true")
	s.notify(name)
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	g, found := s.store.Group(name)
	if !found {
		g = &model.Group{Name: name}
	}
	opts := PromptOptions{
		IncludeResolved: r.URL.Query().Get("resolved") == "true",
		NoInstruction:   r.URL.Query().Get("instruction") == "false",
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, Prompt(g, opts))
}

// SubmitReviewRequest is the body of POST .../review.
type SubmitReviewRequest struct {
	Note string `json:"note"`
	// Verdict is approved, commented or changes-requested; empty means
	// commented, which is what a reviewer who did not choose is saying.
	Verdict string `json:"verdict,omitempty"`
}

// handleSubmitReview records that the human is done. It is the moment the
// comments become worth reading, and the moment the hooks fire.
func (s *Server) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	var req SubmitReviewRequest
	// ContentLength is -1 when the length is unknown, which is what a
	// chunked request and many HTTP/2 clients send. Only 0 promises there
	// is no body, so decode for anything else: a verdict dropped here is
	// recorded as a plain "commented", and --exit-code downstream then
	// reports the opposite of what the reviewer decided.
	if r.ContentLength != 0 {
		// The body may still turn out to be empty, which is not an error:
		// the fields are all optional and default to a commented review.
		if !decodeOptionalBody(w, r, maxBodySize, "the review", &req) {
			return
		}
	}
	verdict, ok := model.ParseVerdict(req.Verdict)
	if !ok {
		http.Error(w, "verdict must be approved, commented or changes-requested", http.StatusBadRequest)
		return
	}
	g, found := s.store.SubmitReview(name, req.Note, verdict)
	if !found {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	// A review is worth keeping past the round it belongs to: it is the
	// only record of what this reviewer looks for.
	if err := history.Append(s.opts.HistoryFile, history.FromGroup(g)); err != nil {
		slog.Warn("could not record the review", "error", err)
	}
	s.notifyReview(g)
	s.runHooks(g)
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleHooks(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	hooks := s.store.Hooks(name)
	if hooks == nil {
		hooks = []*model.Hook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (s *Server) handleAddHook(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	var h model.Hook
	if !decodeBody(w, r, maxBodySize, "the hook", &h) {
		return
	}
	added, err := s.store.AddHook(name, &model.Hook{Command: h.Command, URL: h.URL})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, added)
}

func (s *Server) handleDeleteHooks(w http.ResponseWriter, r *http.Request) {
	name, ok := s.groupParam(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	removed := s.store.DeleteHooks(name, id)
	// The by-id route has to say when it matched nothing, or a typo'd or
	// already-deleted id looks exactly like a success. Every other by-id
	// delete in the API answers 404 for that. The clear-all route keeps
	// its 200 and its count: removing nothing from an empty list is what
	// was asked for, not a miss.
	if id != "" && removed == 0 {
		http.Error(w, "no such hook", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.requestShutdown()
	}()
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	// Take the slot before promising a stream, so a refusal is a plain 503
	// and not a 200 that ends immediately.
	ch, ok := s.broker.subscribe()
	if !ok {
		slog.Warn("refused an event subscriber: too many are already connected",
			"max", maxSubscribers)
		http.Error(w, "too many event subscribers", http.StatusServiceUnavailable)
		return
	}
	defer s.broker.unsubscribe(ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// Set the reconnect delay rather than leaving it to the browser default.
	io.WriteString(w, "retry: 2000\n\n")
	flusher.Flush()

	// Replay the review notices this client has not seen - but only for a
	// client that says where it left off. A browser resends the last id it
	// got as Last-Event-ID when it reconnects, and that is the one case
	// where a stored notice is news.
	//
	// A client opening the stream for the first time has missed nothing by
	// definition, and replaying to it is actively wrong: `sbnn wait` opens a
	// fresh stream with no Last-Event-ID, so handing it every group's last
	// review made it return a review submitted before anyone asked it to
	// wait. The diffs sent since then would have gone unreviewed with the
	// caller told otherwise. runWait already answers "the review is already
	// in" from the group's own state before it ever reaches the stream.
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if since, err := strconv.ParseUint(v, 10, 64); err == nil {
			for _, ev := range s.broker.missedReviews(since) {
				writeEvent(w, ev)
			}
			flusher.Flush()
		}
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdown:
			return
		case ev := <-ch:
			writeEvent(w, ev)
			flusher.Flush()
		case <-ping.C:
			io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// writeEvent frames one SSE message. Only review notices carry an id, which is
// what a reconnecting client echoes back in Last-Event-ID.
func writeEvent(w io.Writer, ev event) {
	if ev.id != 0 {
		fmt.Fprintf(w, "id: %d\n", ev.id)
	}
	fmt.Fprintf(w, "data: %s\n\n", ev.data)
}

// notifyReview tells everyone listening that a review was submitted. An
// agent waiting with `sbnn wait` is one of them.
func (s *Server) notifyReview(g *model.Group) {
	msg, err := json.Marshal(map[string]any{
		"type":       "review",
		"group":      g.Name,
		"reviewedAt": g.ReviewedAt,
		"comments":   len(openComments(g)),
		"verdict":    g.ReviewVerdict,
	})
	if err != nil {
		return
	}
	s.broker.publishReview(g.Name, msg)
}

// notify tells connected browsers that a group changed.
func (s *Server) notify(group string) {
	msg, err := json.Marshal(map[string]string{"type": "change", "group": group})
	if err != nil {
		return
	}
	s.broker.publishChange(msg)
}

// decodeBody decodes a JSON request body under a size limit, answering the
// client itself when it cannot.
//
// The limit used to be an io.LimitReader, which does not report that it
// truncated - it simply ends the stream. The decoder then met a body cut
// mid-JSON and blamed the JSON, so a large but perfectly valid diff came back
// as "400 invalid request: unexpected EOF": nothing named the limit, and
// nothing said a limit was involved. http.MaxBytesReader reports the overrun
// as *http.MaxBytesError, which lets us say what actually happened, and it
// stops the upload instead of letting the client finish sending a body that
// is already rejected.
func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, what string, dst any) bool {
	return decodeJSON(w, r, limit, what, dst, false)
}

// decodeOptionalBody is decodeBody for a request whose body may legitimately
// be absent. An empty body leaves dst at its zero value instead of failing;
// an overrun and malformed JSON are answered exactly as decodeBody answers
// them.
func decodeOptionalBody(w http.ResponseWriter, r *http.Request, limit int64, what string, dst any) bool {
	return decodeJSON(w, r, limit, what, dst, true)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, what string, dst any, emptyOK bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if emptyOK && errors.Is(err, io.EOF) {
			return true
		}
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, fmt.Sprintf("%s is too large (max %s)", what, byteLimit(limit)),
				http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// byteLimit renders a limit the way the command line words one: "32MB",
// "65MB", "512KB", "900B".
//
// Dividing by a megabyte and stopping there was not enough: every limit under
// a megabyte came out as "0MB", so a refusal could name a limit of nothing and
// leave the sender no idea what would fit.
func byteLimit(n int64) string {
	scaled := func(unit int64, suffix string) string {
		if n%unit == 0 {
			return strconv.FormatInt(n/unit, 10) + suffix
		}
		return strconv.FormatFloat(float64(n)/float64(unit), 'f', 1, 64) + suffix
	}
	switch {
	case n >= 1<<20:
		return scaled(1<<20, "MB")
	case n >= 1<<10:
		return scaled(1<<10, "KB")
	default:
		return strconv.FormatInt(n, 10) + "B"
	}
}

func (s *Server) groupParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	name, err := ValidateGroupName(r.PathValue("group"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	return name, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Warn("failed to write response", "error", err)
	}
}

func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	addrs, err := net.LookupIP(host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, ip := range addrs {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

// maxSubscribers bounds how many event streams can be open at once.
//
// /_/events is a GET, so crossOrigin deliberately lets it through: CORS keeps
// another site from *reading* the events. It does not keep that site from
// *holding the connection open*, and sbnn has no authentication by design. Any
// page the user visits can point unlimited EventSources at localhost:6280 and
// keep them, and each one costs a goroutine, a channel and a 25-second ticker.
// A cap turns that from unbounded growth into a refusal.
const maxSubscribers = 64

// event is one server-sent event. A non-zero id marks a review notice, which
// is kept for replay and is not dropped when a subscriber falls behind; change
// notices carry id 0.
type event struct {
	id   uint64
	data []byte
}

// broker fans server side events out to every connected browser.
type broker struct {
	mu   sync.Mutex
	subs map[chan event]struct{}
	// seq numbers review notices so a reconnecting client can say what it
	// already has, via SSE's id:/Last-Event-ID.
	seq uint64
	// reviews holds the most recent review notice per group, so a client that
	// missed one while catching up still gets it when it reconnects.
	reviews map[string]event
}

func newBroker() *broker {
	return &broker{
		subs:    map[chan event]struct{}{},
		reviews: map[string]event{},
	}
}

// subscribe registers a listener, reporting false when the cap is reached.
func (b *broker) subscribe() (chan event, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subs) >= maxSubscribers {
		return nil, false
	}
	ch := make(chan event, 8)
	b.subs[ch] = struct{}{}
	return ch, true
}

func (b *broker) unsubscribe(ch chan event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
}

// count reports how many listeners are connected, which tests use to know
// that a subscriber is ready.
func (b *broker) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// missedReviews returns the stored review notices newer than since, oldest
// first. since is what the client reported in Last-Event-ID; zero means it has
// seen nothing and wants every group's latest.
func (b *broker) missedReviews(since uint64) []event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []event
	for _, ev := range b.reviews {
		if ev.id > since {
			out = append(out, ev)
		}
	}
	slices.SortFunc(out, func(a, b event) int { return cmp.Compare(a.id, b.id) })
	return out
}

// forgetReviews drops the stored review notice for a group, so that closing a
// review does not leave a notice behind to be replayed for a group that is
// gone. Without it b.reviews only ever grows.
func (b *broker) forgetReviews(group string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.reviews, group)
}

// publishChange tells subscribers a group changed. Losing one of these costs
// nothing: the next one supersedes it, and the browser refetches the group.
func (b *broker) publishChange(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fanout(event{data: msg})
}

// publishReview tells subscribers a review was submitted, and remembers it.
// This is the notice that wakes `sbnn wait`, so it is numbered, stored for
// replay, and not dropped in favour of change notices.
func (b *broker) publishReview(group string, msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	ev := event{id: b.seq, data: msg}
	b.reviews[group] = ev
	b.fanout(ev)
}

// fanout queues ev for every subscriber. b.mu must be held.
func (b *broker) fanout(ev event) {
	for ch := range b.subs {
		deliver(ch, ev)
	}
}

// deliver queues one event for one subscriber without blocking the publisher.
//
// A change notice is dropped if the queue is full, as it always was: a slow
// browser refetches on the next event anyway. A review notice is not. Its
// reasoning only held while more events were coming, and the review notice is
// typically the *last* of a burst - a subscriber more than a queue behind when
// a review lands never learned it happened, and `sbnn wait` then blocked
// forever on a review that already finished. So make room for it by discarding
// queued change notices, keeping any review notices in order.
func deliver(ch chan event, ev event) {
	select {
	case ch <- ev:
		return
	default:
	}
	if ev.id == 0 {
		return
	}
	keep := make([]event, 0, cap(ch)+1)
drain:
	for {
		select {
		case old := <-ch:
			if old.id != 0 {
				keep = append(keep, old)
			}
		default:
			break drain
		}
	}
	keep = append(keep, ev)
	for _, q := range keep {
		select {
		case ch <- q:
		default:
			slog.Warn("dropped a review notice: the event queue is full", "id", q.id)
		}
	}
}
