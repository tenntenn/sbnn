package version

import "runtime/debug"

// sbnnModule is the module a `go install` of sbnn builds as its main module.
const sbnnModule = "github.com/tenntenn/sbnn"

// GoInstalled reports whether this binary was built by `go install
// github.com/tenntenn/sbnn@<version>`.
//
// Such a binary belongs to the Go toolchain's GOBIN rather than to sbnn, and
// replacing it from the releases page would leave a file the toolchain no
// longer recognises; sbnn update tells its owner to run go install again
// instead. See goInstalledFrom for how it is told apart.
var GoInstalled bool

// goInstalledFrom tells a binary built from the module cache from every other
// kind of build.
//
// The module version alone cannot do it. Since Go 1.24 a `go build` of a
// checkout whose HEAD is tagged is stamped with that tag, so the GoReleaser
// build of v1.2.3 and `go install ...@v1.2.3` both report v1.2.3. What only
// the second one has is a checksum for the main module: the toolchain fills
// Main.Sum in when it builds a module it downloaded and verified, and leaves
// it empty for a module that is a directory on disk, which is what GoReleaser
// and every source build compile.
func goInstalledFrom(bi *debug.BuildInfo, ok bool) bool {
	if !ok || bi == nil {
		return false
	}
	return bi.Main.Path == sbnnModule && bi.Main.Sum != ""
}

func init() {
	bi, ok := debug.ReadBuildInfo()
	GoInstalled = goInstalledFrom(bi, ok)
}
