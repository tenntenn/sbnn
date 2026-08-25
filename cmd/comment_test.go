package cmd

import (
	"strings"
	"testing"
)

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
