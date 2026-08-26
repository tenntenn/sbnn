package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The sidebar path is truncated from the left, because the tail of a path is
// what identifies the file: "...onents/DiffFileSection.tsx" is useful and
// "web/src/components/DiffF..." is not. That comes from the box being RTL,
// and an RTL box also resolves every bidi-neutral character at either end of
// the string against RTL - so ".github/workflows/ci.yml" was painted as
// "github/workflows/ci.yml." (issue #73).
//
// The two halves have to be separated: the box keeps clipping from the left,
// and the string inside it is isolated so that it resolves its own direction.
// unicode-bidi: plaintext does both at once, and the half it takes away is
// the clipping - with it, the ellipsis moves to the end of the path and the
// file name is the part that disappears.
//
// This is a stylesheet-and-markup pairing that neither side can express
// alone, which is why it is checked here rather than in either file. Nothing
// in this test says what it looks like; the rendering itself was measured in
// a browser and is reported on the pull request.
//
// Every top level name here starts with "sidebarPath" on purpose: the other
// tests in this package are merged separately and must not collide over a
// shared helper name.

const (
	sidebarPathStylesheet = "src/styles.css"
	sidebarPathComponent  = "src/components/Sidebar.tsx"
)

func sidebarPathRule(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(sidebarPathStylesheet)
	if err != nil {
		t.Fatalf("read %s: %v", sidebarPathStylesheet, err)
	}
	m := regexp.MustCompile(`(?s)\n\.file-path\s*\{(.*?)\}`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("no .file-path rule in %s", sidebarPathStylesheet)
	}
	return m[1]
}

func TestSidebarPathClipsFromTheStart(t *testing.T) {
	rule := sidebarPathRule(t)
	for _, want := range []string{"direction: rtl", "text-overflow: ellipsis"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".file-path does not declare %q, so a truncated path loses its file name instead of its leading directories:\n%s",
				want, rule)
		}
	}
}

func TestSidebarPathDoesNotUsePlaintextBidi(t *testing.T) {
	rule := sidebarPathRule(t)
	if strings.Contains(rule, "unicode-bidi: plaintext") {
		t.Errorf(".file-path declares unicode-bidi: plaintext, which hands the box's inline direction back to the string as well as the string's own:\n"+
			"the ellipsis then lands at the end of the path and hides the file name. Isolate the string in the markup instead - see %s.\n%s",
			sidebarPathComponent, rule)
	}
}

func TestSidebarPathIsIsolatedInTheMarkup(t *testing.T) {
	b, err := os.ReadFile(sidebarPathComponent)
	if err != nil {
		t.Fatalf("read %s: %v", sidebarPathComponent, err)
	}
	src := string(b)
	span := regexp.MustCompile(`(?s)<span className="file-path".*?</span>`).FindString(src)
	if span == "" {
		t.Fatalf(`no <span className="file-path"> in %s`, sidebarPathComponent)
	}
	if !strings.Contains(span, "<bdi>") {
		t.Errorf("the path in %s is not wrapped in <bdi>, so the RTL box resolves its neutral characters against RTL and \".github/workflows/ci.yml\" is painted as \"github/workflows/ci.yml.\":\n%s",
			sidebarPathComponent, span)
	}
}
