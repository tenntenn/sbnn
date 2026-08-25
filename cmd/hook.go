package cmd

import (
	"context"
	"errors"
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
	hookRemove  string
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
  $ sbnn hook --remove h2           # forget one of them
  $ sbnn hook --clear               # forget it

The command runs through the shell with the review prompt on its stdin and
these variables set: SBNN_GROUP, SBNN_URL, SBNN_SERVER, SBNN_PORT, SBNN_COMMENTS,
SBNN_REVIEW_NOTE, SBNN_VERDICT and SBNN_BLOCKING. The URL is sent the same
thing as JSON, where the verdict is the "verdict" field.

SBNN_VERDICT is what the reviewer decided - approved, commented or
changes-requested - and is empty when the review carried no verdict, so a
hook that wants a default picks its own.

SBNN_BLOCKING answers the question a hook would otherwise re-implement: it
is 1 when the review stops the change going ahead and 0 when it does not.
It is the same answer sbnn wait --exit-code and sbnn submit --exit-code end
on, so a hook and a pipeline reading the exit status never disagree. An
approval is not blocking however many remarks it carries, changes-requested
always is, and a review that only commented blocks while it still has an
open comment.

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
	f.StringVar(&hookRemove, "remove", "", "Remove one hook by ID (sbnn hook lists the IDs)")
	f.BoolVar(&jsonOutput, "json", false, "Print structured JSON on stdout")
	// A removal and a registration asked for in the same command disagree
	// about what the command is for, and the switch in runHook would settle
	// it by order: the removal happens, the new hook is dropped without a
	// word, and the user is left believing it is registered. --clear had
	// the same trouble. Refusing the combinations says so once. The flags
	// are paired one by one rather than put in a single group of four: a
	// hook may carry a command and a URL at once, and one group would
	// forbid that too.
	hookCmd.MarkFlagsMutuallyExclusive("remove", "clear")
	hookCmd.MarkFlagsMutuallyExclusive("remove", "on-review")
	hookCmd.MarkFlagsMutuallyExclusive("remove", "on-review-url")
	hookCmd.MarkFlagsMutuallyExclusive("clear", "on-review")
	hookCmd.MarkFlagsMutuallyExclusive("clear", "on-review-url")
}

// hookAction is what a run of "sbnn hook" turns out to be.
type hookAction int

const (
	hookList hookAction = iota
	hookRemoveOne
	hookClearAll
	hookAdd
)

// hookActionFor reads the flags rather than the values they hold, because a
// flag that was given an empty value - "sbnn hook --remove $HOOK_ID" with
// HOOK_ID unset is the way it happens - is indistinguishable from an absent
// flag by value alone. Deciding by value sent that run down the default
// branch, so it printed the hook list and exited 0 while the user believed a
// hook had been removed. An empty value is a mistake worth saying out loud.
func hookActionFor(cmd *cobra.Command) (hookAction, error) {
	fs := cmd.Flags()
	switch {
	case fs.Changed("remove"):
		if hookRemove == "" {
			return hookList, errors.New("--remove needs a hook ID: sbnn hook lists them")
		}
		return hookRemoveOne, nil
	case hookClear:
		return hookClearAll, nil
	// A hook may carry a command, a URL, or both, so emptiness is only a
	// mistake when everything that was given is empty.
	case fs.Changed("on-review") || fs.Changed("on-review-url"):
		if hookCommand == "" && hookURL == "" {
			return hookList, errors.New("--on-review and --on-review-url need something to run")
		}
		return hookAdd, nil
	default:
		return hookList, nil
	}
}

func runHook(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	// The flags are settled before the server is contacted so that a
	// mistyped command is answered by the mistake, not by whether a server
	// happens to be running.
	action, err := hookActionFor(cmd)
	if err != nil {
		return err
	}
	group, err := groupName(target)
	if err != nil {
		return err
	}
	c := client.New(addr(), 5*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}

	switch action {
	case hookRemoveOne:
		removed, err := c.DeleteHook(ctx, group, hookRemove)
		if err != nil {
			return err
		}
		// The server answers an unknown ID with a count of 0 rather than
		// an error, so without this the user is told a hook went that was
		// never there.
		if removed == 0 {
			return fmt.Errorf("no hook %q on group %q", hookRemove, group)
		}
		fmt.Fprintf(os.Stderr, "sbnn: removed hook %q from group %q\n", hookRemove, group)
		return nil
	case hookClearAll:
		removed, err := c.DeleteHooks(ctx, group)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "sbnn: removed %d hook(s) from group %q\n", removed, group)
		return nil
	case hookAdd:
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
