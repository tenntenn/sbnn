// Package mo drives the mo Markdown viewer (https://github.com/k1LoW/mo).
//
// sbnn does not render Markdown itself. It hands the file to mo, which keeps
// its own resident server, and embeds the resulting page next to the diff.
//
// mo cannot be used as a Go library: everything but its cobra entry point
// lives under internal/, and the published module does not carry the
// embedded SPA, so it does not even build as a dependency. Driving the
// installed binary is therefore the supported integration point, and its
// --json output is a documented interface.
package mo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultPort is mo's own default port.
const DefaultPort = 6275

// DefaultBind is the address mo binds to by default.
const DefaultBind = "localhost"

// ErrNotInstalled is returned when the mo binary cannot be found.
var ErrNotInstalled = errors.New("mo is not installed")

// InstallHint tells the user how to get mo. mo is distributed as a
// pre-built binary; "go install" does not work because the published module
// does not contain mo's embedded frontend.
const InstallHint = "install mo with `brew install k1LoW/tap/mo`, " +
	"or download a binary from https://github.com/k1LoW/mo/releases"

// Runner runs the mo command.
type Runner struct {
	// Bin is the mo executable, "mo" by default.
	Bin string
	// Port and Bind describe the mo server sbnn talks to.
	Port int
	Bind string
}

// New returns a Runner with defaults applied.
func New(bin string, port int, bind string) *Runner {
	if bin == "" {
		bin = "mo"
	}
	if port == 0 {
		port = DefaultPort
	}
	if bind == "" {
		bind = DefaultBind
	}
	return &Runner{Bin: bin, Port: port, Bind: bind}
}

// Addr returns the host:port of the mo server.
func (r *Runner) Addr() string {
	return net.JoinHostPort(r.Bind, strconv.Itoa(r.Port))
}

// BaseURL returns the URL of the mo server.
func (r *Runner) BaseURL() string {
	return "http://" + r.Addr()
}

// Available reports whether the mo binary can be found.
func (r *Runner) Available() error {
	if _, err := exec.LookPath(r.Bin); err != nil {
		return fmt.Errorf("%w: %s", ErrNotInstalled, InstallHint)
	}
	return nil
}

// File is one entry of mo's --json output.
type File struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Result is mo's --json output.
type Result struct {
	URL   string `json:"url"`
	Files []File `json:"files"`
}

// Open adds paths to mo's group and returns mo's answer. mo starts its own
// resident server on first use and adds to it afterwards, so this is safe to
// call repeatedly.
func (r *Runner) Open(ctx context.Context, group string, paths ...string) (*Result, error) {
	if err := r.Available(); err != nil {
		return nil, err
	}
	args := []string{
		"--json",
		"--no-open",
		"--port", strconv.Itoa(r.Port),
		"--bind", r.Bind,
	}
	if group != "" {
		args = append(args, "--target", group)
	}
	args = append(args, paths...)

	cmd := exec.CommandContext(ctx, r.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("mo failed: %s", msg)
	}

	var res Result
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &res); err != nil {
		return nil, fmt.Errorf("cannot read mo output: %w", err)
	}
	return &res, nil
}

// URLFor returns the deep link mo reported for path, or "" when mo reported
// no file whose path is the one asked for.
//
// mo echoes back a path of its own making: it may be absolute where sbnn
// passed a relative one, cleaned, or resolved through a symlink. So a plain
// string comparison is only the fast path; anything else is compared as
// canonical paths. Nothing matching is answered with "" on purpose. The
// group URL would render, but it is some other file's page, and framing it
// tells the reviewer nothing is wrong when the deep link in fact failed.
func (res *Result) URLFor(path string) string {
	for _, f := range res.Files {
		if f.Path == path {
			return f.URL
		}
	}
	want := canonicalPath(path)
	if want == "" {
		return ""
	}
	for _, f := range res.Files {
		if canonicalPath(f.Path) == want {
			return f.URL
		}
	}
	return ""
}

// canonicalPath returns the form of p used to compare mo's answer with the
// path sbnn asked about: absolute, cleaned, and with symlinks resolved.
// Resolving needs the file to exist, so a path that cannot be resolved -- a
// deleted file, a temporary copy already gone -- falls back to the cleaned
// absolute path. An empty path stays empty: it identifies no file, and must
// not be allowed to match one.
func canonicalPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}
