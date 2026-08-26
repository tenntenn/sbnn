package version

import (
	"os"
	"os/exec"
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

// tagprSetting reads one key out of .tagpr, which is git-config format: a
// [tagpr] section header and then indented "key = value" lines. The second
// return says whether the key is there at all, because an absent key and a key
// set to "-" mean opposite things to tagpr.
func tagprSetting(t *testing.T, key string) (string, bool) {
	t.Helper()
	for line := range strings.SplitSeq(readRepoFile(t, ".tagpr"), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		return strings.TrimSpace(v), true
	}
	return "", false
}

// #332 settled that the git tag is the only authority on the version: a
// release binary gets the tag from GoReleaser's -X, `go install ...@v1.2.3`
// reads it out of the build info, and anything else is a source build that
// reports "dev". Nothing is supposed to write a version number into the tree.
//
// tagpr disagrees by default, and the two ways it can disagree are both live:
//
//   - Pointed at a file, it rewrites it. `bumpVersionFile` (tagpr v1.20.1
//     versionfile.go:179) replaces the first match of `(v|\b)<current>\b`, and
//     `retrieveVersionFromFile` (:199) falls back to the first bare
//     `\d+\.\d+\.\d+` in the file and, at tagpr.go:756, lets that value
//     *override the version being released*. Aimed at version/version.go those
//     find the doc comments this package grew in #332: the pseudo-version
//     example becomes "v0.0.1-20260826015029-0b87768de8e9", and the
//     `go install ...@v1.2.3` example makes tagpr propose v1.2.3 as the next
//     release. Before #332 the file held no digits and the setting looked
//     inert, which is why the release pull request that has already been
//     raised left it untouched.
//
//   - With no versionFile key at all, `cfg.versionFile` is nil, so tagpr.go:480
//     runs detectVersionFile(".", currVer), which in this repository picks
//     web/package.json ("version": "0.0.0"), rewrites it, and writes the path
//     it chose back into .tagpr. Deleting the line is therefore worse than
//     leaving it wrong.
//
// "-" is tagpr's own word for "git tags only" (its README: "If you do not want
// to use versioning files but only git tags, specify the '-' string here"), and
// it is the only value that leaves the tree alone.
func TestTagprWritesNoVersionFile(t *testing.T) {
	value, ok := tagprSetting(t, "versionFile")
	if !ok {
		t.Fatal(`.tagpr sets no versionFile, so tagpr picks one itself: ` +
			`it would rewrite web/package.json and record that choice back into .tagpr. ` +
			`Set "versionFile = -" to keep the git tag the only authority on the version.`)
	}
	if value != "-" {
		t.Errorf(`.tagpr says versionFile = %s; want "-".`+"\n"+
			`Any real path makes tagpr write a release number into the tree, which is what `+
			`version.versionFrom exists to stop a source build from claiming.`, value)
	}
}

// `tsc -b` rewrites its incremental cache on every `pnpm run build`, so a
// tracked .tsbuildinfo makes the working tree dirty every time anyone builds
// the review page. It carries nothing another checkout can use - timestamps
// and file hashes from the machine that ran the build - so it has to be
// ignored, and it has to be untracked as well: an ignore rule does nothing
// for a file git already follows.
func TestTheTypeScriptBuildCacheIsNotTracked(t *testing.T) {
	t.Run("ignored", func(t *testing.T) {
		if !strings.Contains(readRepoFile(t, ".gitignore"), ".tsbuildinfo") {
			t.Error(".gitignore does not ignore tsc's .tsbuildinfo cache, so every build dirties the tree")
		}
	})
	t.Run("untracked", func(t *testing.T) {
		out, err := exec.Command("git", "-C", "..", "ls-files").Output()
		if err != nil {
			t.Skipf("git ls-files: %v", err)
		}
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.HasSuffix(line, ".tsbuildinfo") {
				t.Errorf("%s is tracked; ignoring it is not enough once git already follows it", line)
			}
		}
	})
}
