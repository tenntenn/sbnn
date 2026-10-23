package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/repository"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/internal/servermap"
)

// basePort is the first port handed to a repository's server; the ones after
// it are taken in turn. It is a variable so that a test can keep clear of the
// range a real sbnn on the machine is using.
var basePort = DefaultPort

// portFlagHelp is the help of every --port flag.
const portFlagHelp = "Server port (default: the port of this repository's server; sbnn --status --all lists them)"

// repositoryFlag is the hidden --repository a background server is started
// with: the invocation that spawns it has already worked out which
// repository it is for, and the server's own working directory may be
// another worktree of it, or gone by the time anyone asks.
var repositoryFlag string

// serverRef is the server an invocation talks to.
type serverRef struct {
	// repo is the repository the invocation runs in, the key of its server.
	repo string
	// port is the port of that server, or 0 when the repository has never
	// had one and the command does not start one.
	port int
	// explicit is set when --port chose the port rather than the map.
	explicit bool
}

// currentRepository is the repository the working directory belongs to.
func currentRepository() string {
	return repository.Find(workingDir())
}

// portGiven reports whether --port chose the server. A command that has no
// --port flag at all - one a test builds to call a run function directly -
// counts as having been given one: whoever built it set port themselves.
func portGiven(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup("port")
	return f == nil || f.Changed
}

// resolveServer works out the server cmd talks to and sets port to it.
//
// --port wins when it is given. Otherwise the port is the one the server map
// keeps for the repository the command runs in, so that every invocation in
// a repository, and in every worktree of it, reaches the same server and no
// other. A repository that has no port yet is given one only when assign is
// set, which is for the commands that start a server: asking about a server
// that was never started must not leave an entry behind.
func resolveServer(cmd *cobra.Command, assign bool) (serverRef, error) {
	ref := serverRef{repo: currentRepository()}
	if f := cmd.Flags().Lookup("repository"); f != nil && f.Changed {
		ref.repo = repositoryFlag
	}
	if portGiven(cmd) {
		if port <= 0 || port > servermap.MaxPort {
			return ref, fmt.Errorf("--port %d is not a port: use 1-%d", port, servermap.MaxPort)
		}
		ref.port, ref.explicit = port, true
		return ref, nil
	}
	mapPath, err := paths.ServersFile()
	if err != nil {
		return ref, err
	}
	if !assign {
		m, err := servermap.Load(mapPath)
		if err != nil {
			return ref, err
		}
		ref.port, _ = m.Port(ref.repo)
		port = ref.port
		return ref, nil
	}
	p, err := assignPort(mapPath, ref.repo, os.Stderr)
	if err != nil {
		return ref, err
	}
	ref.port, port = p, p
	return ref, nil
}

