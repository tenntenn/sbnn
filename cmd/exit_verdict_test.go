package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// exitVerdictSpec names the case the helper process should run. Its presence
// is what tells that process it is the helper.
const exitVerdictSpec = "SBNN_TEST_EXIT_VERDICT"

// exitVerdictNotBlocking is what the helper ends with when exitWithVerdict
// returns rather than exiting. It is not 0 and not ExitOpenComments, so a
// helper that died for some other reason cannot be read as either answer.
const exitVerdictNotBlocking = 7

// TestExitVerdictHelperProcess is the child half of
// TestExitWithVerdictFollowsTheSameRuleAsSBNNBlocking. It runs
// exitWithVerdict for real, so the status under test is the status sbnn
// would leave a pipeline with, not a copy of the rule.
func TestExitVerdictHelperProcess(t *testing.T) {
	spec := os.Getenv(exitVerdictSpec)
	if spec == "" {
		t.Skip("not the helper process")
	}
	// spec is "<verdict>|<open>|<resolved>".
	parts := strings.Split(spec, "|")
	if len(parts) != 3 {
		t.Fatalf("bad spec %q", spec)
	}
	open, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := strconv.Atoi(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	var comments []*model.Comment
	for range open {
		comments = append(comments, &model.Comment{Body: "a remark"})
	}
	for range resolved {
		comments = append(comments, &model.Comment{Body: "a remark", Resolved: true})
	}

	if err := exitWithVerdict(model.Verdict(parts[0]), comments); err != nil {
		t.Fatalf("exitWithVerdict = %v", err)
	}
	os.Exit(exitVerdictNotBlocking)
}

// TestExitWithVerdictFollowsTheSameRuleAsSBNNBlocking is the tie between the
// two answers to one question: the status sbnn wait --exit-code and sbnn
// submit --exit-code end on, and the SBNN_BLOCKING a review hook is handed.
//
// SBNN_BLOCKING first shipped as Verdict.Blocking(), which is only the
// verdict's own half of the rule, and disagreed with sbnn for a review that
// commented and left a comment open: sbnn exits 1, the hook was told 0.
// Both now go through model.Blocks, and this runs the real exit path to
// check it really is the same answer.
func TestExitWithVerdictFollowsTheSameRuleAsSBNNBlocking(t *testing.T) {
	cases := []struct {
		name     string
		verdict  model.Verdict
		open     int
		resolved int
	}{
		{"approved", model.VerdictApproved, 0, 0},
		{"an approval with a remark on it", model.VerdictApproved, 1, 0},
		{"changes requested", model.VerdictChangesRequested, 0, 0},
		{"changes requested with nothing left open", model.VerdictChangesRequested, 0, 1},
		{"commented, with a comment still open", model.VerdictCommented, 1, 0},
		{"commented, everything resolved", model.VerdictCommented, 0, 1},
		{"commented, nothing said at all", model.VerdictCommented, 0, 0},
		{"no verdict, with a comment still open", "", 1, 0},
		{"no verdict, everything resolved", "", 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// string(), not %s: Verdict has a String() method that spells
			// changes-requested as "changes requested", which is prose, not
			// the value the wire and the env carry.
			spec := fmt.Sprintf("%s|%d|%d", string(tc.verdict), tc.open, tc.resolved)
			cmd := exec.Command(os.Args[0], "-test.run=^TestExitVerdictHelperProcess$")
			cmd.Env = append(os.Environ(), exitVerdictSpec+"="+spec)
			out, _ := cmd.CombinedOutput()
			code := cmd.ProcessState.ExitCode()

			var blocked bool
			switch code {
			case ExitOpenComments:
				blocked = true
			case exitVerdictNotBlocking:
				blocked = false
			default:
				t.Fatalf("the helper ended with %d, which is neither answer:\n%s", code, out)
			}

			// model.Blocks is what a hook is told through SBNN_BLOCKING.
			if want := model.Blocks(tc.verdict, commentsFor(tc.open, tc.resolved)); blocked != want {
				t.Errorf("sbnn exits %d (blocking=%v) but SBNN_BLOCKING would say %v",
					code, blocked, want)
			}
		})
	}
}

// commentsFor builds the comments a case describes.
func commentsFor(open, resolved int) []*model.Comment {
	var out []*model.Comment
	for range open {
		out = append(out, &model.Comment{Body: "a remark"})
	}
	for range resolved {
		out = append(out, &model.Comment{Body: "a remark", Resolved: true})
	}
	return out
}
