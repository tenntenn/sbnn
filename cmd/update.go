package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/selfupdate"
	"github.com/tenntenn/sbnn/version"
)

// goInstallHint is what a binary built by go install is told to run instead
// of sbnn update.
const goInstallHint = "go install github.com/tenntenn/sbnn@latest"

var (
	updateCheckOnly bool
	updateVersion   string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Replace this sbnn with the latest release",
	Long: `Replace this sbnn with the latest release from GitHub.

  $ sbnn update                     # install the latest release
  $ sbnn update --check             # only say whether there is a newer one
  $ sbnn update --version v1.2.3    # install that release, older ones included

sbnn update downloads the archive for this OS and architecture from
https://github.com/tenntenn/sbnn/releases, checks its SHA-256 against the
checksums.txt published with the same release, and only then takes the sbnn
binary out of it and renames it over this one, so the old binary keeps working
until the new one is complete. Nothing is replaced when the checksum does not
match or checksums.txt does not list the archive. It needs write access to the
directory sbnn is installed in.

It never installs an older release than this one unless --version names it.

A sbnn installed with go install is not replaced: run
` + goInstallHint + ` instead. A source build ("dev")
has no release to compare with and is not replaced either.

A server that is already running keeps running the old binary; sbnn update
says so, and sbnn --restart starts it again from the new one.

Setting GITHUB_TOKEN raises the GitHub API rate limit; the token is sent to
api.github.com and nowhere else.`,
	Args:         cobra.NoArgs,
	RunE:         runUpdate,
	SilenceUsage: true,
}

func init() {
	f := updateCmd.Flags()
	f.BoolVar(&updateCheckOnly, "check", false, "Only report whether a newer release exists; replace nothing")
	f.StringVar(&updateVersion, "version", "", "Release to install, such as v1.2.3, even when it is older than this sbnn")
}

// newUpdateSource is where sbnn update and the start-up check read releases
// from. Tests point it at an httptest server.
var newUpdateSource = func() *selfupdate.Source {
	return selfupdate.NewSource(os.Getenv("GITHUB_TOKEN"))
}

// executablePath is the real path of the running binary: os.Executable can
// return the symlink sbnn was started through (a Homebrew-style bin/
// directory, say), and renaming over the link would replace the link, not
// the binary.
var executablePath = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// updateOptions is everything sbnn update acts on, gathered in one place so
// that a test can run it against an httptest server and a binary in
// t.TempDir instead of the real ones.
type updateOptions struct {
	current     string
	goInstalled bool
	goos        string
	goarch      string
	checkOnly   bool
	version     string
	source      *selfupdate.Source
	executable  func() (string, error)
	// servers lists the sbnn servers that are running, to say which of them
	// still run the old binary.
	servers func(ctx context.Context) []serverListing
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	return doUpdate(cmd.Context(), cmd.OutOrStdout(), updateOptions{
		current:     version.Version,
		goInstalled: version.GoInstalled,
		goos:        runtime.GOOS,
		goarch:      runtime.GOARCH,
		checkOnly:   updateCheckOnly,
		version:     updateVersion,
		source:      newUpdateSource(),
		executable:  executablePath,
		servers:     serversStillRunning,
	})
}

// serversStillRunning lists the servers that answer. A failure to list them is
// not a failure of the update, which has already happened.
func serversStillRunning(ctx context.Context) []serverListing {
	all, err := listServers(ctx, currentRepository())
	if err != nil {
		return nil
	}
	var out []serverListing
	for _, s := range all {
		if s.Running {
			out = append(out, s)
		}
	}
	return out
}

func doUpdate(ctx context.Context, w io.Writer, o updateOptions) error {
	released := selfupdate.Canonical(o.current) != ""

	// Refused before anything is asked of GitHub: neither of these is
	// going to be replaced, whatever the latest release is.
	if !o.checkOnly {
		if o.goInstalled {
			return fmt.Errorf("this sbnn was installed with go install, so it is not replaced from the releases page; run %s instead", goInstallHint)
		}
		if !released {
			return fmt.Errorf("this sbnn is a source build (version %s), so there is no release to replace it with; download one from https://github.com/%s/releases", o.current, selfupdate.Repo)
		}
	}

	var (
		rel selfupdate.Release
		err error
	)
	if o.version != "" {
		tag := "v" + strings.TrimPrefix(o.version, "v")
		rel, err = o.source.Tagged(ctx, tag)
	} else {
		rel, err = o.source.Latest(ctx)
	}
	if err != nil {
		return fmt.Errorf("finding the release: %w", err)
	}

	if o.checkOnly {
		return reportUpdate(w, o, rel)
	}

	switch c := selfupdate.Compare(rel.Tag, o.current); {
	case c == 0:
		fmt.Fprintf(w, "sbnn %s is already installed\n", o.current)
		return nil
	case c < 0 && o.version == "":
		fmt.Fprintf(w, "sbnn %s is newer than the latest release, %s; nothing to do (--version installs an older release)\n", o.current, rel.Tag)
		return nil
	}

	name, err := selfupdate.ArchiveName(o.goos, o.goarch)
	if err != nil {
		return err
	}
	exe, err := o.executable()
	if err != nil {
		return fmt.Errorf("finding this sbnn's executable: %w", err)
	}
	// The whole archive is downloaded and verified before the directory
	// sbnn lives in is touched at all.
	archive, err := o.source.Download(ctx, rel, name)
	if err != nil {
		return err
	}
	bin := selfupdate.BinaryName(o.goos)
	err = selfupdate.Replace(exe, o.goos, func(f io.Writer) error {
		return selfupdate.Extract(archive, name, bin, f, selfupdate.MaxBinary)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "sbnn %s -> %s (%s)\n", o.current, rel.Tag, exe)

	if o.servers != nil {
		want := selfupdate.Canonical(rel.Tag)
		for _, s := range o.servers(ctx) {
			if selfupdate.Canonical(s.Version) == want {
				continue
			}
			fmt.Fprintf(w, "The server at %s (%s) still runs %s; restart it with: sbnn --restart --port %d\n",
				s.URL, listingRepository(s), s.Version, s.Port)
		}
	}
	return nil
}

// reportUpdate is sbnn update --check: what is installed, what the release
// is, and what to run to get it.
func reportUpdate(w io.Writer, o updateOptions, rel selfupdate.Release) error {
	var b bytes.Buffer
	switch {
	case selfupdate.Canonical(o.current) == "":
		fmt.Fprintf(&b, "sbnn %s is a source build; the latest release is %s\n", o.current, rel.Tag)
	case selfupdate.Newer(rel.Tag, o.current):
		fmt.Fprintf(&b, "sbnn %s is installed; %s is available: run %s\n", o.current, rel.Tag, updateCommand(o.goInstalled))
	case selfupdate.Compare(rel.Tag, o.current) == 0:
		fmt.Fprintf(&b, "sbnn %s is the latest release\n", o.current)
	default:
		fmt.Fprintf(&b, "sbnn %s is newer than %s\n", o.current, rel.Tag)
	}
	_, err := w.Write(b.Bytes())
	return err
}

// updateCommand is what to run to install a newer release.
func updateCommand(goInstalled bool) string {
	if goInstalled {
		return goInstallHint
	}
	return "sbnn update"
}
