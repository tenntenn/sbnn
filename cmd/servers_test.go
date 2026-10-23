package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/repository"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/internal/servermap"
)

// withServerState gives a test a state directory of its own, an empty server
// map in it, and the flags resolveServer reads put back afterwards. It
// returns the path of the map.
func withServerState(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	oldPort, oldBind, oldBase := port, bind, basePort
	t.Cleanup(func() { port, bind, basePort = oldPort, oldBind, oldBase })
	bind = "localhost"
	mapPath, err := paths.ServersFile()
	if err != nil {
		t.Fatal(err)
	}
	return mapPath
}

// fakeRepository makes a directory that sbnn takes for a repository - the
// .git directory is all it looks at - and returns its root as Find reports
// it.
func fakeRepository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakeWorktree adds a linked worktree to the repository at root and returns
// its directory.
func fakeWorktree(t *testing.T, root string) string {
	t.Helper()
	gitdir := filepath.Join(root, ".git", "worktrees", "wt")
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return wt
}

// outsideAnyRepository returns a directory in no repository.
func outsideAnyRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if got := repository.Find(dir); got != repository.None {
		t.Skipf("the temporary directory %s is inside the repository %s", dir, got)
	}
	return dir
}

// portCommand is a command with a --port flag, given when explicit is not 0.
func portCommand(t *testing.T, explicit int) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().IntVarP(&port, "port", "p", 0, portFlagHelp)
	if explicit != 0 {
		if err := cmd.Flags().Set("port", strconv.Itoa(explicit)); err != nil {
			t.Fatal(err)
		}
	}
	return cmd
}

