package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// failingAdder stores comments until failAt (1-based), then refuses, the way
// the server does for an entry naming a path no diff carries.
type failingAdder struct {
	failAt int
	seen   int
}

func (a *failingAdder) AddComment(_ context.Context, _ string, req server.AddCommentRequest) (*model.Comment, error) {
	a.seen++
	if a.failAt > 0 && a.seen == a.failAt {
		return nil, fmt.Errorf("no diff carries %q", req.Path)
	}
	return &model.Comment{
		ID:        "c" + strconv.Itoa(a.seen),
		Path:      req.Path,
		StartLine: req.StartLine,
		EndLine:   req.EndLine,
	}, nil
}

func bulkRequests(paths ...string) []server.AddCommentRequest {
	requests := make([]server.AddCommentRequest, 0, len(paths))
	for i, p := range paths {
		requests = append(requests, server.AddCommentRequest{
			Path: p, StartLine: i + 1, EndLine: i + 1, Body: "x",
		})
	}
	return requests
}

// withJSONOut sets the package-level --json-output flag for one test.
func withJSONOut(t *testing.T, on bool) {
	t.Helper()
	was := commentJSONOut
	commentJSONOut = on
	t.Cleanup(func() { commentJSONOut = was })
}

func TestAddCommentsPartialFailureIsReported(t *testing.T) {
	withJSONOut(t, false)
	var out, errOut bytes.Buffer

	adder := &failingAdder{failAt: 3}
	err := addComments(context.Background(), adder, "default",
		bulkRequests("a.go", "b.go", "gone.go", "d.go"), &out, &errOut)
	if err == nil {
		t.Fatal("addComments succeeded, want the third comment to fail")
	}
	if !strings.Contains(err.Error(), "comment 3 of 4 (gone.go:3)") {
		t.Errorf("error is %q, want it to name which entry failed", err)
	}
	if !strings.Contains(err.Error(), "no diff carries") {
		t.Errorf("error is %q, want the server's reason kept", err)
	}
	if adder.seen != 3 {
		t.Errorf("the fourth comment was sent after the failure (%d sent)", adder.seen)
	}
	report := errOut.String()
	if !strings.Contains(report, "2 of 4 comments were stored") {
		t.Errorf("stderr is %q, want it to say how many are already on the server", report)
	}
	if !strings.Contains(report, "added twice") {
		t.Errorf("stderr is %q, want it to warn that re-running duplicates them", report)
	}
}

// The --json-output run is the one that lost everything: nothing is printed
// until the end, so a failure used to leave the caller with only the error.
func TestAddCommentsPartialFailurePrintsStoredJSON(t *testing.T) {
	withJSONOut(t, true)
	var out, errOut bytes.Buffer

	err := addComments(context.Background(), &failingAdder{failAt: 3}, "default",
		bulkRequests("a.go", "b.go", "gone.go"), &out, &errOut)
	if err == nil {
		t.Fatal("addComments succeeded, want the third comment to fail")
	}
	var stored []*model.Comment
	if err := json.Unmarshal(out.Bytes(), &stored); err != nil {
		t.Fatalf("stdout is not the JSON of what was stored (%q): %v", out.String(), err)
	}
	if len(stored) != 2 {
		t.Fatalf("stdout lists %d stored comments, want the 2 that were written", len(stored))
	}
	if stored[0].ID != "c1" || stored[1].ID != "c2" {
		t.Errorf("stdout lists %q and %q, want the IDs the server gave back", stored[0].ID, stored[1].ID)
	}
}

func TestAddCommentsFirstFailureKeepsTheErrorPlain(t *testing.T) {
	withJSONOut(t, false)
	var out, errOut bytes.Buffer

	err := addComments(context.Background(), &failingAdder{failAt: 1}, "default",
		bulkRequests("gone.go"), &out, &errOut)
	if err == nil {
		t.Fatal("addComments succeeded, want the only comment to fail")
	}
	if got := err.Error(); got != `no diff carries "gone.go"` {
		t.Errorf("error is %q, want the server's own message for a single comment", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr is %q, want nothing said when nothing was stored", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout is %q, want nothing printed when nothing was stored", out.String())
	}
}

func TestAddCommentsAllStored(t *testing.T) {
	withJSONOut(t, true)
	var out, errOut bytes.Buffer

	if err := addComments(context.Background(), &failingAdder{}, "default",
		bulkRequests("a.go", "b.go"), &out, &errOut); err != nil {
		t.Fatalf("addComments failed: %v", err)
	}
	var stored []*model.Comment
	if err := json.Unmarshal(out.Bytes(), &stored); err != nil {
		t.Fatalf("stdout is not JSON (%q): %v", out.String(), err)
	}
	if len(stored) != 2 {
		t.Errorf("stdout lists %d comments, want 2", len(stored))
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr is %q, want nothing next to the JSON", errOut.String())
	}
}
