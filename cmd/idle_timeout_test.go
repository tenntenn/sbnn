package cmd

// End-to-end cover for --idle-timeout.
//
// The timeout lives in the server, but the server that matters is the
// detached one the CLI spawns, and that one parses its own flags. An
// Options field alone therefore proves nothing: the flag has to survive the
// hand-off in spawnServer, or the background server - the only one that ever
// outlives anything - runs on the zero value and stays up forever, which is
// exactly the state this issue describes. So this test drives the real
// binary through the real spawn path rather than calling into the package.

import (
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// idleDiff is the smallest thing sbnn will accept as a review.
const idleDiff = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`

// buildSbnn builds the command under test and returns the binary's path.
func buildSbnn(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbnn")
	out, err := exec.Command("go", "build", "-o", bin, "github.com/tenntenn/sbnn").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// freePort returns a port nothing is listening on, by taking one and giving
// it back.
func freePort(t *testing.T) int {
	t.Helper()
	_, p, err := net.SplitHostPort(freeAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// serverUp reports whether an sbnn server is answering on port.
func serverUp(port int) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/_/api/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var st struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return false
	}
	return st.App == "sbnn"
}

// waitFor polls until want is reported or the deadline passes, and returns
// how long it took.
func waitFor(port int, want bool, timeout time.Duration) (time.Duration, bool) {
	start := time.Now()
	for time.Since(start) < timeout {
		if serverUp(port) == want {
			return time.Since(start), true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return time.Since(start), false
}

// A background server holding a review must stay up however long it is left
// alone, and must end itself once the review is closed and it holds nothing.
// Both halves run against the spawned binary, because the bug being fixed was
// entirely in the hand-off to it.
func TestBackgroundServerStopsOnceItHoldsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the sbnn binary")
	}
	bin := buildSbnn(t)
	state := t.TempDir()
	t.Setenv("HOME", state)
	t.Setenv("XDG_STATE_HOME", filepath.Join(state, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(state, "cache"))

	port := freePort(t)
	p := strconv.Itoa(port)
	const idle = 2 * time.Second

	sbnn := func(stdin string, args ...string) string {
		t.Helper()
		args = append([]string{"--port", p, "--no-open", "--history-file", HistoryOffWords[0],
			"--idle-timeout", idle.String()}, args...)
		c := exec.Command(bin, args...)
		c.Stdin = strings.NewReader(stdin)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("sbnn %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	// Starting the server is the first invocation adding a diff to it.
	sbnn(idleDiff)
	t.Cleanup(func() {
		if serverUp(port) {
			_ = exec.Command(bin, "--port", p, "--shutdown").Run()
		}
	})
	if _, ok := waitFor(port, true, 15*time.Second); !ok {
		t.Fatal("the server never came up")
	}

	// A review someone may come back to is never collected, however long the
	// server is left alone. Two and a half times the timeout is well past the
	// point where an unconditional timer would have fired.
	time.Sleep(5 * time.Second)
	if !serverUp(port) {
		t.Fatal("the server collected itself while a diff was still open")
	}

	// Closing the review leaves it holding nothing, and now it has to go.
	sbnn("", "--clear", "--all", "--yes")
	took, ok := waitFor(port, false, 30*time.Second)
	if !ok {
		t.Fatalf("the server was still up %s after the last review was closed", took)
	}
	t.Logf("the server ended itself %s after the review was closed (--idle-timeout %s)", took.Round(100*time.Millisecond), idle)
}

// The flag has to be on the command line of the spawned server: it is a
// different process, so a value only this one holds never reaches it.
func TestSpawnedServerArgsCarryTheIdleTimeout(t *testing.T) {
	args := backgroundServerArgs([]string{"--history-file", "off"})
	i := slices.Index(args, "--idle-timeout")
	if i < 0 {
		t.Fatalf("args = %v, want --idle-timeout among them", args)
	}
	if i+1 >= len(args) {
		t.Fatalf("args = %v, want a value after --idle-timeout", args)
	}
	got, err := time.ParseDuration(args[i+1])
	if err != nil {
		t.Fatalf("--idle-timeout %q: %v", args[i+1], err)
	}
	if got != idleTimeout {
		t.Errorf("--idle-timeout %s, want %s", got, idleTimeout)
	}
}
