package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// hookTimeout bounds a hook, so that a command waiting for something never
// keeps a review round open.
const hookTimeout = 10 * time.Minute

// ReviewEvent is what a hook is told about a submitted review.
type ReviewEvent struct {
	Group      string    `json:"group"`
	URL        string    `json:"url"`
	ReviewedAt time.Time `json:"reviewedAt"`
	Note       string    `json:"note,omitempty"`
	// Verdict is what the reviewer decided about the change as a whole. It
	// is here so that a hook can act on the decision without counting
	// comments or reading the prose in Prompt.
	Verdict  model.Verdict    `json:"verdict"`
	Comments []*model.Comment `json:"comments"`
	Prompt   string           `json:"prompt"`
}

// runHooks reacts to a submitted review.
//
// This is the half of the loop that does not need anyone waiting: the person
// who sent the diff may be in a meeting, or long gone, and the review still
// gets picked up when they come back to it.
func (s *Server) runHooks(g *model.Group) {
	event := ReviewEvent{
		Group:      g.Name,
		URL:        GroupURL(s.BaseURL(), g.Name),
		ReviewedAt: g.ReviewedAt,
		Note:       g.ReviewNote,
		Verdict:    g.ReviewVerdict,
		Comments:   openComments(g),
		Prompt:     Prompt(g, PromptOptions{}),
	}
	for _, h := range g.Hooks {
		go s.runHook(h, event)
	}
}

func openComments(g *model.Group) []*model.Comment {
	out := make([]*model.Comment, 0, len(g.Comments))
	for _, c := range g.Comments {
		if !c.Resolved {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) runHook(h *model.Hook, event ReviewEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	// Each half records how it went. A hook is the case where nobody is
	// waiting, so a failure has to be readable later; the log line alone
	// lives in a file nobody opens.
	if h.Command != "" {
		s.store.RecordHookCommandRun(event.Group, h.ID, s.runHookCommand(ctx, h, event))
	}
	if h.URL != "" {
		s.store.RecordHookPostRun(event.Group, h.ID, s.postHook(ctx, h, event))
	}
}

// maxHookDetail bounds the outcome kept for a hook. It is shown on one line
// by "sbnn hook", and it is written to the session file on every review, so
// a command that fails with a megabyte on stderr must not grow the file by a
// megabyte a round.
const maxHookDetail = 200

// hookRun builds the outcome record, folding a multi-line message onto the
// one line that "sbnn hook" has room for.
func hookRun(ok bool, detail string) model.HookRun {
	detail = strings.TrimSpace(detail)
	if i := strings.IndexAny(detail, "\r\n"); i >= 0 {
		detail = strings.TrimSpace(detail[:i]) + " ..."
	}
	if len(detail) > maxHookDetail {
		detail = detail[:maxHookDetail] + " ..."
	}
	return model.HookRun{At: time.Now(), OK: ok, Detail: detail}
}

// hookEnv is what a command hook is told about the review, besides the prompt
// on its stdin.
//
// SBNN_VERDICT carries the verdict exactly as the JSON event spells it, and
// stays empty for a review that has none - a hook that wants a default picks
// its own rather than being handed an invented one.
//
// SBNN_BLOCKING is the answer to the question every hook would otherwise
// re-implement: may the change go ahead? It is model.Blocks, the same rule
// wait --exit-code and submit --exit-code end on, so a hook that branches on
// it and a pipeline that branches on sbnn's exit status agree. That is more
// than the verdict alone: a review that only commented still blocks while it
// has a comment open, which is why the verdict is not consulted by itself
// here.
func (s *Server) hookEnv(event ReviewEvent) []string {
	blocking := "0"
	if model.Blocks(event.Verdict, event.Comments) {
		blocking = "1"
	}
	return []string{
		"SBNN_GROUP=" + event.Group,
		"SBNN_URL=" + event.URL,
		"SBNN_SERVER=" + s.BaseURL(),
		"SBNN_PORT=" + strconv.Itoa(s.opts.Port),
		"SBNN_COMMENTS=" + strconv.Itoa(len(event.Comments)),
		"SBNN_REVIEW_NOTE=" + event.Note,
		"SBNN_VERDICT=" + string(event.Verdict),
		"SBNN_BLOCKING=" + blocking,
	}
}

// runHookCommand runs the command through the shell, with the review prompt
// on its stdin and the details in the environment.
func (s *Server) runHookCommand(ctx context.Context, h *model.Hook, event ReviewEvent) model.HookRun {
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/c"
	}
	cmd := exec.CommandContext(ctx, shell, flag, h.Command)
	cmd.Stdin = bytes.NewReader([]byte(event.Prompt))
	cmd.Env = append(cmd.Environ(), s.hookEnv(event)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("review hook failed", "hook", h.ID, "command", h.Command, "error", err,
			"output", string(bytes.TrimSpace(out)))
		detail := err.Error()
		if trimmed := string(bytes.TrimSpace(out)); trimmed != "" {
			detail += ": " + trimmed
		}
		return hookRun(false, detail)
	}
	slog.Info("review hook ran", "hook", h.ID, "command", h.Command,
		"output", string(bytes.TrimSpace(out)))
	return hookRun(true, string(bytes.TrimSpace(out)))
}

// validateHookURL reports whether a URL is one postHook could actually
// deliver to.
//
// postHook speaks http and https and nothing else, so a hook URL that is
// neither is a hook that will never run. Left to delivery it fails once per
// review, at warn level, in a log file nobody is reading - and by then the
// person who registered it is long gone. Registration calls this too, so a
// mistyped URL is refused with a 400 while someone is still there to read
// it.
func validateHookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not a url: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		if u.Scheme == "" {
			return fmt.Errorf("a hook url has to start with http:// or https://: %q", raw)
		}
		return fmt.Errorf("a hook url has to be http or https, not %q: %q", u.Scheme, raw)
	}
	if u.Host == "" {
		return fmt.Errorf("a hook url needs a host: %q", raw)
	}
	return nil
}

func (s *Server) postHook(ctx context.Context, h *model.Hook, event ReviewEvent) model.HookRun {
	// Registration refuses these now, but a session file written before it
	// did still holds them, and they are loaded back on start.
	if err := validateHookURL(h.URL); err != nil {
		slog.Warn("review hook has an unusable url", "hook", h.ID, "url", h.URL, "error", err)
		return hookRun(false, err.Error())
	}
	body, err := json.Marshal(event)
	if err != nil {
		return hookRun(false, err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("review hook has an unusable url", "hook", h.ID, "url", h.URL, "error", err)
		return hookRun(false, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("review hook could not be delivered", "hook", h.ID, "url", h.URL, "error", err)
		return hookRun(false, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("review hook was refused", "hook", h.ID, "url", h.URL, "status", resp.Status)
		return hookRun(false, "refused with "+resp.Status)
	}
	slog.Info("review hook delivered", "hook", h.ID, "url", h.URL, "status", resp.Status)
	return hookRun(true, resp.Status)
}
