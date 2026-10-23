package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tenntenn/sbnn/internal/repository"
	"github.com/tenntenn/sbnn/internal/servermap"
)

func TestForgetServers(t *testing.T) {
	tests := []struct {
		name string
		// repoGone removes the repository's directory, session writes a
		// session file for its port, removed counts that file as one the
		// prune removes, and running answers on the port.
		repoGone, session, removed, running bool
		outside                             bool
		want                                bool
	}{
		{name: "a repository that is gone and left nothing", repoGone: true, want: true},
		{name: "a repository that still exists", want: false},
		{name: "a session file that stays", repoGone: true, session: true, want: false},
		{name: "a session file the prune removes", repoGone: true, session: true, removed: true, want: true},
		{name: "a server still running", repoGone: true, running: true, want: false},
		{name: "outside any repository has no directory to lose", outside: true, want: false},
	}
	for _, tt := range tests {
		for _, dryRun := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/dry run %v", tt.name, dryRun), func(t *testing.T) {
				mapPath := withServerState(t)
				stateDir := filepath.Dir(mapPath)
				repo := fakeRepository(t)
				if tt.outside {
					repo = repository.None
				}
				if tt.repoGone {
					if err := os.RemoveAll(repo); err != nil {
						t.Fatal(err)
					}
				}
				const p = 7001
				writeMap(t, mapPath, servermap.Entry{Repository: repo, Port: p})
				var removed []pruneFile
				if tt.session {
					path := sessionPath(stateDir, p)
					touch(t, path)
					if tt.removed {
						removed = append(removed, pruneFile{Kind: pruneSession, Path: path, Port: p})
						if !dryRun {
							// A prune that is not a dry run has removed
							// it by the time the map is looked at.
							os.Remove(path)
						}
					}
				}
				ports := map[int]fakePort{}
				if tt.running {
					ports[p] = fakePort{}
				}
				got, err := forgetServers(stateDir, removed, fakePorts(ports), dryRun)
				if err != nil {
					t.Fatal(err)
				}
				if (len(got) == 1) != tt.want {
					t.Errorf("forgetServers = %+v, want forgotten: %v", got, tt.want)
				}
				m, err := servermap.Load(mapPath)
				if err != nil {
					t.Fatal(err)
				}
				_, kept := m.Port(repo)
				if wantKept := dryRun || !tt.want; kept != wantKept {
					t.Errorf("the map keeps the server: %v, want %v", kept, wantKept)
				}
			})
		}
	}
}
