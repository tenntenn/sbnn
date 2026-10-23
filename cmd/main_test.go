package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain keeps every test in this package away from the user's own sbnn.
//
// The commands resolve their server through the server map in the state
// directory, and a test that reaches one without pointing HOME somewhere else
// first would read - or, for a command that starts a server, write - the map
// of the machine it runs on, and hand a port next to the user's real server to
// a test server. So the state and cache directories are a scratch directory
// for the whole package, and the ports handed out start at one nothing was
// listening on a moment ago rather than at 6280.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "sbnn-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)

	// The tests that build sbnn run go, which finds its caches from HOME
	// too. Pinning them where they already are keeps those builds warm.
	if out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH").Output(); err == nil {
		vals := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, k := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
			if i < len(vals) && os.Getenv(k) == "" && vals[i] != "" {
				os.Setenv(k, vals[i])
			}
		}
	}
	for _, k := range []string{"HOME", "USERPROFILE", "AppData", "LocalAppData"} {
		os.Setenv(k, dir)
	}
	os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	// A test that runs a command through rootCmd would otherwise start the
	// check for a newer release whenever its stderr is a terminal, and no
	// test may reach GitHub.
	os.Setenv(NoUpdateCheckEnv, "1")

	if l, err := net.Listen("tcp", "localhost:0"); err == nil {
		basePort = l.Addr().(*net.TCPAddr).Port
		l.Close()
	}
	return m.Run()
}
