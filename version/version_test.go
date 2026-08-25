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
