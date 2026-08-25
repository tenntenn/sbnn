package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/model"
)

var (
	hookCommand string
	hookURL     string
	hookClear   bool
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Run something when the review is submitted",
	Long: `List, add or drop what sbnn does when a review is submitted.

A review often lands long after whoever asked for it stopped waiting: the
reviewer was in a meeting, the agent session timed out. A hook is the way
round that - the sbnn server starts the work when the human presses "Submit
review", with nobody waiting in between.

  $ sbnn hook --on-review 'claude -p "$(sbnn comments)"'
  $ sbnn hook --on-review-url http://localhost:9000/reviews
  $ sbnn hook                       # what is registered
  $ sbnn hook --clear               # forget it

The command runs through the shell with the review prompt on its stdin and
these variables set: SBNN_GROUP, SBNN_URL, SBNN_SERVER, SBNN_PORT, SBNN_COMMENTS,
SBNN_REVIEW_NOTE, SBNN_VERDICT and SBNN_BLOCKING. The URL is sent the same
thing as JSON, where the verdict is the "verdict" field.

SBNN_VERDICT is what the reviewer decided - approved, commented or
changes-requested - and is empty when the review carried no verdict, so a
hook that wants a default picks its own. SBNN_BLOCKING answers the question
a hook would otherwise re-implement: it is 1 when the verdict stops the
change going ahead and 0 when it does not.

  $ sbnn hook --on-review '[ "$SBNN_BLOCKING" = 1 ] && notify-send "changes requested"'

Hooks belong to a group, survive a restart, and can also be registered while
sending the diff:

  $ git diff | sbnn --target api --on-review 'notify-send "review is in"'

A hook runs a command of your own on your own machine, which is exactly what
makes it useful and worth being deliberate about.`,
	Args:         cobra.NoArgs,
	RunE:         runHook,
	SilenceUsage: true,
}

func init() {
	f := hookCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVar(&hookCommand, "on-review", "", "Shell command to run when a review is submitted")
	f.StringVar(&hookURL, "on-review-url", "", "URL to POST to when a review is submitted")
	f.BoolVar(&hookClear, "clear", false, "Remove the hooks of the group")
	f.BoolVar(&jsonOutput, "json", false, "Print structured JSON on stdout")
}

func runHook(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}
	c := client.New(addr(), 5*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}

	switch {
	case hookClear:
		removed, err := c.DeleteHooks(ctx, group)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "sbnn: removed %d hook(s) from group %q\n", removed, group)
		return nil
	case hookCommand != "" || hookURL != "":
		added, err := c.AddHook(ctx, group, model.Hook{Command: hookCommand, URL: hookURL})
		if err != nil {
			return err
		}
		if jsonOutput {
			return jsonEncoder(os.Stdout).Encode(added)
		}
		fmt.Fprintf(os.Stderr, "sbnn: %s will run when the review of %q is submitted\n",
			describeHook(added), group)
		return nil
	default:
		return listHooks(ctx, c, group)
	}
}

func listHooks(ctx context.Context, c *client.Client, group string) error {
	hooks, err := c.Hooks(ctx, group)
	if err != nil {
		return err
	}
	if jsonOutput {
		return jsonEncoder(os.Stdout).Encode(hooks)
	}
	if len(hooks) == 0 {
		fmt.Printf("no hook on group %q\n", group)
		return nil
	}
	for _, h := range hooks {
		fmt.Printf("%s  %s\n", h.ID, describeHook(h))
	}
	return nil
}

func describeHook(h *model.Hook) string {
	switch {
	case h.Command != "" && h.URL != "":
		return fmt.Sprintf("%s (and POST %s)", h.Command, h.URL)
	case h.URL != "":
		return "POST " + h.URL
	default:
		return h.Command
	}
}

// registerHooks is called when a diff is sent with --on-review, so that the
// diff and the way back are set up in one command.
func registerHooks(ctx context.Context, c *client.Client, group string) error {
	if onReviewCommand == "" && onReviewURL == "" {
		return nil
	}
	added, err := c.AddHook(ctx, group, model.Hook{Command: onReviewCommand, URL: onReviewURL})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sbnn: %s will run when the review of %q is submitted\n",
		describeHook(added), group)
	return nil
}
