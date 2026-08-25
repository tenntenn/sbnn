package server

// Tests for the request body limits: what a body over the limit is told, and
// that the diff limit is measured on the diff rather than on its JSON
// envelope. They sit in their own file rather than at the end of
// server_test.go so that other work on internal/server does not have to land
// in the same place.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A body over the limit used to be cut mid-JSON by an io.LimitReader, which
// does not say it truncated. The decoder then blamed the JSON, so a large but
// perfectly valid request came back as "400 invalid request: unexpected EOF" -
// the user is told they sent something malformed, nothing names the limit, and
// nothing says a limit was involved. The CLI already words it properly on the
// same value; every other client (curl, an MCP server, an editor extension)
// got the obscure one.
func TestOversizedBodyNamesTheLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	file := added.Diff.Files[0]

	// A review note with a large stack trace pasted into it.
	huge := strings.Repeat("x", maxBodySize+(1<<10))
	body, err := json.Marshal(AddCommentRequest{
		DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(),
		Side: "new", StartLine: 2, Body: huge,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/comments",
		"application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %s, want 413: %s", resp.Status, got)
	}
	if strings.Contains(string(got), "unexpected EOF") {
		t.Errorf("body = %q, want the limit named rather than the JSON blamed", got)
	}
	if !strings.Contains(string(got), "too large") || !strings.Contains(string(got), "1MB") {
		t.Errorf("body = %q, want it to name the limit", got)
	}
}

// The two failures have to stay distinguishable: an overrun is 413 and names
// the limit, genuinely malformed JSON under the limit is still 400.
func TestDecodeBodySeparatesOverrunFromBadJSON(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		limit    int64
		want     int
		contains string
	}{
		{"a body over the limit", `{"content":"` + strings.Repeat("x", 128) + `"}`, 32,
			http.StatusRequestEntityTooLarge, "too large"},
		{"malformed JSON under the limit", `{"content":`, 1 << 20,
			http.StatusBadRequest, "invalid request"},
		{"a body that fits", `{"content":"ok"}`, 1 << 20, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var dst AddDiffRequest
			if decodeBody(w, r, tc.limit, "the diff", &dst) {
				if tc.want != http.StatusOK {
					t.Fatalf("decodeBody accepted %q, want %d", tc.body, tc.want)
				}
				return
			}
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", w.Code, tc.want, w.Body)
			}
			if !strings.Contains(w.Body.String(), tc.contains) {
				t.Errorf("body = %q, want it to contain %q", w.Body, tc.contains)
			}
		})
	}
}

// The command line and the server bound a diff by the same number, and they
// are still two constants in two packages. The old test here only compared
// MaxDiffSize with the literal it is written as, so cmd could be changed to
// 4MB and everything stayed green. This one reads cmd's constant and fails
// when the two drift, which is the whole reason the server's is exported.
func TestMaxDiffSizeMatchesTheCommandLine(t *testing.T) {
	path := filepath.Join("..", "..", "cmd", "root.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^const maxDiffSize = (.+)$`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("no `const maxDiffSize` in %s; if cmd now uses server.MaxDiffSize, delete this test", path)
	}
	got, err := constExpr(m[1])
	if err != nil {
		t.Fatalf("cmd/root.go: maxDiffSize = %q: %v", m[1], err)
	}
	if got != MaxDiffSize {
		t.Errorf("cmd/root.go maxDiffSize = %d, server.MaxDiffSize = %d; the two ends of the same limit have drifted",
			got, MaxDiffSize)
	}
}

// constExpr evaluates the handful of shapes the limit is written in: a plain
// number, or "n << k".
func constExpr(src string) (int64, error) {
	src = strings.TrimSpace(strings.SplitN(src, "//", 2)[0])
	lhs, rhs, shift := strings.Cut(src, "<<")
	n, err := strconv.ParseInt(strings.TrimSpace(lhs), 10, 64)
	if err != nil {
		return 0, err
	}
	if !shift {
		return n, nil
	}
	k, err := strconv.ParseInt(strings.TrimSpace(rhs), 10, 64)
	if err != nil {
		return 0, err
	}
	return n << k, nil
}

// A diff inside MaxDiffSize has to be accepted however much JSON escaping adds
// to it on the way. Bounding the request body by MaxDiffSize meant a
// 32,999,998-byte diff - under 32MB, and accepted by `sbnn`'s own stdin check
// - arrived as a 33,804,914-byte body and was refused with "the diff is too
// large (max 32MB)", which was not true of the diff.
func TestADiffJustUnderTheLimitIsAccepted(t *testing.T) {
	ts, _ := newTestServer(t)

	// Every line carries a quote, so JSON escaping grows the body the way the
	// reported diff did.
	var b strings.Builder
	b.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,2 @@\n-old\n")
	for b.Len() < MaxDiffSize-64 {
		b.WriteString("+a line with \"quotes\" in it\n")
	}
	content := b.String()
	if len(content) > MaxDiffSize {
		t.Fatalf("test diff is %d bytes, over the limit it is meant to sit under", len(content))
	}

	body, err := json.Marshal(AddDiffRequest{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) <= MaxDiffSize {
		t.Fatalf("body is %d bytes, not over MaxDiffSize (%d): this test would pass without the fix",
			len(body), MaxDiffSize)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/diffs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %s for a %d-byte diff in a %d-byte body, want 200: %s",
			resp.Status, len(content), len(body), got)
	}
}

// ...and a diff genuinely over the limit is still refused, with the size the
// sender can check: the diff's, not the envelope's.
func TestADiffOverTheLimitNamesTheDiff(t *testing.T) {
	ts, _ := newTestServer(t)

	content := "diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,2 @@\n-old\n+" +
		strings.Repeat("x", MaxDiffSize) + "\n"
	body, err := json.Marshal(AddDiffRequest{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/_/api/groups/default/diffs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %s, want 413: %s", resp.Status, got)
	}
	if !strings.Contains(string(got), "the diff is too large (max 32MB)") {
		t.Errorf("body = %q, want the diff named with its own limit", got)
	}
}

// byteLimit is what every one of those messages ends with, so a limit it
// cannot render is a message that tells the sender nothing.
func TestByteLimit(t *testing.T) {
	for _, tt := range []struct {
		want string
		in   int64
	}{
		{in: MaxDiffSize, want: "32MB"},
		{in: maxDiffBodySize, want: "65MB"},
		{in: maxBodySize, want: "1MB"},
		// Regression: everything under a megabyte used to render "0MB", so a
		// refusal named a limit of nothing.
		{in: 512 << 10, want: "512KB"},
		{in: 1 << 10, want: "1KB"},
		{in: 900, want: "900B"},
		{in: 0, want: "0B"},
		{in: 3 << 19, want: "1.5MB"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			if got := byteLimit(tt.in); got != tt.want {
				t.Errorf("byteLimit(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
