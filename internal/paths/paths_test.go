package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// sandboxHome points every "where does this user live" lookup the standard
// library makes at a temporary directory, so a test that falls through to a
// platform default cannot read or create anything under the real home.
func sandboxHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "AppData", "LocalAppData"} {
		t.Setenv(k, home)
	}
	return home
}

func TestStateBase(t *testing.T) {
	tests := []struct {
		name string
		goos string
		xdg  string
		want func(t *testing.T, home string) string
	}{
		{
			name: "linux honors XDG_STATE_HOME",
			goos: "linux",
			xdg:  "set",
			want: func(t *testing.T, home string) string { return filepath.Join(home, "xdg-state") },
		},
		{
			name: "darwin honors XDG_STATE_HOME too",
			goos: "darwin",
			xdg:  "set",
			want: func(t *testing.T, home string) string { return filepath.Join(home, "xdg-state") },
		},
		{
			name: "windows honors XDG_STATE_HOME too",
			goos: "windows",
			xdg:  "set",
			want: func(t *testing.T, home string) string { return filepath.Join(home, "xdg-state") },
		},
		{
			name: "linux without XDG_STATE_HOME uses ~/.local/state",
			goos: "linux",
			want: func(t *testing.T, home string) string { return filepath.Join(home, ".local", "state") },
		},
		{
			name: "darwin without XDG_STATE_HOME uses the OS config dir",
			goos: "darwin",
			want: userConfigDir,
		},
		{
			name: "windows without XDG_STATE_HOME uses the OS config dir",
			goos: "windows",
			want: userConfigDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := sandboxHome(t)
			if tt.xdg == "set" {
				t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))
			} else {
				t.Setenv("XDG_STATE_HOME", "")
			}
			got, err := stateBase(tt.goos)
			if err != nil {
				t.Fatalf("stateBase(%q) error: %v", tt.goos, err)
			}
			if want := tt.want(t, home); got != want {
				t.Errorf("stateBase(%q) = %q, want %q", tt.goos, got, want)
			}
		})
	}
}

func userConfigDir(t *testing.T, _ string) string {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir: %v", err)
	}
	return dir
}

func TestStateDirCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)

	dir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if want := filepath.Join(base, appName); dir != want {
		t.Fatalf("StateDir = %q, want %q", dir, want)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %q: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("mode = %#o, want 0700", perm)
		}
	}
}

func TestStateFiles(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	state := filepath.Join(base, appName)

	tests := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{
			name: "session file for the default port",
			got:  func() (string, error) { return SessionFile(6280) },
			want: filepath.Join(state, "session-6280.json"),
		},
		{
			name: "session file for another port",
			got:  func() (string, error) { return SessionFile(1) },
			want: filepath.Join(state, "session-1.json"),
		},
		{
			name: "review log",
			got:  HistoryFile,
			want: filepath.Join(state, "reviews.jsonl"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.got()
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCacheDirXDG pins the asymmetry the README has to describe: unlike
// $XDG_STATE_HOME, $XDG_CACHE_HOME is only consulted on Unix, because
// os.UserCacheDir ignores it on macOS and Windows.
func TestCacheDirXDG(t *testing.T) {
	home := sandboxHome(t)
	xdg := filepath.Join(home, "xdg-cache")
	t.Setenv("XDG_CACHE_HOME", xdg)

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	underXDG := dir == filepath.Join(xdg, appName)
	switch runtime.GOOS {
	case "darwin", "windows":
		if underXDG {
			t.Errorf("CacheDir = %q, want a path outside $XDG_CACHE_HOME on %s", dir, runtime.GOOS)
		}
	default:
		if !underXDG {
			t.Errorf("CacheDir = %q, want %q", dir, filepath.Join(xdg, appName))
		}
	}
	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("stat %q: %v", dir, err)
	} else if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
}
