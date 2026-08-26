// Package nogit defines an Analyzer that reports sbnn shelling out to git.
//
// sbnn never runs git. A diff reaches it on stdin and nowhere else, which is
// what lets it review a diff no working tree can produce any more - one
// pasted out of a mail, one built by another tool, one whose repository is
// not on this machine. The rule is written down in AGENTS.md, and a single
// exec.Command("git", ...) added in passing would quietly undo it, so it is
// checked here rather than left to review.
package nogit

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

const doc = `nogit reports calls that run the git binary

sbnn takes diffs from stdin only; it must never shell out to git. The
analyzer reports a call to os/exec.Command, exec.CommandContext or
exec.LookPath whose command name is a constant naming the git binary -
"git", "/usr/bin/git", "git.exe" and so on.`

// Analyzer reports calls that run the git binary.
var Analyzer = &analysis.Analyzer{
	Name:     "nogit",
	Doc:      doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// nameArg maps each os/exec function that takes a binary name to the index
// of that argument.
var nameArg = map[string]int{
	"Command":        0,
	"CommandContext": 1,
	"LookPath":       0,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "os/exec" {
			return
		}
		i, ok := nameArg[fn.Name()]
		if !ok || len(call.Args) <= i {
			return
		}

		arg := call.Args[i]
		name, ok := constantString(pass, arg)
		if !ok || !isGit(name) {
			return
		}
		pass.Reportf(arg.Pos(), "sbnn must not run git: exec.%s(%q) - a diff only ever arrives on stdin (see AGENTS.md)", fn.Name(), name)
	})

	return nil, nil
}

// constantString returns the value of expr when it is a constant string. A
// name built at run time is not reported: what can be checked is the literal
// spelling, and that is the one a change would introduce.
func constantString(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[ast.Unparen(expr)]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// isGit reports whether name refers to the git binary, whether it is bare
// ("git"), a path ("/usr/bin/git") or a Windows executable ("git.exe").
func isGit(name string) bool {
	base := path.Base(strings.ReplaceAll(name, `\`, "/"))
	return strings.TrimSuffix(base, ".exe") == "git"
}
