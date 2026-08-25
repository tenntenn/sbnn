package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/server"
)

var (
	waitTimeout  time.Duration
	waitFormat   string
	waitJSON     bool
	waitExitCode bool
	waitQuiet    bool
)

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait until the review is submitted, then print it",
	Long: `Block until the human submits their review of a group, then print it.

The server pushes the notice over its event stream, so nothing is polled and
nothing is missed:

  $ git diff | sbnn --target api
  $ sbnn wait --target api          # returns when "Submit review" is pressed
  $ sbnn comments --target api      # ... or read them yourself afterwards

A review that was already submitted for the diffs sbnn holds returns straight
away, so waiting is safe to retry.

Waiting only helps while you are still around. When the review might land
after you are gone - a meeting, a night, a session that times out - register
what to do instead and let the server start it:

  $ git diff | sbnn --on-review 'claude -p "$(sbnn comments)"'

--timeout gives up after a while and exits with status 2, which tells a
caller "not reviewed yet" as opposed to "something went wrong". With
--exit-code, a review that left comments exits 1 and a clean one exits 0, so
a pipeline can carry on by itself:

  $ git diff | sbnn && sbnn wait -q && git commit -m "..."`,
	Args:         cobra.NoArgs,
	RunE:         runWait,
	SilenceUsage: true,
}

func init() {
	f := waitCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.DurationVar(&waitTimeout, "timeout", 0, "Give up after this long (0 waits forever)")
	f.StringVar(&waitFormat, "format", "prompt", "Output format: prompt, markdown or json")
	f.BoolVar(&waitJSON, "json", false, "Shorthand for --format json")
	f.BoolVar(&waitExitCode, "exit-code", false,
		"Exit 1 when the review left something to address, 0 when it did not")
	f.BoolVarP(&waitQuiet, "quiet", "q", false, "Print nothing; implies --exit-code")
}

// exitNotReviewed is the status of a wait that timed out. It is not a
// failure: the review simply has not happened yet.
const exitNotReviewed = 2

func runWait(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}
	// Everything the flags alone can decide is decided here, before the
	// server is contacted and long before the wait begins: a wait that runs
	// for hours and then refuses to print what it waited for is the review
	// gone, not just an error message.
	format, err := resolveFormat(waitFormat, waitJSON)
	if err != nil {
		return err
	}
	c := client.New(addr(), 10*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}

	g, err := c.Group(ctx, group)
	if err != nil {
		return err
	}
	if g.Reviewed() {
		// The review landed before anyone started waiting for it.
		return printReview(ctx, c, group, format)
	}

	if waitTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, waitTimeout)
		defer cancel()
	}
	fmt.Fprintf(os.Stderr, "sbnn: waiting for the review of %s\n", server.GroupURL(c.BaseURL(), group))

	notice, err := c.WaitForReview(ctx, group)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "sbnn: no review after %s\n", waitTimeout)
			os.Exit(exitNotReviewed)
		}
		return err
	}
	fmt.Fprintf(os.Stderr, "sbnn: review submitted, %d open comment(s)\n", notice.Comments)
	return printReview(ctx, c, group, format)
}

// exitReview ends with the status the reviewer's verdict calls for.
func exitReview(ctx context.Context, c *client.Client, group string) error {
	g, err := c.Group(ctx, group)
	if err != nil {
		return err
	}
	return exitWithVerdict(g.ReviewVerdict, g.Comments)
}

// printReview writes the review the same way `sbnn comments` does. The
// format has already been resolved and checked by runWait.
func printReview(ctx context.Context, c *client.Client, group, format string) error {
	if waitQuiet {
		return exitReview(ctx, c, group)
	}
	switch format {
	case "json":
		comments, err := c.Comments(ctx, group)
		if err != nil {
			return err
		}
		open := comments[:0]
		for _, cm := range comments {
			if !cm.Resolved {
				open = append(open, cm)
			}
		}
		if err := jsonEncoder(os.Stdout).Encode(open); err != nil {
			return err
		}
		if waitExitCode {
			return exitReview(ctx, c, group)
		}
		return nil
	case "prompt", "markdown":
		text, err := c.Prompt(ctx, group, false, format == "prompt")
		if err != nil {
			return err
		}
		fmt.Print(text)
		if waitExitCode {
			return exitReview(ctx, c, group)
		}
		return nil
	default:
		// Unreachable: runWait resolves the format before waiting.
		return fmt.Errorf("unknown format %q: use prompt, markdown or json", format)
	}
}
