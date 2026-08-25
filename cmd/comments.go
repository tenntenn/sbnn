package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
)

var (
	commentsFormat       string
	commentsResolved     bool
	commentsResolvedOnly bool
	commentsClear        bool
	commentsJSON         bool
	commentsExitCode     bool
	commentsQuiet        bool
)

var commentsCmd = &cobra.Command{
	Use:   "comments",
	Short: "Print the review comments left in the browser",
	Long: `Print the review comments of a group.

The comments live in the sbnn server, so an agent can read them back after a
human has written them:

  $ sbnn comments                    # ready to paste into an agent
  $ sbnn comments --format json      # machine readable
  $ sbnn comments -t api             # comments of the "api" group
  $ sbnn comments --clear            # drop them before the next round
  $ sbnn comments --clear --resolved-only   # ... keeping what is still open

It is meant to be combined with other commands, so it says what it found in
its exit status as well:

  $ sbnn comments --exit-code        # 1 when the review blocks the change
  $ sbnn wait && sbnn comments -q && git commit   # commit if the review was clean

--exit-code answers the same question sbnn wait and sbnn submit answer, and
in the same way: what the reviewer decided about this round outranks counting
comments. A review that asked for changes exits 1; an approval exits 0
whatever it said along the way; a plain "commented" - and a round nobody has
submitted yet, which is what a diff sent after the last review starts -
exits 1 only if a comment is left open.`,
	Args:         cobra.NoArgs,
	RunE:         runComments,
	SilenceUsage: true,
}

func init() {
	f := commentsCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVar(&commentsFormat, "format", "prompt", "Output format: prompt, markdown or json")
	f.BoolVar(&commentsResolved, "include-resolved", false, "Include comments marked as resolved")
	f.BoolVar(&commentsClear, "clear", false, "Remove the comments instead of printing them")
	f.BoolVar(&commentsResolvedOnly, "resolved-only", false,
		"With --clear, remove only the comments marked as resolved")
	f.BoolVar(&commentsJSON, "json", false, "Shorthand for --format json")
	f.BoolVar(&commentsExitCode, "exit-code", false,
		"Exit 1 when the review blocks the change, 0 when it does not")
	f.BoolVarP(&commentsQuiet, "quiet", "q", false, "Print nothing; implies --exit-code")
}

func runComments(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}
	if err := checkClearFlags(); err != nil {
		return err
	}
	c := client.New(addr(), 5*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}

	if commentsClear {
		removed, err := c.ClearComments(ctx, group, commentsResolvedOnly)
		if err != nil {
			return err
		}
		what := "comment(s)"
		if commentsResolvedOnly {
			what = "resolved comment(s)"
		}
		fmt.Fprintf(os.Stderr, "sbnn: removed %d %s from group %q\n", removed, what, group)
		return nil
	}

	if commentsQuiet {
		commentsExitCode = true
		return exitReview(ctx, c, group)
	}

	format := commentsFormat
	if commentsJSON {
		format = "json"
	}
	switch format {
	case "json":
		comments, err := c.Comments(ctx, group)
		if err != nil {
			return err
		}
		if !commentsResolved {
			open := comments[:0]
			for _, cm := range comments {
				if !cm.Resolved {
					open = append(open, cm)
				}
			}
			comments = open
		}
		if err := jsonEncoder(os.Stdout).Encode(comments); err != nil {
			return err
		}
		if commentsExitCode {
			return exitReview(ctx, c, group)
		}
		return nil
	case "prompt", "markdown":
		text, err := c.Prompt(ctx, group, commentsResolved, format == "prompt")
		if err != nil {
			return err
		}
		fmt.Print(text)
		if commentsExitCode {
			return exitReview(ctx, c, group)
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q: use prompt, markdown or json", format)
	}
}

// checkClearFlags rejects the two ways of asking --clear for something it
// cannot do. Both used to be accepted and then ignored, which is the worse
// answer when the subject is deleting comments: --resolved-only without
// --clear reads as "only print the resolved ones" and would have printed
// everything, and --include-resolved with --clear reads as "clear those too"
// when clearing already takes them.
func checkClearFlags() error {
	if commentsResolvedOnly && !commentsClear {
		return fmt.Errorf("--resolved-only only says what to remove: use it with --clear, or --include-resolved to print the resolved comments")
	}
	if commentsClear && commentsResolved {
		return fmt.Errorf("--include-resolved does nothing with --clear, which removes resolved comments anyway: --resolved-only removes only those")
	}
	return nil
}
