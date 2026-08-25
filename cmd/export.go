package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/export"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/version"
	"github.com/tenntenn/sbnn/web"
)

var (
	exportFragment bool
	exportTitle    string
	exportForce    bool
)

var exportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Write a review as a single self-contained HTML page",
	Long: `Write a review as one self-contained HTML page.

The page carries the same UI as the sbnn server with the diff frozen into it:
no server, no mo, no network. Comments can still be written; they are kept in
the browser and "Copy prompt" produces the same text as ` + "`sbnn comments`" + `.

  $ git diff | sbnn export review.html    # straight from stdin, no server needed
  $ sbnn export -t api review.html        # the "api" group of a running server
  $ git diff | sbnn export                # to stdout

  # body only, to embed in a page that brings its own <html> (an artifact,
  # a static site, a mail):
  $ git diff | sbnn export --fragment review.body.html

The file is written where you point it, and the argument is positional, so
an existing file that sbnn did not write is left alone and reported instead
of overwritten. Refreshing an earlier export is not stopped: a page sbnn
rendered is recognised as one and replaced. --force overwrites either way.
Nothing is ever asked, so this works the same in a pipe and in CI.`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runExport,
	SilenceUsage: true,
}

func init() {
	f := exportCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Port of the sbnn server to read the group from")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address of the sbnn server")
	f.StringVar(&title, "title", "", "Title of the diff read from stdin")
	f.StringVar(&exportTitle, "page-title", "", "Title of the generated page")
	f.BoolVar(&exportFragment, "fragment", false, "Write only the page body, for embedding")
	f.BoolVarP(&exportForce, "force", "f", false, "Overwrite the file even if sbnn did not write it")
}

func runExport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}

	content, err := readStdin()
	if err != nil {
		return err
	}

	var g *model.Group
	switch {
	case content != "":
		// A piped diff is exported on its own: no server is started, and a
		// running one is left alone.
		files := diff.Parse(content)
		if len(files) == 0 {
			return fmt.Errorf("no file diff found in the input")
		}
		name := title
		if name == "" {
			name = "diff 1"
		}
		g = &model.Group{
			Name: group,
			Diffs: []*model.Diff{{
				ID:        "d1",
				Title:     name,
				BaseDir:   workingDir(),
				CreatedAt: time.Now(),
				Raw:       content,
				Files:     files,
			}},
		}
	default:
		c := client.New(addr(), 10*time.Second)
		if _, err := c.Status(ctx); err != nil {
			return fmt.Errorf("no diff on stdin and no sbnn server on %s", c.Addr)
		}
		g, err = c.Group(ctx, group)
		if err != nil {
			return err
		}
		if len(g.Diffs) == 0 {
			return fmt.Errorf("group %q has no diff to export", group)
		}
	}

	payload := export.Build(g, version.Version, time.Now())
	page, err := export.Render(payload, web.FS(), export.Options{
		Title:    exportTitle,
		Fragment: exportFragment,
	})
	if err != nil {
		return err
	}

	if len(args) == 0 {
		_, err := os.Stdout.WriteString(page)
		return err
	}
	if err := checkOverwrite(args[0], exportForce); err != nil {
		return err
	}
	if err := os.WriteFile(args[0], []byte(page), 0o644); err != nil {
		return err
	}
	files := 0
	for _, d := range g.Diffs {
		files += len(d.Files)
	}
	fmt.Fprintf(os.Stderr, "sbnn: wrote %s (%d file(s), %d preview(s), %d KiB)\n",
		args[0], files, len(payload.Previews), len(page)/1024)
	return nil
}

// exportMarker is the line every page sbnn renders reads its data back out
// of, fragment or not. Finding it is what tells an earlier export apart
// from a file that happens to sit at the same path.
const exportMarker = "window.__SBNN_DATA__ = "

// scanLimit is how far into a file the marker is looked for. An exported
// page carries it after the stylesheet, a few hundred KiB in; the limit is
// there so that pointing export at something enormous does not read all of
// it. A page past the limit is treated as not ours, which errs towards
// keeping the file.
const scanLimit = 8 << 20

// checkOverwrite refuses to destroy a file sbnn did not write. The
// destination is positional and unlabelled, so "sbnn export index.html" in
// the wrong directory is a plausible slip rather than a theoretical one,
// and every other destructive thing here either asks or is a flag typed on
// purpose.
//
// It never prompts: export is made to sit in a pipe, and a question asked
// of a pipe or of CI is a hang. Overwriting an earlier export is allowed,
// so "refresh that page" stays one command.
func checkOverwrite(path string, force bool) error {
	if force {
		return nil
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		// Unreadable for some other reason: let the write say why.
		return nil
	case !info.Mode().IsRegular():
		// /dev/null, a fifo, a device: writing there destroys nothing.
		return nil
	case isExportedPage(path):
		return nil
	}
	return fmt.Errorf("%s already exists and was not written by sbnn (pass --force to overwrite)", path)
}

// isExportedPage reports whether the file at path is a page sbnn exported.
// The marker can straddle two reads, so the tail of each buffer is carried
// into the next one.
func isExportedPage(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	marker := []byte(exportMarker)
	buf := make([]byte, 64<<10)
	keep, read := 0, 0
	for read < scanLimit {
		n, err := f.Read(buf[keep:])
		if n > 0 {
			read += n
			if bytes.Contains(buf[:keep+n], marker) {
				return true
			}
			end := keep + n
			keep = min(len(marker)-1, end)
			copy(buf, buf[end-keep:end])
		}
		if err != nil {
			return false
		}
	}
	return false
}
