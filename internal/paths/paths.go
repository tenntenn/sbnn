// Package paths resolves the directories sbnn keeps its state and its
// generated preview files in.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "sbnn"

// stateBase returns the parent directory of the sbnn state directory for the
// given GOOS.
//
// $XDG_STATE_HOME wins everywhere when it is set, so it is an escape hatch on
// macOS and Windows too. Otherwise macOS and Windows get the directory the OS
// keeps per-user application data in (~/Library/Application Support, %AppData%)
// rather than an XDG path, and everything else gets the XDG default.
func stateBase(goos string) (string, error) {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v, nil
	}
	if goos == "windows" || goos == "darwin" {
		return os.UserConfigDir()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

// StateDir returns the directory holding the session state, creating it if
// needed. It is $XDG_STATE_HOME/sbnn when that variable is set, and otherwise
// follows the platform convention: ~/.local/state/sbnn on Unix,
// ~/Library/Application Support/sbnn on macOS, %AppData%\sbnn on Windows.
func StateDir() (string, error) {
	base, err := stateBase(runtime.GOOS)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir: %w", err)
	}
	return dir, nil
}

// SessionFile returns the path of the session state file for a port. Servers
// on different ports keep independent sessions, like mo does.
func SessionFile(port int) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("session-%d.json", port)), nil
}

// HistoryFile returns the log of submitted reviews. It is one file for the
// whole machine on purpose: the point of keeping reviews is to read them
// together, long after the servers that recorded them are gone.
func HistoryFile() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reviews.jsonl"), nil
}

// CacheDir returns the directory for files sbnn generates, such as the
// Markdown reconstructed from a diff and handed to mo. It is the platform
// cache directory: $XDG_CACHE_HOME/sbnn or ~/.cache/sbnn on Unix,
// ~/Library/Caches/sbnn on macOS, %LocalAppData%\sbnn on Windows.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	return dir, nil
}
