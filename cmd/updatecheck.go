package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/selfupdate"
	"github.com/tenntenn/sbnn/version"
)

// hookChildEnv is set by the server in the environment of every command
// hook (see server.hookEnv). An sbnn run from a hook is part of a review
// that has just been submitted, its stderr goes to the server's log, and a
// notice there helps nobody.
const hookChildEnv = "SBNN_SERVER"

// updateCheck is the start-up check of this invocation, or nil when it was
// skipped. It is started by the root command's PersistentPreRun, once the
// flags are parsed, and read by Execute once the command has finished.
var updateCheck *selfupdate.Check

// updateCheckEnv carries what updateCheckSkipped decides on, so that a test
// can give it any combination.
type updateCheckEnv struct {
	getenv     func(string) string
	current    string
	stderrTerm bool
}

// updateCheckSkipped says why the start-up check is not run for cmd, or ""
// when it is.
//
// The check exists to tell a person at a terminal about a release, so
// everything that is not one is left alone: output that is going to a
// program (a pipe, --json), the background server, a hook's child, and the
// commands for which a notice is either redundant (update) or noise (shell
// completion). A source build has no release to compare with. A binary
// installed with go install is still checked; it is only the hint that
// differs.
func updateCheckSkipped(cmd *cobra.Command, env updateCheckEnv) string {
	switch {
	case env.getenv(NoUpdateCheckEnv) != "":
		return NoUpdateCheckEnv + " is set"
	case selfupdate.Canonical(env.current) == "":
		return "a source build"
	case env.getenv(hookChildEnv) != "":
		return "run from a hook"
	case !env.stderrTerm:
		return "stderr is not a terminal"
	}
	switch cmd.Name() {
	case "update", "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return "the " + cmd.Name() + " command"
	}
	if flagIs(cmd, "foreground", "true") {
		return "the server"
	}
	if flagIs(cmd, "json", "true") || flagIs(cmd, "format", "json") {
		return "JSON output"
	}
	return ""
}

// flagIs reports whether cmd has a flag called name whose value is value.
func flagIs(cmd *cobra.Command, name, value string) bool {
	f := cmd.Flags().Lookup(name)
	return f != nil && f.Value.String() == value
}

// startUpdateCheck starts the check for cmd unless it is skipped.
func startUpdateCheck(cmd *cobra.Command, _ []string) {
	env := updateCheckEnv{getenv: os.Getenv, current: version.Version, stderrTerm: isTerminal(os.Stderr)}
	if updateCheckSkipped(cmd, env) != "" {
		return
	}
	path, err := paths.UpdateCheckFile()
	if err != nil {
		return
	}
	src := newUpdateSource()
	updateCheck = &selfupdate.Check{
		Path:    path,
		Current: version.Version,
		Latest: func(ctx context.Context) (string, error) {
			rel, err := src.Latest(ctx)
			return rel.Tag, err
		},
	}
	// Not the command's context: the check is meant to finish or be
	// abandoned on its own timeout, not to be cut short by whatever the
	// command does with its own.
	updateCheck.Start(context.Background())
}

// printUpdateNotice writes the one line the start-up check has to say, if
// it has an answer by now. It never waits for one.
func printUpdateNotice(w io.Writer, c *selfupdate.Check, goInstalled bool) {
	if c == nil {
		return
	}
	latest, ok := c.Notice()
	if !ok {
		return
	}
	fmt.Fprintf(w, "sbnn: %s is available (this is %s); run %s to update, or set %s=1 to stop checking\n",
		latest, c.Current, updateCommand(goInstalled), NoUpdateCheckEnv)
}

// cleanupReplacedExecutable removes the binary a sbnn update on Windows had
// to leave next to the new one, because Windows does not let a running
// executable be deleted. Elsewhere the rename replaced it outright.
func cleanupReplacedExecutable() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := executablePath(); err == nil {
		selfupdate.CleanupOld(exe)
	}
}
