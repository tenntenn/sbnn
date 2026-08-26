package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// web/test holds the tests for the parts of the review UI no Go test can
// reach: the word-level diff's grapheme splitting, the suggestion parser, the
// search matcher, the shortcut table, the exported page's static client. They
// went unrun on every pull request for as long as the `web` CI job was
// install-and-build, and a green `ci / web` read as "the web changes were
// checked" when what was checked was that TypeScript compiles (#317).
//
// Building is not running: `pnpm run build` is `tsc -b && vite build`, and a
// test that asserts the wrong answer type-checks perfectly.

func repoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(b)
}

func TestTheWebTestSuiteIsWired(t *testing.T) {
	// The script the other two assertions point at. Without it "pnpm test"
	// is a pnpm error, not a test run.
	t.Run("package.json defines it", func(t *testing.T) {
		if !strings.Contains(repoFile(t, "web/package.json"), `"test":`) {
			t.Error("web/package.json defines no test script")
		}
	})

	t.Run("CI runs it", func(t *testing.T) {
		ci := repoFile(t, ".github/workflows/ci.yml")
		if !strings.Contains(ci, "run: pnpm test") {
			t.Error("the web job of .github/workflows/ci.yml never runs the web test suite, " +
				"so nothing under web/test can fail a pull request")
		}
	})

	// `task test` is what CONTRIBUTING.md tells a contributor to run before
	// pushing, so it has to mean what CI means. It delegates rather than
	// spelling out the commands, which is why this looks for the task name.
	t.Run("task test covers it", func(t *testing.T) {
		if !strings.Contains(repoFile(t, "Taskfile.yml"), "task: test-web") {
			t.Error("`task test` does not run the web suite, so it means less locally than it does in CI")
		}
	})
}

// Every file the runner is pointed at has to be one it picks up. The script
// globs test/*.test.ts, so a test parked one directory down, or named without
// the .test.ts suffix, is a file nobody runs and nobody notices.
func TestEveryWebTestFileIsPickedUpByTheRunner(t *testing.T) {
	script := repoFile(t, "web/package.json")
	if !strings.Contains(script, "test/*.test.ts") {
		t.Skip("the test script no longer globs test/*.test.ts; this test describes that glob")
	}
	entries, err := os.ReadDir("test")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			t.Errorf("web/test/%s is a directory, and the runner only globs test/*.test.ts, so nothing in it runs", name)
			continue
		}
		if strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".mjs") {
			continue
		}
		t.Errorf("web/test/%s is neither a *.test.ts the runner globs nor a helper, so it is a test nobody runs", name)
	}
}
