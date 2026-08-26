package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/version"
)

const (
	// listenGrace is how long runServer lets Run fail before announcing the
	// server. Binding an address that is taken fails in microseconds, so the
	// grace only has to be long enough to lose that race on purpose.
	listenGrace = 200 * time.Millisecond

	// spawnTimeout bounds the wait for a background server to answer. It is
	// the last resort: a server that fails outright says so through its log
	// long before this runs out.
	spawnTimeout = 10 * time.Second

	// logTailMax bounds how much of the server log is read back when looking
	// for the reason the server stopped.
	logTailMax = 64 << 10
)

// runServer runs the sbnn server in the foreground. The background server is
// this same code, started by spawnServer with --foreground.
func runServer(ctx context.Context) error {
	sessionFile, err := paths.SessionFile(port)
	if err != nil {
		return err
	}
	cacheDir, err := paths.CacheDir()
	if err != nil {
		return err
	}
	historyFile, err := historyFile(historyPath)
	if err != nil {
		return err
	}
	srv, err := server.New(server.Options{
		Bind:        bind,
		Port:        port,
		SessionFile: sessionFile,
		CacheDir:    cacheDir,
		HistoryFile: historyFile,
		Mo:          moRunner(),
		Version:     version.Version,
		Revision:    version.Revision,
		AllowRemote: allowRemote,
		IdleTimeout: idleTimeout,
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Announce the server only once it is listening. Run opens the listener
	// itself, so saying it first put a line claiming success at the top of
	// the log of a server that was about to fail on a busy address - and
	// that log is all a background server ever gets to say.
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(listenGrace):
	}
	fmt.Fprintf(os.Stderr, "sbnn: serving at %s (pid %d)\n", srv.BaseURL(), os.Getpid())
	return <-errCh
}

// spawnServer starts the server in the background and waits until it answers.
func spawnServer(ctx context.Context, c *client.Client) (*server.Status, error) {
	// Whatever holds the address, the child is going to hit it too, and can
	// only complain about it into its log. Ask here, where the answer can be
	// put in front of the user at once instead of after the timeout.
	if err := checkAddrFree(addr()); err != nil {
		return nil, err
	}
	bin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find the sbnn binary: %w", err)
	}
	// The server is the one that writes the log, so it has to be told where
	// this invocation wants it.
	history, err := historyFileArgs(historyPath)
	if err != nil {
		return nil, err
	}
	args := backgroundServerArgs(history)

	cmd := exec.Command(bin, args...)
	logPath, logFile := openLog()
	// Where this server's own output begins, so that what it says can be
	// told apart from every run before it.
	var logOffset int64
	if logFile != nil {
		defer logFile.Close()
		if fi, err := logFile.Stat(); err == nil {
			logOffset = fi.Size()
		}
		cmd.Stdout, cmd.Stderr = logFile, logFile
	}
	setSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start the sbnn server: %w", err)
	}
	pid := cmd.Process.Pid
	// Detach: the server outlives the invocation that started it.
	if err := cmd.Process.Release(); err != nil {
		return nil, err
	}

	st, err := waitForReady(ctx, c, spawnTimeout, func() string {
		return serverLogError(logPath, logOffset)
	})
	if err != nil {
		if logPath != "" {
			return nil, fmt.Errorf("%w (pid %d, see %s)", err, pid, logPath)
		}
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "sbnn: serving at %s (pid %d)\n", st.URL, st.PID)
	return st, nil
}

// backgroundServerArgs is the command line the detached server is started
// with, given the --history-file arguments historyFileArgs worked out.
//
// The background server is a separate process that parses these flags for
// itself, so anything this invocation resolved and does not pass here is a
// setting the server that outlives it never sees. --idle-timeout is the one
// that hurts most quietly: left off, the only server that ever outlives
// anything runs on the zero value and stays resident forever, while every
// test that goes through server.Options still passes.
func backgroundServerArgs(history []string) []string {
	args := []string{
		"--foreground",
		"--port", strconv.Itoa(port),
		"--bind", bind,
		"--mo-bin", moBin,
		"--mo-port", strconv.Itoa(moPort),
		"--mo-bind", moBind,
		"--idle-timeout", idleTimeout.String(),
	}
	args = append(args, history...)
	if allowRemote {
		args = append(args, "--dangerously-allow-remote-access")
	}
	return args
}

// historyFileArgs is the --history-file the background server is started with.
// The server writes the log, so it has to be told what this invocation
// resolved: the flag it was given, or $SBNN_HISTORY, or the state directory.
//
// A value historyFile refuses is reported here rather than dropped. Passing no
// --history-file used to be the fallback, which sent the server off to its own
// default and let reviews pile up in a file the caller never asked for - the
// silent log this refusal exists to prevent. It also left the server to fail on
// $SBNN_HISTORY by itself, with nothing to show for it but a readiness timeout.
func historyFileArgs(flag string) ([]string, error) {
	resolved, err := historyFile(flag)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		// The server parses this string the same way, so send a word it knows
		// rather than an empty value, which would mean "use the default".
		return []string{"--history-file", HistoryOffWords[0]}, nil
	}
	return []string{"--history-file", resolved}, nil
}

// checkAddrFree reports whether a server can still be started on addr. The
// listener is opened and closed again: what is wanted is not the socket but
// the error, which is the one the spawned server would hit and never get to
// show anyone.
func checkAddrFree(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln.Close()
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("cannot start the sbnn server: %w "+
			"(something else is already listening on %s; stop it, or pass --port to use another port)", err, addr)
	}
	return fmt.Errorf("cannot start the sbnn server: %w", err)
}

// waitForReady polls until the server answers. failed, when it returns a
// reason, cuts the wait short: the server has already said it is not coming
// up, and there is nothing left to wait for.
func waitForReady(ctx context.Context, c *client.Client, timeout time.Duration, failed func() string) (*server.Status, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := probe(ctx, c, 500*time.Millisecond); err == nil {
			return st, nil
		}
		if failed != nil {
			if reason := failed(); reason != "" {
				return nil, fmt.Errorf("the sbnn server on %s stopped: %s", c.Addr, reason)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("the sbnn server on %s did not become ready", c.Addr)
}

// serverLogError returns the failure the background server reported in its
// log after offset, or "" while it has reported none. A detached child has no
// other way back: its output goes to the log and this process never waits on
// it, so the log is where the reason has to be read from.
func serverLogError(path string, offset int64) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, logTailMax))
	if err != nil {
		return ""
	}
	return logError(string(b))
}

// logError picks the server's complaint out of what it wrote. Exactly two
// kinds of line carry the "sbnn: " prefix: the one announcing the server, and
// the one Execute prints on its way out. Everything else in the log is
// slog's, and slog is used for what does not stop the server.
func logError(out string) string {
	for line := range strings.SplitSeq(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "sbnn: ")
		if !ok || rest == "" || strings.HasPrefix(rest, "serving at ") {
			continue
		}
		return rest
	}
	return ""
}

// openLog returns the log file of the background server. Failing to open it
// is not fatal; the server then simply logs nowhere.
func openLog() (string, *os.File) {
	dir, err := paths.StateDir()
	if err != nil {
		return "", nil
	}
	path := filepath.Join(dir, fmt.Sprintf("server-%d.log", port))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", nil
	}
	return path, f
}
