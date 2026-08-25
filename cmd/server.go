package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/version"
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
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "sbnn: serving at %s (pid %d)\n", srv.BaseURL(), os.Getpid())
	return srv.Run(ctx)
}

// spawnServer starts the server in the background and waits until it answers.
func spawnServer(ctx context.Context, c *client.Client) (*server.Status, error) {
	bin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find the sbnn binary: %w", err)
	}
	args := []string{
		"--foreground",
		"--port", strconv.Itoa(port),
		"--bind", bind,
		"--mo-bin", moBin,
		"--mo-port", strconv.Itoa(moPort),
		"--mo-bind", moBind,
	}
	// The server is the one that writes the log, so it has to be told where
	// this invocation wants it.
	history, err := historyFileArgs(historyPath)
	if err != nil {
		return nil, err
	}
	args = append(args, history...)
	if allowRemote {
		args = append(args, "--dangerously-allow-remote-access")
	}

	cmd := exec.Command(bin, args...)
	logPath, logFile := openLog()
	if logFile != nil {
		defer logFile.Close()
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

	st, err := waitForReady(ctx, c, 10*time.Second)
	if err != nil {
		if logPath != "" {
			return nil, fmt.Errorf("%w (pid %d, see %s)", err, pid, logPath)
		}
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "sbnn: serving at %s (pid %d)\n", st.URL, st.PID)
	return st, nil
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

func waitForReady(ctx context.Context, c *client.Client, timeout time.Duration) (*server.Status, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := probe(ctx, c, 500*time.Millisecond); err == nil {
			return st, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("the sbnn server on %s did not become ready", c.Addr)
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
