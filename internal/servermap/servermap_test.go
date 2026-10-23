package servermap

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestAssign(t *testing.T) {
	tests := []struct {
		name string
		// before is the map on disk, nil for no file at all.
		before []Entry
		repo   string
		// busy are the ports usable turns down.
		busy []int
		want int
	}{
		{name: "the first repository gets the base port", repo: "/a", want: 6280},
		{
			name:   "a repository keeps the port it has",
			before: []Entry{{"/a", 6280}, {"/b", 6281}},
			repo:   "/b",
			busy:   []int{6281},
			want:   6281,
		},
		{
			name:   "a new repository skips the ports others are kept on",
			before: []Entry{{"/a", 6280}, {"/b", 6281}},
			repo:   "/c",
			want:   6282,
		},
		{
			name: "a new repository skips ports that are in use",
			repo: "/a",
			busy: []int{6280, 6281},
			want: 6282,
		},
		{
			name:   "a gap left by a forgotten repository is reused",
			before: []Entry{{"/a", 6280}, {"/c", 6282}},
			repo:   "/b",
			want:   6281,
		},
		{
			name:   "outside any repository is a key like any other",
			before: []Entry{{"/a", 6280}},
			repo:   "",
			want:   6281,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "servers.json")
			if tt.before != nil {
				if err := Update(path, func(m *Map) error { m.Servers = tt.before; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			usable := func(p int) bool { return !slices.Contains(tt.busy, p) }
			got, err := Assign(path, tt.repo, 6280, usable)
			if err != nil {
				t.Fatalf("Assign = %v", err)
			}
			if got != tt.want {
				t.Errorf("Assign = %d, want %d", got, tt.want)
			}
			// The answer is remembered: asked again with every port
			// refused, the repository still gets the same one.
			again, err := Assign(path, tt.repo, 6280, func(int) bool { return false })
			if err != nil || again != got {
				t.Errorf("Assign again = %d, %v, want %d", again, err, got)
			}
			m, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if p, ok := m.Port(tt.repo); !ok || p != tt.want {
				t.Errorf("Load().Port(%q) = %d, %v, want %d", tt.repo, p, ok, tt.want)
			}
		})
	}
}

// TestAssignConcurrent runs many invocations at once, three for each
// repository: each repository has to come out with one port, and no two
// repositories with the same one.
func TestAssignConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	const repos = 8
	got := make([][]int, repos)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range repos * 3 {
		wg.Go(func() {
			repo := fmt.Sprintf("/repo%d", i%repos)
			p, err := Assign(path, repo, 7000, nil)
			if err != nil {
				t.Errorf("Assign(%s) = %v", repo, err)
				return
			}
			mu.Lock()
			got[i%repos] = append(got[i%repos], p)
			mu.Unlock()
		})
	}
	wg.Wait()
	seen := map[int]int{}
	for r, ports := range got {
		if len(ports) == 0 {
			continue
		}
		for _, p := range ports {
			if p != ports[0] {
				t.Errorf("repo%d got ports %v, want one", r, ports)
				break
			}
		}
		if other, dup := seen[ports[0]]; dup {
			t.Errorf("repo%d and repo%d share port %d", r, other, ports[0])
		}
		seen[ports[0]] = r
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Servers) != repos {
		t.Errorf("the map holds %d servers, want %d: %+v", len(m.Servers), repos, m.Servers)
	}
}

func TestAssignNoPortLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	_, err := Assign(path, "/a", MaxPort-1, func(int) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "no free port") {
		t.Errorf("Assign = %v, want no free port", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a failed Assign wrote the map")
	}
}

func TestLoadRefusesWhatItCannotRead(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"broken", "{not json", "is broken"},
		{"newer", `{"version": 99, "servers": []}`, "newer sbnn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "servers.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
			// And nothing is written over it.
			if _, err := Assign(path, "/a", 6280, nil); err == nil {
				t.Error("Assign over an unreadable map succeeded")
			}
			b, _ := os.ReadFile(path)
			if string(b) != tt.content {
				t.Errorf("the file was rewritten: %q", b)
			}
		})
	}
}

func TestUpdateWritesNothingWhenNothingChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	if err := Update(path, func(*Map) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("an Update that changed nothing created the map")
	}
}
