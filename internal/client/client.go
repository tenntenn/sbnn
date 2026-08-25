// Package client talks to a running sbnn server.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// Client is an HTTP client for the sbnn API.
type Client struct {
	Addr string
	HTTP *http.Client
}

// New returns a client for the server at addr (host:port).
func New(addr string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Client{Addr: addr, HTTP: &http.Client{Timeout: timeout}}
}

// BaseURL returns the URL of the server.
func (c *Client) BaseURL() string { return "http://" + c.Addr }

func (c *Client) url(format string, args ...any) string {
	return c.BaseURL() + fmt.Sprintf(format, args...)
}

// Status reports the state of the server. It doubles as the probe telling
// whether a sbnn server owns the port.
func (c *Client) Status(ctx context.Context) (*server.Status, error) {
	var st server.Status
	if err := c.do(ctx, http.MethodGet, c.url("/_/api/status"), nil, &st); err != nil {
		return nil, err
	}
	if st.App != "sbnn" {
		return nil, fmt.Errorf("the server on %s is not sbnn", c.Addr)
	}
	return &st, nil
}

// AddDiff sends a diff to a group.
func (c *Client) AddDiff(ctx context.Context, group string, req server.AddDiffRequest) (*server.AddDiffResponse, error) {
	var res server.AddDiffResponse
	if err := c.do(ctx, http.MethodPost, c.url("/_/api/groups/%s/diffs", url.PathEscape(group)), req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Group returns a whole group: its diffs and its comments.
func (c *Client) Group(ctx context.Context, group string) (*model.Group, error) {
	var g model.Group
	if err := c.do(ctx, http.MethodGet, c.url("/_/api/groups/%s", url.PathEscape(group)), nil, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// AddComment leaves a review comment.
func (c *Client) AddComment(ctx context.Context, group string, req server.AddCommentRequest) (*model.Comment, error) {
	var out model.Comment
	if err := c.do(ctx, http.MethodPost, c.url("/_/api/groups/%s/comments", url.PathEscape(group)), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Comments returns the review comments of a group.
func (c *Client) Comments(ctx context.Context, group string) ([]*model.Comment, error) {
	var comments []*model.Comment
	if err := c.do(ctx, http.MethodGet, c.url("/_/api/groups/%s/comments", url.PathEscape(group)), nil, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// Prompt returns the review comments of a group rendered for an agent. When
// instruction is false the closing "address every comment" line is left out.
func (c *Client) Prompt(ctx context.Context, group string, includeResolved, instruction bool) (string, error) {
	q := url.Values{}
	if includeResolved {
		q.Set("resolved", "true")
	}
	if !instruction {
		q.Set("instruction", "false")
	}
	u := c.url("/_/api/groups/%s/prompt", url.PathEscape(group))
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

// ClearComments removes the comments of a group and returns how many were
// removed.
func (c *Client) ClearComments(ctx context.Context, group string, resolvedOnly bool) (int, error) {
	u := c.url("/_/api/groups/%s/comments", url.PathEscape(group))
	if resolvedOnly {
		u += "?resolved=true"
	}
	var res struct {
		Removed int `json:"removed"`
	}
	if err := c.do(ctx, http.MethodDelete, u, nil, &res); err != nil {
		return 0, err
	}
	return res.Removed, nil
}

// DeleteGroup drops a whole group, diffs and comments alike.
func (c *Client) DeleteGroup(ctx context.Context, group string) error {
	return c.do(ctx, http.MethodDelete, c.url("/_/api/groups/%s", url.PathEscape(group)), nil, nil)
}

// DeleteAllGroups closes every review and returns how many went.
func (c *Client) DeleteAllGroups(ctx context.Context) (int, error) {
	var res struct {
		Removed int `json:"removed"`
	}
	if err := c.do(ctx, http.MethodDelete, c.url("/_/api/groups"), nil, &res); err != nil {
		return 0, err
	}
	return res.Removed, nil
}

// Reviews returns the reviews the server has recorded.
func (c *Client) Reviews(ctx context.Context, filter history.Filter) ([]history.Record, error) {
	q := url.Values{}
	if filter.Group != "" {
		q.Set("group", filter.Group)
	}
	if !filter.Since.IsZero() {
		q.Set("since", filter.Since.Format(time.RFC3339))
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.Itoa(filter.Limit))
	}
	u := c.url("/_/api/reviews")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var res server.ReviewsResponse
	if err := c.do(ctx, http.MethodGet, u, nil, &res); err != nil {
		return nil, err
	}
	return res.Reviews, nil
}

// Shutdown asks the server to stop.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, c.url("/_/api/shutdown"), nil, nil)
}

func (c *Client) do(ctx context.Context, method, u string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		text := strings.TrimSpace(string(msg))
		if text == "" {
			text = resp.Status
		}
		return fmt.Errorf("%s %s: %s", method, u, text)
	}
	if out == nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

// SubmitReview marks the review of a group as done, which is what wakes
// anything waiting on it.
func (c *Client) SubmitReview(ctx context.Context, group, note string, verdict model.Verdict) (*model.Group, error) {
	var g model.Group
	body := server.SubmitReviewRequest{Note: note, Verdict: string(verdict)}
	if err := c.do(ctx, http.MethodPost, c.url("/_/api/groups/%s/review", url.PathEscape(group)), body, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// Hooks returns what will run when a review of the group is submitted.
func (c *Client) Hooks(ctx context.Context, group string) ([]*model.Hook, error) {
	var hooks []*model.Hook
	if err := c.do(ctx, http.MethodGet, c.url("/_/api/groups/%s/hooks", url.PathEscape(group)), nil, &hooks); err != nil {
		return nil, err
	}
	return hooks, nil
}

// AddHook registers a command or a URL to be run when a review is submitted.
func (c *Client) AddHook(ctx context.Context, group string, h model.Hook) (*model.Hook, error) {
	var out model.Hook
	if err := c.do(ctx, http.MethodPost, c.url("/_/api/groups/%s/hooks", url.PathEscape(group)), h, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteHooks removes the hooks of a group and returns how many went.
func (c *Client) DeleteHooks(ctx context.Context, group string) (int, error) {
	var res struct {
		Removed int `json:"removed"`
	}
	if err := c.do(ctx, http.MethodDelete, c.url("/_/api/groups/%s/hooks", url.PathEscape(group)), nil, &res); err != nil {
		return 0, err
	}
	return res.Removed, nil
}

// DeleteHook removes one hook by ID and returns how many went, which is 0
// when the group has no hook with that ID: the server reports that as a
// count, not as an error, so the caller decides what it means.
func (c *Client) DeleteHook(ctx context.Context, group, id string) (int, error) {
	var res struct {
		Removed int `json:"removed"`
	}
	u := c.url("/_/api/groups/%s/hooks/%s", url.PathEscape(group), url.PathEscape(id))
	if err := c.do(ctx, http.MethodDelete, u, nil, &res); err != nil {
		return 0, err
	}
	return res.Removed, nil
}

// ReviewNotice is what the server pushes when a review is submitted.
type ReviewNotice struct {
	Type       string    `json:"type"`
	Group      string    `json:"group"`
	ReviewedAt time.Time `json:"reviewedAt"`
	Comments   int       `json:"comments"`
}

// WaitForReview blocks on the server's event stream until the given group is
// reviewed. Nothing is polled: the server pushes the notice.
func (c *Client) WaitForReview(ctx context.Context, group string) (*ReviewNotice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/_/events"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	// The stream stays open for as long as the review takes, so this one
	// request must not carry the client timeout.
	stream := &http.Client{}
	resp, err := stream.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cannot listen to %s: %s", c.Addr, resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var notice ReviewNotice
		if err := json.Unmarshal([]byte(data), &notice); err != nil {
			continue
		}
		if notice.Type == "review" && (notice.Group == "" || notice.Group == group) {
			return &notice, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("the sbnn server closed the event stream")
}
