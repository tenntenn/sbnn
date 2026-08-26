package a

import (
	"context"
	"os/exec"
)

// runsGitInATest is not reported, and the absence of a "want" comment on
// these three lines is the assertion. A _test.go file is not compiled into
// sbnn, so a test may ask git about the checkout - version/release_test.go
// runs git ls-files to prove no .tsbuildinfo is tracked - without the
// reviewer gaining a dependency on git.
func runsGitInATest(ctx context.Context) {
	exec.Command("git", "ls-files")
	exec.CommandContext(ctx, "git", "ls-files")
	exec.LookPath("git")
}