// assignPort returns the port of repo's server, giving it a new one when it
// has none. A port is new when nothing listens on it and no session file is
// kept for it: a session file outside the map is a server from before the
// map, and taking its port would load its reviews into this repository's
// server.
//
// A server from before the map belongs to no repository, and is never taken
// for this repository's - it may hold the reviews of several. It is left
// running where it is, and w is told once, the first time a repository
// passes it by, how to reach it.
func assignPort(mapPath, repo string, w io.Writer) (int, error) {
	stateDir := filepath.Dir(mapPath)
	var older []int
	p, err := servermap.Assign(mapPath, repo, basePort, func(p int) bool {
		if _, err := os.Stat(sessionPath(stateDir, p)); err == nil {
			return false
		}
		a := net.JoinHostPort(bind, strconv.Itoa(p))
		if checkAddrFree(a) == nil {
			return true
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if st, err := client.New(a, time.Second).Status(ctx); err == nil && st.Repository == nil {
			older = append(older, p)
		}
		return false
	})
	if err != nil || len(older) == 0 {
		return p, err
	}
	var tell []int
	err = servermap.Update(mapPath, func(m *servermap.Map) error {
		for _, o := range older {
			if !slices.Contains(m.Told, o) {
				m.Told = append(m.Told, o)
				tell = append(tell, o)
			}
		}
		return nil
	})
	for _, o := range tell {
		fmt.Fprintf(w, "sbnn: the server on port %d was started by an older sbnn and belongs to no repository, "+
			"so %s gets a server of its own on port %d. The reviews already on port %d stay there: "+
			"pass -p %d to reach them (sbnn --status -p %d).\n",
			o, repository.Describe(repo), p, o, o, o)
	}
	return p, err
}

// sessionPath is the session file of the server on port p.
func sessionPath(stateDir string, p int) string {
	return filepath.Join(stateDir, fmt.Sprintf("session-%d.json", p))
}

// check refuses a server that belongs to a repository other than the one the
// invocation runs in. Talking to it would put this repository's diff or
// comments into another repository's reviews, which is the mixing a server
// per repository exists to end.
//
// A server that names no repository was started by an older sbnn. When the
// map keeps its port for this repository it is this repository's server
// from before the upgrade; when --port picked it, the user asked for it by
// number. Either way it is used.
func (r serverRef) check(st *server.Status) error {
	if st.Repository == nil || *st.Repository == r.repo {
		return nil
	}
	if r.explicit {
		return fmt.Errorf("the sbnn server on port %d belongs to %s, and this directory to %s: "+
			"run sbnn from there, or leave --port out to use the server of this repository",
			r.port, repository.Describe(*st.Repository), repository.Describe(r.repo))
	}
	return fmt.Errorf("port %d, which sbnn keeps for %s, is taken by the server of %s "+
		"(started there with --port?): stop it with sbnn --shutdown -p %d",
		r.port, repository.Describe(r.repo), repository.Describe(*st.Repository), r.port)
}

// checkSpawn refuses to start this repository's server on a port the map
// keeps for another repository. Only --port can ask for that; the server it
// started would answer the invocations of the repository the port is kept
// for, and be refused by every one of them.
func (r serverRef) checkSpawn() error {
	if !r.explicit {
		return nil
	}
	mapPath, err := paths.ServersFile()
	if err != nil {
		return err
	}
	m, err := servermap.Load(mapPath)
	if err != nil {
		return err
	}
	owner, ok := m.Owner(r.port)
	if !ok || owner == r.repo {
		return nil
	}
	hint := "leave --port out to start the server of this repository on its own port"
	if own, ok := m.Port(r.repo); ok {
		hint = fmt.Sprintf("this repository's server is on port %d", own)
	}
	return fmt.Errorf("port %d is kept for the server of %s, not of %s: %s",
		r.port, repository.Describe(owner), repository.Describe(r.repo), hint)
}

// errNoServer says that the server a command needed is not there, in the
// terms the user chose it in.
func (r serverRef) errNoServer(addr string) error {
	if r.explicit {
		return fmt.Errorf("no sbnn server found on %s", addr)
	}
	if r.port != 0 {
		return fmt.Errorf("no sbnn server found on %s, the port sbnn keeps for %s", addr, repository.Describe(r.repo))
	}
	return fmt.Errorf("no sbnn server found for %s: none was ever started here, and sending a diff starts one "+
		"(sbnn --status --all lists the servers there are)",
		repository.Describe(r.repo))
}

// connect finds the running server of the repository cmd runs in, refuses it
// when it turns out to belong to another repository, and returns a client for
// it with what it said about itself.
func connect(ctx context.Context, cmd *cobra.Command, timeout time.Duration) (*client.Client, *server.Status, error) {
	ref, err := resolveServer(cmd, false)
	if err != nil {
		return nil, nil, err
	}
	if ref.port == 0 {
		return nil, nil, ref.errNoServer("")
	}
	c := client.New(addr(), timeout)
	st, err := c.Status(ctx)
	if err != nil {
		return nil, nil, ref.errNoServer(c.Addr)
	}
	if err := ref.check(st); err != nil {
		return nil, nil, err
	}
	return c, st, nil
}

// serverListing is one server sbnn --status --all lists.
type serverListing struct {
	// Repository is the repository the server belongs to, absent for a
	// stopped server the map does not know - one an older sbnn left a
	// session file for - and for a running one that does not say.
	Repository *string `json:"repository,omitempty"`
	Port       int     `json:"port"`
	URL        string  `json:"url"`
	Running    bool    `json:"running"`
	PID        int     `json:"pid,omitempty"`
	Version    string  `json:"version,omitempty"`
	Reviews    int     `json:"reviews"`
	// Current marks the server of the repository sbnn was run in.
	Current bool `json:"current,omitempty"`
}

// listServers finds every server sbnn knows of: the ones the map keeps a
// port for, and the ones an older sbnn left a session file for, which is the
// only trace a server from before the map leaves.
func listServers(ctx context.Context, current string) ([]serverListing, error) {
	mapPath, err := paths.ServersFile()
	if err != nil {
		return nil, err
	}
	m, err := servermap.Load(mapPath)
	if err != nil {
		return nil, err
	}
	byPort := map[int]*serverListing{}
	for _, e := range m.Servers {
		repo := e.Repository
		byPort[e.Port] = &serverListing{Repository: &repo, Port: e.Port}
	}
	for p := range stateFiles(filepath.Dir(mapPath)) {
		if _, ok := byPort[p]; !ok {
			byPort[p] = &serverListing{Port: p}
		}
	}
	ports := make([]int, 0, len(byPort))
	for p := range byPort {
		ports = append(ports, p)
	}
	slices.Sort(ports)
	out := make([]serverListing, 0, len(ports))
	for _, p := range ports {
		l := byPort[p]
		c := client.New(net.JoinHostPort(bind, strconv.Itoa(p)), time.Second)
		l.URL = c.BaseURL()
		if st, err := c.Status(ctx); err == nil {
			l.Running, l.PID, l.Version, l.Reviews = true, st.PID, st.Version, len(st.Groups)
			// What the server says about itself outranks the map: it is
			// the one that would be talked to.
			if st.Repository != nil {
				l.Repository = st.Repository
			}
		}
		l.Current = l.Repository != nil && *l.Repository == current
		out = append(out, *l)
	}
	return out, nil
}

// runStatusAll lists every server, running or not.
func runStatusAll(ctx context.Context) error {
	current := currentRepository()
	servers, err := listServers(ctx, current)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(servers)
	}
	printServers(os.Stdout, servers)
	return nil
}

