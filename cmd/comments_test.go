package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/model"
)

// reviewedGroup is a group whose review is over, with a verdict and a
// number of comments left open by it.
func reviewedGroup(verdict model.Verdict, open, resolved int) *model.Group {
	g := &model.Group{
		Name:          "default",
		ReviewedAt:    time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		ReviewVerdict: verdict,
	}
	for i := range open {
		g.Comments = append(g.Comments, &model.Comment{
			ID: fmt.Sprintf("open-%d", i), Group: g.Name,
			Path: "main.go", Side: "new", StartLine: 1, EndLine: 1,
			Body: "something to address",
		})
	}
	for i := range resolved {
		g.Comments = append(g.Comments, &model.Comment{
			ID: fmt.Sprintf("done-%d", i), Group: g.Name,
			Path: "main.go", Side: "new", StartLine: 2, EndLine: 2,
			Body: "already handled", Resolved: true,
		})
	}
	return g
}

// serveGroup stands in for the sbnn server with just the endpoints the
// comments command reads.
func serveGroup(t *testing.T, g *model.Group) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/_/api/status", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"app": "sbnn"})
	})
	mux.HandleFunc("/_/api/groups/"+g.Name, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(g)
	})
	mux.HandleFunc("/_/api/groups/"+g.Name+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(g.Comments)
	})
	mux.HandleFunc("/_/api/groups/"+g.Name+"/prompt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%d comment(s)\n", len(g.Comments))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	oldBind, oldPort, oldTarget := bind, port, target
	t.Cleanup(func() { bind, port, target = oldBind, oldPort, oldTarget })
	bind, port, target = host, p, g.Name
}

// The exit status of sbnn comments --exit-code is the reviewer's verdict,
// the same answer sbnn wait and sbnn submit give. It used to count open
// comments instead, so an approval carrying two remarks exited 1 and the
// pipeline the help suggests - sbnn wait && sbnn comments -q && git commit -
// stopped on a review that had said yes.
func TestCommentsExitCodeFollowsTheVerdict(t *testing.T) {
	tests := []struct {
		name     string
		verdict  model.Verdict
		open     int
		resolved int
		mode     string
		want     int
	}{
		{name: "approved with remarks", verdict: model.VerdictApproved, open: 2, mode: "quiet", want: 0},
		{name: "approved and silent", verdict: model.VerdictApproved, mode: "quiet", want: 0},
		{name: "changes requested with no comment", verdict: model.VerdictChangesRequested, mode: "quiet", want: 1},
		{name: "changes requested with comments", verdict: model.VerdictChangesRequested, open: 1, mode: "quiet", want: 1},
		{name: "commented with something open", verdict: model.VerdictCommented, open: 1, mode: "quiet", want: 1},
		{name: "commented with everything resolved", verdict: model.VerdictCommented, resolved: 2, mode: "quiet", want: 0},
		{name: "not submitted yet", open: 1, mode: "quiet", want: 1},

		// --exit-code prints as well, through either printer.
		{name: "approved, prompt output", verdict: model.VerdictApproved, open: 2, mode: "exit-code", want: 0},
		{name: "approved, json output", verdict: model.VerdictApproved, open: 2, mode: "json", want: 0},
		{name: "changes requested, prompt output", verdict: model.VerdictChangesRequested, mode: "exit-code", want: 1},
		{name: "changes requested, json output", verdict: model.VerdictChangesRequested, mode: "json", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runCommentsExitStatus(t, tt.verdict, tt.open, tt.resolved, tt.mode)
			if got != tt.want {
				t.Errorf("sbnn comments (%s) on a %q review with %d open and %d resolved comment(s) exited %d, want %d",
					tt.mode, verdictName(tt.verdict), tt.open, tt.resolved, got, tt.want)
			}
		})
	}
}

func verdictName(v model.Verdict) string {
	if v == "" {
		return "not submitted"
	}
	return string(v)
}

// The command ends by calling os.Exit, so its status can only be observed
// from outside: the test binary re-runs itself as the child below.
const (
	childEnv         = "SBNN_TEST_COMMENTS_CHILD"
	childVerdictEnv  = "SBNN_TEST_COMMENTS_VERDICT"
	childOpenEnv     = "SBNN_TEST_COMMENTS_OPEN"
	childResolvedEnv = "SBNN_TEST_COMMENTS_RESOLVED"
	childModeEnv     = "SBNN_TEST_COMMENTS_MODE"
)

func runCommentsExitStatus(t *testing.T, verdict model.Verdict, open, resolved int, mode string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCommentsExitCodeChild", "-test.v=false")
	cmd.Env = append(os.Environ(),
		childEnv+"=1",
		childVerdictEnv+"="+string(verdict),
		childOpenEnv+"="+strconv.Itoa(open),
		childResolvedEnv+"="+strconv.Itoa(resolved),
		childModeEnv+"="+mode,
		TargetEnv+"=",
	)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		t.Fatalf("running the child: %v\n%s", err, out)
		return -1
	}
}

// TestCommentsExitCodeChild is the other half of the test above: it runs
// the real command against a stub server and lets it exit as it likes.
func TestCommentsExitCodeChild(t *testing.T) {
	if os.Getenv(childEnv) == "" {
		t.Skip("run from TestCommentsExitCodeFollowsTheVerdict")
	}
	open, _ := strconv.Atoi(os.Getenv(childOpenEnv))
	resolved, _ := strconv.Atoi(os.Getenv(childResolvedEnv))
	serveGroup(t, reviewedGroup(model.Verdict(os.Getenv(childVerdictEnv)), open, resolved))

	commentsQuiet, commentsExitCode, commentsJSON = false, false, false
	switch os.Getenv(childModeEnv) {
	case "quiet":
		commentsQuiet = true
	case "exit-code":
		commentsExitCode = true
	case "json":
		commentsExitCode, commentsJSON = true, true
	}
	commentsFormat, commentsClear, commentsResolved = "prompt", false, false

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := runComments(cmd, nil); err != nil {
		fmt.Fprintln(os.Stderr, "sbnn comments failed:", err)
		os.Exit(3)
	}
	os.Exit(0)
}
