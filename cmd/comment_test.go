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

func TestFlexLinesUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in         string
		wantStart  int
		wantEnd    int
		wantErr    string
		wantNoLine bool
	}{
		"string line":       {in: `"12"`, wantStart: 12, wantEnd: 12},
		"string range":      {in: `"12-18"`, wantStart: 12, wantEnd: 18},
		"integer":           {in: `12`, wantStart: 12, wantEnd: 12},
		"integral float":    {in: `12.0`, wantStart: 12, wantEnd: 12},
		"integral float 00": {in: `12.00`, wantStart: 12, wantEnd: 12},
		"exponent":          {in: `1.2e1`, wantStart: 12, wantEnd: 12},
		"negative zero":     {in: `-0.0`, wantStart: 0, wantEnd: 0},
		"null":              {in: `null`, wantNoLine: true},
		"empty string":      {in: `""`, wantNoLine: true},
		"fraction":          {in: `12.5`, wantErr: "whole number"},
		"tiny fraction":     {in: `12.0000001`, wantErr: "whole number"},
		"huge":              {in: `1e30`, wantErr: "out of range"},
		"not a number":      {in: `true`, wantErr: "line must be a number"},
		"bad string":        {in: `"twelve"`, wantErr: "not a line or a line range"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got flexLines
			err := json.Unmarshal([]byte(test.in), &got)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("unmarshalling %s = %+v, want an error mentioning %q", test.in, got, test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("unmarshalling %s failed with %q, want it to mention %q", test.in, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshalling %s failed: %v", test.in, err)
			}
			if test.wantNoLine {
				if got.Start != 0 || got.End != 0 {
					t.Fatalf("unmarshalling %s = %+v, want no line so that startLine can stand in", test.in, got)
				}
				return
			}
			if got.Start != test.wantStart || got.End != test.wantEnd {
				t.Errorf("unmarshalling %s = %d-%d, want %d-%d", test.in, got.Start, got.End, test.wantStart, test.wantEnd)
			}
		})
	}
}

// TestReadBulkCommentsFloatLine is the shape a generator that does arithmetic
// produces: jq's {line: (.start + 1)} prints 12.0, not 12.
func TestReadBulkCommentsFloatLine(t *testing.T) {
	in := `[{"path": "main.go", "line": 12.0, "body": "x"},
	        {"path": "README.md", "line": 3, "endLine": 4, "body": "y"}]`
	got, err := readBulkComments(strings.NewReader(in))
	if err != nil {
		t.Fatalf("readBulkComments failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("readBulkComments returned %d comments, want 2", len(got))
	}
	if got[0].StartLine != 12 || got[0].EndLine != 12 {
		t.Errorf("line 12.0 became %d-%d, want 12-12", got[0].StartLine, got[0].EndLine)
	}
}

func TestReadBulkCommentsFractionalLine(t *testing.T) {
	_, err := readBulkComments(strings.NewReader(`[{"path": "main.go", "line": 12.5, "body": "x"}]`))
	if err == nil {
		t.Fatal("readBulkComments accepted line 12.5, want an error")
	}
	if !strings.Contains(err.Error(), "whole number") {
		t.Errorf("readBulkComments failed with %q, want it to say the line must be a whole number", err)
	}
}

func TestNormalizeSide(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      string
		want    string
		wantErr bool
	}{
		"empty":         {in: "", want: "new"},
		"new":           {in: "new", want: "new"},
		"old":           {in: "old", want: "old"},
		"capitalized":   {in: "New", want: "new"},
		"upper":         {in: "OLD", want: "old"},
		"mixed":         {in: "nEw", want: "new"},
		"padded":        {in: " old ", want: "old"},
		"padded empty":  {in: "   ", want: "new"},
		"padded tab":    {in: "\tNEW\n", want: "new"},
		"unknown":       {in: "left", wantErr: true},
		"unknown upper": {in: "BOTH", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeSide(test.in)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeSide(%q) = %q, want an error", test.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSide(%q) returned an error: %v", test.in, err)
			}
			if got != test.want {
				t.Errorf("normalizeSide(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestParseLineSpec(t *testing.T) {
	t.Parallel()

	const tooBig = "99999999999999999999" // one digit past an int64

	tests := map[string]struct {
		spec      string
		wantPath  string
		wantStart int
		wantEnd   int
		wantErr   string
	}{
		"single line":   {spec: "main.go:42", wantPath: "main.go", wantStart: 42, wantEnd: 42},
		"range":         {spec: "README.md:12-18", wantPath: "README.md", wantStart: 12, wantEnd: 18},
		"path in dirs":  {spec: "internal/server/server.go:120", wantPath: "internal/server/server.go", wantStart: 120, wantEnd: 120},
		"colon in path": {spec: "C:/tmp/x.go:7", wantPath: "C:/tmp/x.go", wantStart: 7, wantEnd: 7},
		"no line":       {spec: "main.go", wantErr: "say path:line"},
		"zero":          {spec: "main.go:0", wantErr: "not a line range"},
		"backwards":     {spec: "main.go:18-12", wantErr: "not a line range"},
		"not digits":    {spec: "main.go:forty", wantErr: "not a line or a line range"},

		// An overflowing number used to survive as MaxInt64 and be sent to the
		// server, which anchors the comment to a line no file will ever have.
		"start overflows": {spec: "main.go:" + tooBig, wantErr: "out of range"},
		"end overflows":   {spec: "main.go:12-" + tooBig, wantErr: "out of range"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path, start, end, err := parseLineSpec(test.spec)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("parseLineSpec(%q) = %q %d-%d, want an error mentioning %q",
						test.spec, path, start, end, test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parseLineSpec(%q) failed with %q, want it to mention %q", test.spec, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLineSpec(%q) failed: %v", test.spec, err)
			}
			if path != test.wantPath || start != test.wantStart || end != test.wantEnd {
				t.Errorf("parseLineSpec(%q) = %q %d-%d, want %q %d-%d",
					test.spec, path, start, end, test.wantPath, test.wantStart, test.wantEnd)
			}
		})
	}
}

// TestReadBulkCommentsOverflowingLine covers the same hole reached through
// --json, whose string values go through parseLines too.
func TestReadBulkCommentsOverflowingLine(t *testing.T) {
	in := `[{"path": "main.go", "line": "99999999999999999999", "body": "x"}]`
	got, err := readBulkComments(strings.NewReader(in))
	if err == nil {
		t.Fatalf("readBulkComments accepted an overflowing line, giving %+v", got)
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("readBulkComments failed with %q, want it to say the line is out of range", err)
	}
}
