// Package servermap remembers which port the sbnn server of each repository
// listens on.
//
// A server belongs to one repository, and its port is what its URLs are made
// of. The map is what keeps those URLs the same from one day to the next: a
// server that stopped when it went idle, or was restarted, comes back on the
// port its repository had, instead of whichever port happened to be free.
//
// It is one small JSON file in the state directory, shared by every sbnn on
// the machine, so it is only changed under a lock and only ever replaced
// whole.
package servermap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/tenntenn/sbnn/internal/filelock"
)

// fileVersion is the format this build writes and understands.
const fileVersion = 1

// MaxPort is the highest port Assign hands out.
const MaxPort = 65535

// Entry is the server of one repository.
type Entry struct {
	// Repository is the root of the repository, or "" (repository.None)
	// for the diffs sent from outside any repository.
	Repository string `json:"repository"`
	Port       int    `json:"port"`
}

// Map is the content of the file.
type Map struct {
	Version int     `json:"version"`
	Servers []Entry `json:"servers"`
	// Told lists the ports of servers started by an sbnn older than this
	// map, which belong to no repository, that the user has already been
	// told how to reach. It is said once per port, not on every invocation.
	Told []int `json:"told,omitempty"`
}

// Port returns the port of repo's server.
func (m *Map) Port(repo string) (int, bool) {
	for _, e := range m.Servers {
		if e.Repository == repo {
			return e.Port, true
		}
	}
	return 0, false
}

// Owner returns the repository port is kept for.
func (m *Map) Owner(port int) (string, bool) {
	for _, e := range m.Servers {
		if e.Port == port {
			return e.Repository, true
		}
	}
	return "", false
}

// Remove forgets the server of repo.
func (m *Map) Remove(repo string) {
	m.Servers = slices.DeleteFunc(m.Servers, func(e Entry) bool { return e.Repository == repo })
}

// Load reads the map at path. A file that does not exist yet is an empty
// map; one that cannot be read is an error, because guessing would hand a
// repository's port to another repository.
func Load(path string) (*Map, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Map{Version: fileVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Map
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is broken (%v): remove it, and every repository is given a port again", path, err)
	}
	if m.Version > fileVersion {
		return nil, fmt.Errorf("%s was written by a newer sbnn (format version %d, this one understands %d): upgrade sbnn",
			path, m.Version, fileVersion)
	}
	m.Version = fileVersion
	return &m, nil
}

// Update reads the map, lets fn change it, and writes it back, all under a
// lock held against every other sbnn: two invocations in two repositories
// that both find no port of their own must not be handed the same one. The
// file is written whole to a temporary file and renamed into place, so a
// reader never sees half of it, and nothing is written when fn returns an
// error or leaves the map as it was.
func Update(path string, fn func(m *Map) error) error {
	// The lock is taken on a file of its own: the map itself is replaced by
	// a rename, and a lock on the old file would not stop anyone opening
	// the new one.
	lf, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()
	unlock, err := filelock.Lock(lf)
	if err != nil {
		return fmt.Errorf("locking %s: %w", path, err)
	}
	defer unlock()

	m, err := Load(path)
	if err != nil {
		return err
	}
	before, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := fn(m); err != nil {
		return err
	}
	after, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if compact, err := json.Marshal(m); err == nil && string(compact) == string(before) {
		return nil
	}
	return writeAtomic(path, append(after, '\n'))
}

// Assign returns the port of repo's server, giving it one when it has none:
// the lowest port from base up that no other repository is kept on and that
// usable accepts. usable is asked under the lock, so it may look at the
// port itself - whether something listens on it - without another sbnn
// picking the same port in between.
func Assign(path, repo string, base int, usable func(port int) bool) (int, error) {
	var port int
	err := Update(path, func(m *Map) error {
		if p, ok := m.Port(repo); ok {
			port = p
			return nil
		}
		for p := base; p <= MaxPort; p++ {
			if _, taken := m.Owner(p); taken {
				continue
			}
			if usable != nil && !usable(p) {
				continue
			}
			m.Servers = append(m.Servers, Entry{Repository: repo, Port: p})
			port = p
			return nil
		}
		return fmt.Errorf("no free port from %d up for a new sbnn server", base)
	})
	return port, err
}

// writeAtomic replaces path with b.
func writeAtomic(path string, b []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".servers-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(name)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
