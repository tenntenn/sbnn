package cmd

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		asJSON  bool
		want    string
		wantErr bool
	}{
		{name: "prompt", format: "prompt", want: "prompt"},
		{name: "markdown", format: "markdown", want: "markdown"},
		{name: "json", format: "json", want: "json"},
		{name: "json shorthand over the default", format: "prompt", asJSON: true, want: "json"},
		{name: "json shorthand wins", format: "markdown", asJSON: true, want: "json"},
		{name: "unknown", format: "bogus", wantErr: true},
		{name: "empty", format: "", wantErr: true},
		{name: "wrong case", format: "JSON", wantErr: true},
		// A format nobody would read is still a typo worth reporting.
		{name: "unknown under the json shorthand", format: "bogus", asJSON: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveFormat(tt.format, tt.asJSON)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveFormat(%q, %v) = %q, want an error", tt.format, tt.asJSON, got)
				}
				if !strings.Contains(err.Error(), tt.format) {
					t.Errorf("error does not name the format: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFormat(%q, %v): %v", tt.format, tt.asJSON, err)
			}
			if got != tt.want {
				t.Errorf("resolveFormat(%q, %v) = %q, want %q", tt.format, tt.asJSON, got, tt.want)
			}
		})
	}
}

// closedPort returns a port nothing is listening on, so a command that
// contacts the server fails in a way that is easy to tell apart from a
// command that never got that far.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// "sbnn wait" blocks until the review is submitted, so a bad --format found
// on the way out costs the whole wait and the review is over by the time
// anything is printed. The format has to be rejected before the server is
// touched at all, which is what the closed port here stands in for: reaching
// the server would give "no sbnn server found" instead.
func TestWaitRejectsBadFormatBeforeTouchingTheServer(t *testing.T) {
	tests := []struct {
		name   string
		format string
		asJSON bool
		quiet  bool
	}{
		{name: "plain", format: "bogus"},
		{name: "with the json shorthand", format: "bogus", asJSON: true},
		{name: "quiet", format: "bogus", quiet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(TargetEnv, "")
			withWaitFlags(t, tt.format, tt.asJSON, tt.quiet, closedPort(t))

			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			err := runWait(cmd, nil)
			if err == nil {
				t.Fatal("runWait succeeded, want an error about the format")
			}
			if !strings.Contains(err.Error(), "unknown format") {
				t.Fatalf("runWait failed with %v, want the format rejected before the server is contacted", err)
			}
		})
	}
}

// A format sbnn can print gets as far as the server, which is how we know
// the check above rejects the format and not everything else.
func TestWaitAcceptsAGoodFormat(t *testing.T) {
	t.Setenv(TargetEnv, "")
	withWaitFlags(t, "markdown", false, false, closedPort(t))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runWait(cmd, nil)
	if err == nil {
		t.Fatal("runWait succeeded without a server, want a connection error")
	}
	if !strings.Contains(err.Error(), "no sbnn server found") {
		t.Fatalf("runWait failed with %v, want it to have reached the server", err)
	}
}

// withWaitFlags sets the package-level flag variables the wait command
// reads and puts them back afterwards.
func withWaitFlags(t *testing.T, format string, asJSON, quiet bool, p int) {
	t.Helper()
	oldFormat, oldJSON, oldQuiet, oldPort, oldBind, oldTarget := waitFormat, waitJSON, waitQuiet, port, bind, target
	t.Cleanup(func() {
		waitFormat, waitJSON, waitQuiet, port, bind, target = oldFormat, oldJSON, oldQuiet, oldPort, oldBind, oldTarget
	})
	waitFormat, waitJSON, waitQuiet, port, bind, target = format, asJSON, quiet, p, "localhost", ""
}
