package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// test/visual/README.md exists to say what each test.fail() annotation means
// and why it is there. Nothing read it, so it went stale exactly the way it
// was written to stop the specs going stale: #325 deleted three annotations
// and the document went on listing them as pinned defects, along with a colour
// the bundle had stopped painting.
//
// These read the two files against each other. They cannot check that the
// prose is true - that needs a browser - but they can check the one fact the
// document and the spec must agree on: which defects are pinned.

func readVisualFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "test", "visual", name))
	if err != nil {
		t.Fatalf("reading test/visual/%s: %v", name, err)
	}
	return string(b)
}

// issueRefs collects the #NNN references in a string, deduplicated and sorted.
var issueRef = regexp.MustCompile(`#([0-9]+)`)

func issueRefs(s string) []string {
	seen := map[string]bool{}
	for _, m := range issueRef.FindAllStringSubmatch(s, -1) {
		seen["#"+m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// testFail matches the arguments of a test.fail(...) call, across the line
// breaks a formatter puts in. Only the call is read, not the comment above it:
// a comment naturally cites the issues that were fixed and the ones that
// explain the history, and none of those are what is pinned.
var testFail = regexp.MustCompile(`(?s)test\.fail\((.*?)\)\n`)

// pinnedSection returns the body of the README's "Tests that are expected to
// fail" section, which is where a pinned defect is written down.
func pinnedSection(t *testing.T, doc string) string {
	t.Helper()
	const head = "## Tests that are expected to fail\n"
	_, rest, ok := strings.Cut(doc, head)
	if !ok {
		t.Fatal("test/visual/README.md has no \"Tests that are expected to fail\" section")
	}
	if before, _, ok := strings.Cut(rest, "\n## "); ok {
		return before
	}
	return rest
}

// guardSection returns the body of the README's "Tests that guard" section.
func guardSection(t *testing.T, doc string) string {
	t.Helper()
	const head = "## Tests that guard\n"
	_, rest, ok := strings.Cut(doc, head)
	if !ok {
		t.Fatal("test/visual/README.md has no \"Tests that guard\" section")
	}
	if before, _, ok := strings.Cut(rest, "\n## "); ok {
		return before
	}
	return rest
}

// TestVisualReadmePinsMatchTheSpec holds the two in step in both directions.
//
// One direction catches #325's accident: an annotation is deleted and the
// document keeps listing the issue as pinned. The other catches the opposite,
// an annotation added with nothing written down about what it means, which is
// the state the document was written to prevent.
func TestVisualReadmePinsMatchTheSpec(t *testing.T) {
	spec := readVisualFile(t, "geometry.spec.ts")
	pinned := pinnedSection(t, readVisualFile(t, "README.md"))

	var annotated []string
	for _, m := range testFail.FindAllStringSubmatch(spec, -1) {
		annotated = append(annotated, issueRefs(m[1])...)
	}

	documented := issueRefs(pinned)

	inSpec := map[string]bool{}
	for _, n := range annotated {
		inSpec[n] = true
	}
	inDoc := map[string]bool{}
	for _, n := range documented {
		inDoc[n] = true
	}

	for _, n := range documented {
		if !inSpec[n] {
			t.Errorf("test/visual/README.md lists %s as a pinned defect, but no test.fail() in geometry.spec.ts names it.\n"+
				"If the annotation was deleted, take the row out of the table and move what was measured into \"Tests that guard\".", n)
		}
	}
	for _, n := range annotated {
		if !inDoc[n] {
			t.Errorf("geometry.spec.ts has a test.fail() naming %s, and test/visual/README.md does not list it as a pinned defect.\n"+
				"A pin nobody wrote down is a pin the next reader deletes.", n)
		}
	}

	// A wrong count is the cheapest way for the two tables to disagree while
	// every issue number still lines up, so say what was found either way.
	t.Logf("test.fail() annotations cite %v; the README pins %v", annotated, documented)
}

// The two tables have to be talking about different tests. They stopped being
// so when #325 took the #74 annotation out: the pinned-defect table went on
// listing "the page does not scroll sideways" as pinned while the guard table
// listed the same test as guarding, so one test was in both tables at once and
// the document contradicted itself about whether the defect was open.
func TestVisualReadmeTablesDoNotOverlap(t *testing.T) {
	doc := readVisualFile(t, "README.md")
	pinned := issueRefs(pinnedSection(t, doc))
	guarding := map[string]bool{}
	for _, n := range issueRefs(guardSection(t, doc)) {
		guarding[n] = true
	}
	for _, n := range pinned {
		if guarding[n] {
			t.Errorf("test/visual/README.md lists %s in both \"Tests that are expected to fail\" and \"Tests that guard\".\n"+
				"A test is one or the other; carrying it in both is how the document came to say the defect was open and closed at once.", n)
		}
	}
}
