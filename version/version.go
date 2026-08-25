// Package version carries the build information of sbnn.
package version

// Version is the sbnn version, overridden at release time.
var Version = "dev"

// Revision is the commit sbnn was built from. It is read from the build info
// at startup (see revision.go), and may also be set at link time with
// -X github.com/tenntenn/sbnn/version.Revision=<commit>.
var Revision = "HEAD"
