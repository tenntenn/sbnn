package version

import (
	"os"
	"strings"
	"testing"
)

// The release configuration stamps this package by name, from a file that no
// compiler checks. Rename Version or move the package and the build still
// succeeds - every binary on the releases page just goes back to saying
// "dev", which is the failure #101 is about and the one nobody notices until
// a user reports their version wrong.
//
// These read the two files rather than mock anything: what is asserted is
// that they say what the code needs them to say.

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(b)
}

// modulePath is what go.mod calls this module, which is the prefix a -X flag
// has to name.
func modulePath(t *testing.T) string {
	t.Helper()
	for line := range strings.SplitSeq(readRepoFile(t, "go.mod"), "\n") {
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after)
		}
	}
	t.Fatal("go.mod names no module")
	return ""
}

func TestGoreleaserStampsThisPackage(t *testing.T) {
	config := readRepoFile(t, ".goreleaser.yml")
	mod := modulePath(t)
	for _, name := range []string{"Version", "Revision"} {
		t.Run(name, func(t *testing.T) {
			want := "-X " + mod + "/version." + name + "="
			if !strings.Contains(config, want) {
				t.Errorf(".goreleaser.yml does not stamp %s.\nwant a ldflag starting %q", name, want)
			}
		})
	}
}

// The six the README sends a reader to the releases page for.
func TestGoreleaserBuildsEveryPlatformTheReadmeOffers(t *testing.T) {
	config := readRepoFile(t, ".goreleaser.yml")
	for _, want := range []string{"darwin", "linux", "windows", "amd64", "arm64"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(config, "- "+want+"\n") {
				t.Errorf(".goreleaser.yml builds no %s", want)
			}
		})
	}
}

// Nothing builds the binaries unless a tag push runs the workflow. tagpr
// pushes the tag; this is what hangs off it.
func TestReleaseWorkflowRunsOnATagPush(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/release.yml")
	for _, want := range []string{`tags: ["v*"]`, "goreleaser/goreleaser-action@", "contents: write"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(workflow, want) {
				t.Errorf("the release workflow does not contain %q", want)
			}
		})
	}
}

// GoReleaser writes its archives into ./dist, which must never be committed -
// and web/dist, which is the built review page, must stay committed. One
// ignore rule has to say exactly that.
func TestOnlyTheRootDistIsIgnored(t *testing.T) {
	ignore := readRepoFile(t, ".gitignore")
	if !strings.Contains(ignore, "/dist/") {
		t.Error(".gitignore does not ignore the dist directory GoReleaser writes")
	}
	for line := range strings.SplitSeq(ignore, "\n") {
		if strings.TrimSpace(line) == "dist/" {
			t.Error("a bare dist/ rule would also ignore web/dist, which is committed on purpose")
		}
	}
}
