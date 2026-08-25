package version

import "runtime/debug"

// revisionFrom picks the commit out of the build info, and returns fallback
// when the build carries no VCS stamp.
//
// Anything short of a non-empty vcs.revision leaves fallback untouched: an
// empty Revision would be dropped entirely by the omitempty tag on the status
// payload, and a build linked with -X ...Revision=<commit> must keep the value
// the linker put there when the build info has nothing better to offer.
func revisionFrom(bi *debug.BuildInfo, ok bool, fallback string) string {
	if !ok || bi == nil {
		return fallback
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	return fallback
}

func init() {
	bi, ok := debug.ReadBuildInfo()
	Revision = revisionFrom(bi, ok, Revision)
}
