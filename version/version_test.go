package version

import (
	"runtime/debug"
	"testing"
)

func TestRevisionFrom(t *testing.T) {
	const fallback = "HEAD"
	for _, tt := range []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want string
	}{
		{
			"vcs.revision is used",
			&debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
			}},
			true,
			"abc123",
		},
		{
			"no vcs.revision keeps the fallback",
			&debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2026-08-25T06:39:29Z"},
			}},
			true,
			fallback,
		},
		{
			"empty vcs.revision keeps the fallback",
			&debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: ""},
			}},
			true,
			fallback,
		},
		{
			"no build info keeps the fallback",
			&debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
			}},
			false,
			fallback,
		},
		{
			"nil build info keeps the fallback",
			nil,
			true,
			fallback,
		},
		{
			"no settings at all keeps the fallback",
			&debug.BuildInfo{},
			true,
			fallback,
		},
		{
			"only vcs.revision is picked out of the other settings",
			&debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2026-08-25T06:39:29Z"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.revision", Value: "def456"},
				{Key: "-tags", Value: "netgo"},
			}},
			true,
			"def456",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := revisionFrom(tt.bi, tt.ok, fallback); got != tt.want {
				t.Errorf("revisionFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRevisionIsSet only checks that init left something behind: the actual
// commit depends on how the test binary was built, so asserting a value here
// would make the test environment-dependent.
func TestRevisionIsSet(t *testing.T) {
	if Revision == "" {
		t.Error("Revision is empty; it must keep its fallback when the build carries no VCS stamp")
	}
}

// A release binary has to say which release it is. There are two ways it can
// know, because there are two ways to get one: the releases page hands out a
// GoReleaser build, which is a compile of a checkout and carries the tag at
// link time; `go install ...@v1.2.3` compiles the module itself and carries
// no linker flags at all, so the module version in the build info is the only
// place the tag is written down. Reading only the linker flag reported "dev"
// for the whole second half of that.
func TestVersionFrom(t *testing.T) {
	const fallback = "dev"
	main := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/tenntenn/sbnn", Version: v}}
	}
	for _, tt := range []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want string
	}{
		{"a module version is the release", main("v1.2.3"), true, "1.2.3"},
		{"a prerelease keeps everything after the v", main("v1.2.3-rc.1"), true, "1.2.3-rc.1"},
		{"a build from a checkout is not a release", main("(devel)"), true, fallback},
		{"no module version at all keeps the fallback", main(""), true, fallback},
		// `go install ...@latest` on a repository with no reachable tag.
		{"a pseudo-version names a commit, not a release",
			main("v0.0.0-20260826015029-0b87768de8e9"), true, fallback},
		{"a pseudo-version after a tag is still one",
			main("v1.2.4-0.20260826015029-0b87768de8e9"), true, fallback},
		// The same shape with something else on the end is a real
		// prerelease and has to survive.
		{"a prerelease that merely looks like one is kept",
			main("v1.2.4-20260826015029-0b87768de8e9.1"), true, "1.2.4-20260826015029-0b87768de8e9.1"},
		// Since Go 1.24 a plain `go build` of a checkout is stamped with a
		// pseudo-version rather than left at "(devel)", and a modified
		// tree gets +dirty on the end of it. Measured on this repository:
		// the binary from `go build .` reported
		// v0.0.0-20260826015029-0b87768de8e9+dirty.
		{"a dirty pseudo-version is still not a release",
			main("v0.0.0-20260826015029-0b87768de8e9+dirty"), true, fallback},
		{"a dirty build of a real release still names it",
			main("v1.2.3+dirty"), true, "1.2.3+dirty"},
		{"an incompatible major keeps its metadata", main("v2.0.0+incompatible"), true, "2.0.0+incompatible"},
		{"no build info keeps what the linker set", main("v1.2.3"), false, fallback},
		{"nil build info keeps what the linker set", nil, true, fallback},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionFrom(tt.bi, tt.ok, fallback); got != tt.want {
				t.Errorf("versionFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A GoReleaser build passes the tag with -X, and the build info it would
// otherwise read says "(devel)": what the linker set has to survive that, or
// every binary on the releases page reports "dev".
func TestVersionFromKeepsWhatTheLinkerSet(t *testing.T) {
	const linked = "1.2.3"
	bi := &debug.BuildInfo{Main: debug.Module{Path: "github.com/tenntenn/sbnn", Version: "(devel)"}}
	if got := versionFrom(bi, true, linked); got != linked {
		t.Errorf("versionFrom() = %q, want the linked %q", got, linked)
	}
}

// TestVersionIsSet only checks that init left something behind: what it is
// depends on how the test binary was built.
func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Error("Version is empty; it must keep its fallback when the build carries no module version")
	}
}