// printServers writes the listing of runStatusAll for a person.
func printServers(w io.Writer, servers []serverListing) {
	if len(servers) == 0 {
		fmt.Fprintln(w, "No sbnn server has been started yet.")
		return
	}
	for _, s := range servers {
		mark := " "
		if s.Current {
			mark = "*"
		}
		state := "stopped"
		if s.Running {
			state = fmt.Sprintf("running (pid %d, %d review(s))", s.PID, s.Reviews)
		}
		fmt.Fprintf(w, "%s %s  %s  %s\n", mark, s.URL, listingRepository(s), state)
	}
}

// listingRepository names the repository of a listed server.
func listingRepository(s serverListing) string {
	switch {
	case s.Repository != nil:
		return repository.Describe(*s.Repository)
	case s.Running:
		return "no repository (started by an older sbnn)"
	default:
		return "unknown (a session left by an older sbnn)"
	}
}

// statusRepository is the line sbnn --status prints about the repository a
// server belongs to.
func statusRepository(st *server.Status) string {
	if st.Repository == nil {
		return "none (started by an older sbnn: sbnn --restart starts it again from this one)"
	}
	return repository.Describe(*st.Repository)
}

// errRepositoryFlag is returned for a --repository given anywhere but to a
// server: it is how a background server is told its repository, not a way to
// talk to another repository's server.
var errRepositoryFlag = errors.New("--repository is only for the background server sbnn starts itself")
