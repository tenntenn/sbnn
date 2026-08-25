// Package server implements the resident sbnn server: it holds the diffs sent
// from stdin, serves the review UI, and keeps the review comments.
package server

import (
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/model"
)

// maxDiffSize bounds a single diff sent to the server.
const maxDiffSize = 32 << 20 // 32MB

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
		slog.Warn("could not restore session", "error", err)
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
	case "", "none", "same-origin":
	default:
		return "Sec-Fetch-Site: " + site, true
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		// No browser is claiming this request. curl, the sbnn command and the
		// hooks it runs all land here.
		return "", origin == "null"
	}
	if !s.ownOrigin(origin) {
		return "Origin: " + origin, true
	}
	return "", false
}

// ownOrigin reports whether an Origin header names this server. The page is
// reached by whichever loopback name the user typed, so all of them count,
// as long as the port is the one sbnn listens on.
func (s *Server) ownOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	if u.Port() != strconv.Itoa(s.opts.Port) {
		return false
	}
	host := u.Hostname()
	if host == s.opts.Bind {
		return true
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
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
	writeJSON(w, http.StatusOK, g)
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
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDiffSize)).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
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
	if req.EndLine < req.StartLine {
		req.EndLine = req.StartLine
	}
	if req.FileID == "" {
		if req.Path == "" {
			http.Error(w, "a comment needs a fileId or a path", http.StatusBadRequest)
			return
		}
		d, f, found := s.store.FindFileByPath(name, req.DiffID, req.Path)
		if !found {
			http.Error(w, fmt.Sprintf("no diff in group %q contains %s", name, req.Path), http.StatusNotFound)
			return
		}
		req.DiffID, req.FileID = d.ID, f.ID
		if req.Snippet == "" {
			req.Snippet = diff.Snippet(f, req.Side, req.StartLine, req.EndLine)
		}
		if req.Snippet == "" {
			http.Error(w, fmt.Sprintf("%s has no line %s in this diff", req.Path, lineSpec(req.StartLine, req.EndLine)),
				http.StatusBadRequest)
			return
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
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
	if r.ContentLength > 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&h); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
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
	removed := s.store.DeleteHooks(name, r.PathValue("id"))
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
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.broker.subscribe()
	defer s.broker.unsubscribe(ch)

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdown:
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ping.C:
			io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		}
	}
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
	s.broker.publish(msg)
}

// notify tells connected browsers that a group changed.
func (s *Server) notify(group string) {
	msg, err := json.Marshal(map[string]string{"type": "change", "group": group})
	if err != nil {
		return
	}
	s.broker.publish(msg)
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

// broker fans server side events out to every connected browser.
type broker struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newBroker() *broker {
	return &broker{subs: map[chan []byte]struct{}{}}
}

func (b *broker) subscribe() chan []byte {
	ch := make(chan []byte, 8)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
	return ch
}

func (b *broker) unsubscribe(ch chan []byte) {
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

func (b *broker) publish(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // a slow browser refetches on the next event anyway
		}
	}
}