func writeMap(t *testing.T, mapPath string, entries ...servermap.Entry) {
	t.Helper()
	err := servermap.Update(mapPath, func(m *servermap.Map) error {
		m.Servers = append(m.Servers, entries...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// startServer runs a server on a free port, belonging to repo, or to no
// repository at all - a server from an older sbnn - when repo is nil.
func startServer(t *testing.T, repo *string) int {
	t.Helper()
	p := freePort(t)
	srv, err := server.New(server.Options{
		Port: p, Version: "test", Repository: repo,
		Mo: mo.New("mo-not-installed-for-tests", 0, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	if _, ok := waitFor(p, true, 5*time.Second); !ok {
		t.Fatal("the server never came up")
	}
	return p
}

func TestResolveServer(t *testing.T) {
	tests := []struct {
		name string
		// where is "repo", "worktree" or "outside".
		where string
		// mapped is the port the map keeps for the repository, 0 for none.
		mapped   int
		explicit int
		assign   bool
		// wantPort -1 means a port the map did not have before, handed
		// out from basePort.
		wantPort     int
		wantExplicit bool
		wantMapped   bool
	}{
		{name: "--port wins over the map", where: "repo", mapped: 7001, explicit: 7002, wantPort: 7002, wantExplicit: true, wantMapped: true},
		{name: "--port is not remembered", where: "repo", explicit: 7002, assign: true, wantPort: 7002, wantExplicit: true},
		{name: "the map gives the port", where: "repo", mapped: 7001, wantPort: 7001, wantMapped: true},
		{name: "a worktree gets the port of its repository", where: "worktree", mapped: 7001, wantPort: 7001, wantMapped: true},
		{name: "asking leaves no entry behind", where: "repo", wantPort: 0},
		{name: "starting gives the repository a port", where: "repo", assign: true, wantPort: -1, wantMapped: true},
		{name: "outside any repository has a port of its own", where: "outside", mapped: 7003, wantPort: 7003, wantMapped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapPath := withServerState(t)
			basePort = freePort(t)
			root := fakeRepository(t)
			dir, key := root, root
			switch tt.where {
			case "worktree":
				dir = fakeWorktree(t, root)
			case "outside":
				dir, key = outsideAnyRepository(t), repository.None
			}
			if tt.mapped != 0 {
				writeMap(t, mapPath, servermap.Entry{Repository: key, Port: tt.mapped})
			}
			t.Chdir(dir)

			ref, err := resolveServer(portCommand(t, tt.explicit), tt.assign)
			if err != nil {
				t.Fatal(err)
			}
			want := tt.wantPort
			if want == -1 {
				want = ref.port
				if want < basePort {
					t.Errorf("port %d, want one from %d up", want, basePort)
				}
			}
			if ref.repo != key || ref.port != want || ref.explicit != tt.wantExplicit {
				t.Errorf("resolveServer = %+v, want repo %q, port %d, explicit %v", ref, key, want, tt.wantExplicit)
			}
			if port != ref.port {
				t.Errorf("port = %d, want it set to %d", port, ref.port)
			}
			m, err := servermap.Load(mapPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := m.Port(key); ok != tt.wantMapped {
				t.Errorf("the map has the repository: %v, want %v (%+v)", ok, tt.wantMapped, m.Servers)
			}
		})
	}
}

func TestResolveServerRefusesANonPort(t *testing.T) {
	withServerState(t)
	for _, p := range []int{-1, 70000} {
		if _, err := resolveServer(portCommand(t, p), false); err == nil {
			t.Errorf("--port %d was accepted", p)
		}
	}
}

func TestServerRefCheck(t *testing.T) {
	a, b, none := "/repo/a", "/repo/b", repository.None
	tests := []struct {
		name     string
		ref      serverRef
		server   *string
		wantErr  []string
		wantNone bool
	}{
		{name: "its own server", ref: serverRef{repo: a, port: 7001}, server: &a},
		{name: "its own server by --port", ref: serverRef{repo: a, port: 7001, explicit: true}, server: &a},
		{name: "outside any repository", ref: serverRef{repo: none, port: 7001}, server: &none},
		{name: "an older server on the port the map keeps", ref: serverRef{repo: a, port: 7001}, server: nil},
		{name: "an older server asked for by --port", ref: serverRef{repo: a, port: 7001, explicit: true}, server: nil},
		{
			name:    "another repository's server by --port",
			ref:     serverRef{repo: a, port: 7001, explicit: true},
			server:  &b,
			wantErr: []string{"port 7001 belongs to /repo/b", "this directory to /repo/a", "leave --port out"},
		},
		{
			name:    "another repository's server on the port the map keeps",
			ref:     serverRef{repo: a, port: 7001},
			server:  &b,
			wantErr: []string{"port 7001, which sbnn keeps for /repo/a", "server of /repo/b", "sbnn --shutdown -p 7001"},
		},
		{
			name:    "a repository's server for a diff from outside any",
			ref:     serverRef{repo: none, port: 7001, explicit: true},
			server:  &a,
			wantErr: []string{"belongs to /repo/a", "outside any repository"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.check(&server.Status{Repository: tt.server})
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Errorf("check = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("check = nil, want a refusal")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("check = %v, want it to say %q", err, want)
				}
			}
		})
	}
}

func TestCheckSpawn(t *testing.T) {
	tests := []struct {
		name    string
		entries []servermap.Entry
		ref     serverRef
		wantErr string
	}{
		{name: "a port from the map", entries: []servermap.Entry{{Repository: "/a", Port: 7001}}, ref: serverRef{repo: "/a", port: 7001}},
		{name: "--port on a port nobody has", ref: serverRef{repo: "/a", port: 7005, explicit: true}},
		{name: "--port on its own port", entries: []servermap.Entry{{Repository: "/a", Port: 7001}}, ref: serverRef{repo: "/a", port: 7001, explicit: true}},
		{
			name:    "--port on another repository's port",
			entries: []servermap.Entry{{Repository: "/a", Port: 7001}, {Repository: "/b", Port: 7002}},
			ref:     serverRef{repo: "/a", port: 7002, explicit: true},
			wantErr: "port 7002 is kept for the server of /b, not of /a: this repository's server is on port 7001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapPath := withServerState(t)
			writeMap(t, mapPath, tt.entries...)
			err := tt.ref.checkSpawn()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("checkSpawn = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("checkSpawn = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// connect is what every command that needs a running server goes through, so
// the refusal is checked against real servers here.
func TestConnect(t *testing.T) {
	a, b := fakeRepository(t), fakeRepository(t)
	ownPort := startServer(t, &a)
	otherPort := startServer(t, &b)
	olderPort := startServer(t, nil)

	tests := []struct {
		name     string
		mapped   int
		explicit int
		wantErr  string
	}{
		{name: "the server the map keeps", mapped: ownPort},
		{name: "its own server by --port", explicit: ownPort},
		{name: "an older server by --port", explicit: olderPort},
		{name: "an older server the map keeps for it", mapped: olderPort},
		{name: "another repository's server by --port", explicit: otherPort, wantErr: "belongs to " + b},
		{name: "another repository's server on its port", mapped: otherPort, wantErr: "is taken by the server of " + b},
		{name: "no server was ever started", wantErr: "none was ever started here"},
		{name: "the server the map keeps is not running", mapped: freePort(t), wantErr: "the port sbnn keeps for " + a},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapPath := withServerState(t)
			if tt.mapped != 0 {
				writeMap(t, mapPath, servermap.Entry{Repository: a, Port: tt.mapped})
			}
			t.Chdir(a)
			_, st, err := connect(context.Background(), portCommand(t, tt.explicit), time.Second)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("connect = %v", err)
				}
				if st == nil {
					t.Fatal("connect returned no status")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("connect = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// A server from before the map is passed by, not taken over, and the user
// is told once how to reach it.
func TestAssignPortPassesAnOlderServerBy(t *testing.T) {
	mapPath := withServerState(t)
	older := startServer(t, nil)
	basePort = older
	// A session file outside the map is a server from before the map too,
	// stopped: its port is not handed out either.
	if err := os.WriteFile(sessionPath(filepath.Dir(mapPath), older+1), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var told bytes.Buffer
	p, err := assignPort(mapPath, "/repo/a", &told)
	if err != nil {
		t.Fatal(err)
	}
	if p == older || p == older+1 {
		t.Errorf("assignPort = %d, the port of a server from before the map", p)
	}
	for _, want := range []string{"port " + strconv.Itoa(older) + " was started by an older sbnn", "-p " + strconv.Itoa(older)} {
		if !strings.Contains(told.String(), want) {
			t.Errorf("told %q, want it to say %q", told.String(), want)
		}
	}

	told.Reset()
	if _, err := assignPort(mapPath, "/repo/b", &told); err != nil {
		t.Fatal(err)
	}
	if told.Len() != 0 {
		t.Errorf("told again: %q", told.String())
	}
}

func TestPrintServers(t *testing.T) {
	a, none := "/repo/a", repository.None
	var b bytes.Buffer
	printServers(&b, []serverListing{
		{Repository: &a, Port: 7001, URL: "http://localhost:7001", Running: true, PID: 42, Reviews: 2, Current: true},
		{Repository: &none, Port: 7002, URL: "http://localhost:7002"},
		{Port: 7003, URL: "http://localhost:7003", Running: true, PID: 43},
		{Port: 7004, URL: "http://localhost:7004"},
	})
	want := `* http://localhost:7001  /repo/a  running (pid 42, 2 review(s))
  http://localhost:7002  the diffs from outside any repository  stopped
  http://localhost:7003  no repository (started by an older sbnn)  running (pid 43, 0 review(s))
  http://localhost:7004  unknown (a session left by an older sbnn)  stopped
`
	if b.String() != want {
		t.Errorf("printServers =\n%s\nwant\n%s", b.String(), want)
	}
}

// TestServerPerRepository runs the built binary the way a user does, in two
// repositories, a worktree of one of them and a directory in neither, and
// checks that each lands on a server of its own and that --port cannot be
// used to cross over.
func TestServerPerRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the sbnn binary")
	}
	bin := buildSbnn(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	mapPath, err := paths.ServersFile()
	if err != nil {
		t.Fatal(err)
	}

	a, b := fakeRepository(t), fakeRepository(t)
	wt := fakeWorktree(t, a)
	outside := outsideAnyRepository(t)
	// The binary hands out ports from 6280, next to whatever sbnn the
	// machine runs, so the map is written first with ports of the test's
	// choosing: that the binary honours it is part of what is tested.
	ports := map[string]int{a: freePort(t), b: freePort(t), repository.None: freePort(t)}
	for repo, p := range ports {
		writeMap(t, mapPath, servermap.Entry{Repository: repo, Port: p})
	}
	t.Cleanup(func() {
		for _, p := range ports {
			if serverUp(p) {
				_ = exec.Command(bin, "--port", strconv.Itoa(p), "--shutdown").Run()
			}
		}
	})

	sbnn := func(dir, stdin string, args ...string) (string, error) {
		t.Helper()
		c := exec.Command(bin, args...)
		c.Dir = dir
		c.Stdin = strings.NewReader(stdin)
		out, err := c.CombinedOutput()
		return string(out), err
	}
	send := func(dir string, args ...string) string {
		t.Helper()
		args = append([]string{"--no-open", "--history-file", HistoryOffWords[0], "--idle-timeout", "5m"}, args...)
		out, err := sbnn(dir, idleDiff, args...)
		if err != nil {
			t.Fatalf("sbnn %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
		}
		return out
	}
	status := func(dir string, args ...string) server.Status {
		t.Helper()
		out, err := sbnn(dir, "", append([]string{"--status", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("sbnn --status in %s: %v\n%s", dir, err, out)
		}
		var st server.Status
		if err := json.Unmarshal([]byte(out), &st); err != nil {
			t.Fatalf("sbnn --status: %v\n%s", err, out)
		}
		return st
	}

	for dir, repo := range map[string]string{a: a, b: b, outside: repository.None} {
		out := send(dir)
		if want := ":" + strconv.Itoa(ports[repo]) + "/"; !strings.Contains(out, want) {
			t.Errorf("sending in %s printed %q, want a URL on %s", dir, out, want)
		}
		st := status(dir)
		if st.Repository == nil || *st.Repository != repo {
			t.Errorf("the server of %s says it belongs to %v", dir, st.Repository)
		}
		if len(st.Groups) != 1 || st.Groups[0].Diffs != 1 {
			t.Errorf("the server of %s holds %+v, want one diff in one review", dir, st.Groups)
		}
	}

	// A worktree joins the server of its repository, in a review of its own.
	if out := send(wt, "--target", "wt"); !strings.Contains(out, ":"+strconv.Itoa(ports[a])+"/wt") {
		t.Errorf("sending in a worktree printed %q, want the server of its repository", out)
	}
	if st := status(wt); st.Repository == nil || *st.Repository != a || len(st.Groups) != 2 {
		t.Errorf("the worktree's server is %v with %+v, want %s with two reviews", st.Repository, st.Groups, a)
	}

	// --port does not cross over to another repository's server.
	out, err := sbnn(b, "", "comments", "--port", strconv.Itoa(ports[a]))
	if err == nil || !strings.Contains(out, "belongs to "+a) || !strings.Contains(out, "this directory to "+b) {
		t.Errorf("sbnn comments -p <a's port> in b: %v\n%s", err, out)
	}
	out, err = sbnn(b, idleDiff, "--no-open", "--port", strconv.Itoa(ports[a]))
	if err == nil || !strings.Contains(out, "belongs to "+a) {
		t.Errorf("sending to a's port from b: %v\n%s", err, out)
	}

	// Every server is listed, and the one of the directory is marked.
	out, err = sbnn(a, "", "--status", "--all", "--json")
	if err != nil {
		t.Fatalf("sbnn --status --all: %v\n%s", err, out)
	}
	var listed []serverListing
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("sbnn --status --all: %v\n%s", err, out)
	}
	if len(listed) != 3 {
		t.Fatalf("listed %d servers, want 3: %s", len(listed), out)
	}
	for _, l := range listed {
		if !l.Running || l.Repository == nil || ports[*l.Repository] != l.Port || l.Current != (*l.Repository == a) {
			t.Errorf("listed %+v", l)
		}
	}

	// A restart run from anywhere keeps the repository of the server it
	// restarts.
	if out, err := sbnn(outside, "", "--restart", "--port", strconv.Itoa(ports[b])); err != nil {
		t.Fatalf("sbnn --restart -p <b's port>: %v\n%s", err, out)
	}
	if st := status(b); st.Repository == nil || *st.Repository != b || len(st.Groups) != 1 {
		t.Errorf("after a restart the server of b is %v with %+v", st.Repository, st.Groups)
	}
}
