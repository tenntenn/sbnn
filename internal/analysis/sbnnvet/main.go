// Command sbnnvet is the vet tool for this repository: sbnn's own analyzer
// plus the golang.org/x/tools analyzers that go vet does not run by default.
//
// task lint runs it, so it needs no installing:
//
//	go vet -vettool=$(go tool -n sbnnvet) ./...
//
// The tool directive in go.mod pins it, and go builds it with the toolchain
// go.mod asks for. That matters: a vet tool built with an older Go than the
// module it analyzes refuses the whole package graph with "package requires
// newer Go version".
//
// A single analyzer can be picked out while working on it:
//
//	go vet -vettool=$(go tool -n sbnnvet) -nogit ./...
package main

import (
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/unitchecker"

	"github.com/tenntenn/sbnn/internal/analysis/nogit"
)

func main() {
	unitchecker.Main(
		// sbnn's own rule: diffs come from stdin, never from git.
		nogit.Analyzer,
		// nilness reports a dereference or a branch that is nil on every
		// path - the mistake go vet's default set leaves to the compiler,
		// which does not catch it.
		nilness.Analyzer,
		// unusedwrite reports a write to a struct field or an array element
		// that nothing reads back, which is how a fix applied to a copy
		// instead of the original looks.
		unusedwrite.Analyzer,
	)
}
