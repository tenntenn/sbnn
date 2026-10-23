package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/model"
)

var (
	submitNote     string
	submitVerdict  string
	submitApprove  bool
	submitChanges  bool
	submitQuiet    bool
	submitExitCode bool
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Finish the review of a group, as the Submit button does",
	Long: `Finish a review from the command line.

Pressing "Submit review" in the browser is what ends a round: it wakes
anything waiting on it, starts the hooks, and writes the round into the log
of past reviews. This does the same thing without a browser, which is what a
reviewer who is not a person needs.

  $ git diff | sbnn --target api                    # the change to look at
  $ sbnn comment main.go:42 -m "..." --author me    # what you found
  $ sbnn submit --target api --note "one thing to fix"

Say what you decided about the change as a whole, the way a review on a
pull request does. Counting comments does not answer it: a change can be
approved with three remarks on it, and can be sent back without a single
line being pointed at.

  $ sbnn submit --approve                     # go ahead
  $ sbnn submit                               # commented: said things, did not decide
  $ sbnn submit --request-changes -m "..."    # not as it is

A review with nothing in it is a review: submitting after finding nothing is
how the other side learns that, and it is the answer sbnn wait was waiting
for.

--exit-code turns the verdict into a status, so a pipeline can act on it
without reading anything: 1 when the review blocks the change, 0 when it
does not. A review that asked for changes blocks it; an approval does not,
whatever it said along the way; a plain "commented" blocks only if it left
something open to address.`,
	Args:         cobra.NoArgs,
	RunE:         runSubmit,
	SilenceUsage: true,
}

func init() {
	f := submitCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", 0, portFlagHelp)
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVarP(&submitNote, "note", "m", "", "What the review says as a whole")
	f.StringVar(&submitVerdict, "verdict", "",
		"approved, commented or changes-requested (default commented)")
	f.BoolVar(&submitApprove, "approve", false, "Shorthand for --verdict approved")
	f.BoolVar(&submitChanges, "request-changes", false, "Shorthand for --verdict changes-requested")
	submitCmd.MarkFlagsMutuallyExclusive("verdict", "approve", "request-changes")
	f.BoolVar(&jsonOutput, "json", false, "Print the reviewed group as JSON")
	f.BoolVar(&submitExitCode, "exit-code", false,
		"Exit 1 when the review blocks the change, 0 when it does not")
	f.BoolVarP(&submitQuiet, "quiet", "q", false, "Print nothing; implies --exit-code")
}

func runSubmit(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}
	verdict, err := chosenVerdict()
	if err != nil {
		return err
	}
	c, _, err := connect(ctx, cmd, 10*time.Second)
	if err != nil {
		return err
	}

	g, err := c.SubmitReview(ctx, group, submitNote, verdict)
	if err != nil {
		return err
	}

	open := 0
	for _, cm := range g.Comments {
		if !cm.Resolved {
			open++
		}
	}
	switch {
	case submitQuiet:
	case jsonOutput:
		if err := writeJSON(g); err != nil {
			return err
		}
	default:
		fmt.Fprintf(os.Stderr, "sbnn: %s %q, %d open comment(s)\n", g.ReviewVerdict.String(), group, open)
	}
	if submitQuiet || submitExitCode {
		return exitWithVerdict(g.ReviewVerdict, g.Comments)
	}
	return nil
}

// chosenVerdict reads the verdict from whichever spelling was used.
func chosenVerdict() (model.Verdict, error) {
	switch {
	case submitApprove:
		return model.VerdictApproved, nil
	case submitChanges:
		return model.VerdictChangesRequested, nil
	}
	v, ok := model.ParseVerdict(submitVerdict)
	if !ok {
		return "", fmt.Errorf("cannot read %q: use approved, commented or changes-requested", submitVerdict)
	}
	return v, nil
}
