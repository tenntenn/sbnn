package cmd

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/client"
)

// freeAddr returns an address nothing is listening on, by taking one and
// giving it back.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

// An occupied address used to cost ten seconds of silence and an error that
// named a log file instead of the problem. It has to be reported here, before
// anything is spawned, and it has to name the address.
func TestCheckAddrFree(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer busy.Close()

	t.Run("taken", func(t *testing.T) {
		addr := busy.Addr().String()
		err := checkAddrFree(addr)
		if err == nil {
			t.Fatal("checkAddrFree on a listening address returned nil, want an error")
		}
		if !strings.Contains(err.Error(), "cannot start the sbnn server") {
			t.Errorf("error %q does not say what failed", err)
		}
		// The way out is a different port, and saying so is the point of
		// reporting this at all - but only where the address being taken is
		// what the platform actually reported.
		if errors.Is(err, syscall.EADDRINUSE) {
			for _, want := range []string{addr, "--port"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		}
	})

	t.Run("free", func(t *testing.T) {
		addr := freeAddr(t)
		if err := checkAddrFree(addr); err != nil {
			t.Fatalf("checkAddrFree(%s) = %v, want nil", addr, err)
		}
		// The probe must hand the address back, or the server it was run
		// for could not use it.
		if err := checkAddrFree(addr); err != nil {
			t.Fatalf("checkAddrFree(%s) twice = %v, want nil", addr, err)
		}
	})
}

// The reason a background server gave up reaches this process through its log
// and nowhere else, so the line has to be found in it.
func TestLogError(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{name: "empty"},
		{
			name: "only the announcement",
			out:  "sbnn: serving at http://localhost:6280 (pid 42)\n",
		},
		{
			name: "bind failure",
			out:  "sbnn: cannot listen on localhost:6401: listen tcp 127.0.0.1:6401: bind: address already in use\n",
			want: "cannot listen on localhost:6401: listen tcp 127.0.0.1:6401: bind: address already in use",
		},
		{
			// The old order wrote the announcement first even when the
			// listener never opened; the failure below it still counts.
			name: "announced, then failed anyway",
			out: "sbnn: serving at http://localhost:6401 (pid 42)\n" +
				"sbnn: cannot listen on localhost:6401: bind: address already in use\n",
			want: "cannot listen on localhost:6401: bind: address already in use",
		},
		{
			// slog is what the server uses for what does not stop it.
			name: "warnings are not failures",
			out: `time=2026-08-25T09:00:00.000Z level=WARN msg="could not restore session" error="no such file"` + "\n" +
				`time=2026-08-25T09:00:00.001Z level=WARN msg="Markdown preview will open in a separate window"` + "\n",
		},
		{
			name: "a warning does not hide the failure under it",
			out: `time=2026-08-25T09:00:00.000Z level=WARN msg="could not restore session"` + "\n" +
				"sbnn: refusing to bind to 0.0.0.0\n",
			want: "refusing to bind to 0.0.0.0",
		},
		{
			name: "trailing carriage returns are trimmed",
			out:  "sbnn: serving at http://localhost:6280 (pid 42)\r\nsbnn: history file is a directory\r\n",
			want: "history file is a directory",
		},
		{
			name: "a bare prefix says nothing",
			out:  "sbnn: \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logError(tt.out); got != tt.want {
				t.Errorf("logError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The log is appended to across runs, so only what this run wrote counts.
func TestServerLogError(t *testing.T) {
	older := "sbnn: serving at http://localhost:6280 (pid 1)\nsbnn: cannot listen on localhost:6280: bind: address already in use\n"
	path := filepath.Join(t.TempDir(), "server-6280.log")
	if err := os.WriteFile(path, []byte(older), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	offset := int64(len(older))

	t.Run("an earlier run is not this run's failure", func(t *testing.T) {
		if got := serverLogError(path, offset); got != "" {
			t.Errorf("serverLogError() = %q, want %q", got, "")
		}
	})

	t.Run("what this run wrote is read back", func(t *testing.T) {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := f.WriteString("sbnn: cannot listen on localhost:6280: bind: permission denied\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		want := "cannot listen on localhost:6280: bind: permission denied"
		if got := serverLogError(path, offset); got != want {
			t.Errorf("serverLogError() = %q, want %q", got, want)
		}
	})

	t.Run("no log, nothing to say", func(t *testing.T) {
		if got := serverLogError("", 0); got != "" {
			t.Errorf("serverLogError(\"\") = %q, want %q", got, "")
		}
		missing := filepath.Join(t.TempDir(), "absent.log")
		if got := serverLogError(missing, 0); got != "" {
			t.Errorf("serverLogError(missing) = %q, want %q", got, "")
		}
	})
}

// A server that has already failed is not going to answer, and waiting the
// full timeout for it is the ten-second hang this fixes.
func TestWaitForReadyReportsAFailureInsteadOfWaiting(t *testing.T) {
	c := client.New(freeAddr(t), 200*time.Millisecond)
	reason := "cannot listen on localhost:6401: bind: address already in use"

	start := time.Now()
	st, err := waitForReady(context.Background(), c, spawnTimeout, func() string { return reason })
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("waitForReady returned %+v, want an error", st)
	}
	if !strings.Contains(err.Error(), reason) {
		t.Errorf("error %q does not carry the reason %q", err, reason)
	}
	if !strings.Contains(err.Error(), c.Addr) {
		t.Errorf("error %q does not name the address %q", err, c.Addr)
	}
	if elapsed >= spawnTimeout {
		t.Errorf("waited %s, want the failure reported well before the %s timeout", elapsed, spawnTimeout)
	}
}

// With nothing reported, the wait is still bounded by the timeout.
func TestWaitForReadyTimesOutWhenNothingIsReported(t *testing.T) {
	c := client.New(freeAddr(t), 50*time.Millisecond)
	_, err := waitForReady(context.Background(), c, 300*time.Millisecond, func() string { return "" })
	if err == nil {
		t.Fatal("waitForReady returned nil, want a timeout error")
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("error %q is not the timeout", err)
	}
}

// A cancelled context stops the wait.
func TestWaitForReadyStopsWithTheContext(t *testing.T) {
	c := client.New(freeAddr(t), 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := waitForReady(ctx, c, spawnTimeout, nil); err == nil {
		t.Fatal("waitForReady on a cancelled context returned nil, want an error")
	}
}

// The background server is a second process, so whatever this invocation
// resolved has to travel to it as a flag. Dropping the flag when the value
// could not be resolved sent the server off to its own default and left the
// reviews piling up in a file nobody asked for - the silent log the refusal of
// "-" exists to prevent - so a value historyFile refuses has to stop the spawn
// instead.
func TestHistoryFileArgs(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    string // the value expected after --history-file
		wantAbs bool
		wantErr bool
	}{
		{name: "off word", flag: "off", want: "off"},
		{name: "false", flag: "false", want: "off"},
		{name: "zero", flag: "0", want: "off"},
		{name: "disabled", flag: "disabled", want: "off"},
		{name: "off from env", env: "none", want: "off"},
		{name: "path", flag: "reviews.jsonl", wantAbs: true},
		{name: "path from env", env: "reviews.jsonl", wantAbs: true},
		{name: "dash", flag: "-", wantErr: true},
		{name: "dash from env", env: "-", wantErr: true},
		{name: "padded dash", flag: " - ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv(HistoryEnv, tt.env)

			args, err := historyFileArgs(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("historyFileArgs(%q) returned %v, want the server not to be started at all",
						tt.flag, args)
				}
				if !strings.Contains(err.Error(), "standard stream") {
					t.Errorf("historyFileArgs(%q) failed with %q, want the standard stream complaint",
						tt.flag, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("historyFileArgs(%q): %v", tt.flag, err)
			}
			if len(args) != 2 || args[0] != "--history-file" {
				t.Fatalf("historyFileArgs(%q) = %v, want a --history-file pair", tt.flag, args)
			}
			// An empty value would tell the server to use its own default, and
			// the server has no way to tell that apart from "not given".
			if args[1] == "" {
				t.Fatalf("historyFileArgs(%q) passed an empty --history-file", tt.flag)
			}
			switch {
			case tt.wantAbs:
				if !filepath.IsAbs(args[1]) {
					t.Errorf("historyFileArgs(%q) = %q, want an absolute path", tt.flag, args[1])
				}
			default:
				if args[1] != tt.want {
					t.Errorf("historyFileArgs(%q) = %q, want %q", tt.flag, args[1], tt.want)
				}
				if !slices.Contains(HistoryOffWords, args[1]) {
					t.Errorf("historyFileArgs(%q) = %q, which the server does not read as off",
						tt.flag, args[1])
				}
			}
		})
	}
}
