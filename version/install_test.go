package version

import (
	"runtime/debug"
	"testing"
)

func TestGoInstalledFrom(t *testing.T) {
	for _, tt := range []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want bool
	}{
		{
			"go install from the module proxy",
			&debug.BuildInfo{Main: debug.Module{Path: sbnnModule, Version: "v1.2.3", Sum: "h1:abc="}},
			true,
			true,
		},
		{
			"GoReleaser build of a tagged checkout",
			&debug.BuildInfo{Main: debug.Module{Path: sbnnModule, Version: "v1.2.3"}},
			true,
			false,
		},
		{
			"source build",
			&debug.BuildInfo{Main: debug.Module{Path: sbnnModule, Version: "(devel)"}},
			true,
			false,
		},
		{
			"sbnn built as a dependency of another module",
			&debug.BuildInfo{Main: debug.Module{Path: "example.com/other", Version: "v0.1.0", Sum: "h1:abc="}},
			true,
			false,
		},
		{
			"no build info",
			&debug.BuildInfo{Main: debug.Module{Path: sbnnModule, Sum: "h1:abc="}},
			false,
			false,
		},
		{
			"nil build info",
			nil,
			true,
			false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := goInstalledFrom(tt.bi, tt.ok); got != tt.want {
				t.Errorf("goInstalledFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}
