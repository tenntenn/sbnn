package a

import (
	"context"
	"os/exec"
)

const gitBin = "git"

// runsGit shells out to git, which sbnn must never do.
func runsGit() {
	exec.Command("git", "diff") // want `sbnn must not run git: exec.Command\("git"\)`
}

// runsGitByPath names the binary by an absolute path.
func runsGitByPath() {
	exec.Command("/usr/bin/git", "diff") // want `sbnn must not run git: exec.Command\("/usr/bin/git"\)`
}

// runsGitOnWindows names the Windows executable.
func runsGitOnWindows() {
	exec.Command(`C:\Program Files\Git\git.exe`, "diff") // want `sbnn must not run git`
}

// runsGitWithContext uses the context form, where the name is the second
// argument.
func runsGitWithContext(ctx context.Context) {
	exec.CommandContext(ctx, "git", "diff") // want `sbnn must not run git: exec.CommandContext\("git"\)`
}

// runsGitViaConstant hides the name behind a constant, which still resolves.
func runsGitViaConstant() {
	exec.Command(gitBin, "diff") // want `sbnn must not run git: exec.Command\("git"\)`
}

// looksForGit only asks whether git is installed, which is the same
// dependency by another name.
func looksForGit() {
	exec.LookPath("git") // want `sbnn must not run git: exec.LookPath\("git"\)`
}

// runsSomethingElse is what the rest of sbnn does: it runs its own binary,
// or the tool a hook names.
func runsSomethingElse(bin string) {
	exec.Command(bin, "--foreground")
	exec.Command("go", "build")
	exec.LookPath("go")
}

// namesGitElsewhere mentions git in an argument rather than as the binary.
func namesGitElsewhere() {
	exec.Command("sh", "-c", "git diff")
	exec.Command("gitk")
	exec.Command("legit")
}

// buildsTheNameAtRunTime is out of reach for a constant check, and is not
// reported.
func buildsTheNameAtRunTime(dir string) {
	exec.Command(dir+"/git", "diff")
}
