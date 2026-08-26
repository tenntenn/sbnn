package doccheck

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/screenshot.png is the first thing anyone sees of sbnn, and for a while
// it showed a UI the released binary did not have: it had been taken against a
// bundle built locally from web/src while the binary serves the committed
// web/dist. 468,411 pixels on 894 rows apart, and nothing in the repository
// noticed - docs/doccheck read the README's prose and not its images, and
// test/visual takes no shot to compare against.
//
// docs/screenshot.md writes the rule down. These hold it. They deliberately do
// not compare pixels; docs/screenshot.md's last section says why, and the short
// version is that a pixel comparison needs a browser, a tolerance and a CI job
// that test/visual does not have yet (#317).

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{repoRoot(t)}, parts...)...))
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Join(parts...), err)
	}
	return string(b)
}

// The conditions #313 settled on, and the one part of them a file on disk can
// still be asked about afterwards. A shot at the wrong viewport, or one taken
// at deviceScaleFactor 2 (which would be 3200x1900), is a shot that cannot be
// compared against the picture it replaces.
const (
	shotWidth  = 1600
	shotHeight = 950
)

func TestScreenshotIsTheDocumentedSize(t *testing.T) {
	f, err := os.Open(filepath.Join(repoRoot(t), "docs", "screenshot.png"))
	if err != nil {
		t.Fatalf("opening docs/screenshot.png: %v", err)
	}
	defer f.Close()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decoding docs/screenshot.png: %v", err)
	}
	if format != "png" {
		t.Errorf("docs/screenshot.png is a %s, not a png", format)
	}
	if cfg.Width != shotWidth || cfg.Height != shotHeight {
		t.Errorf("docs/screenshot.png is %dx%d; docs/screenshot.md says %dx%d at deviceScaleFactor 1.\n"+
			"A shot at another size cannot be compared against the one it replaces.",
			cfg.Width, cfg.Height, shotWidth, shotHeight)
	}
}

// shotStrings reads the "What the picture shows" table out of
// docs/screenshot.md: the backticked value in each row's first cell.
func shotStrings(t *testing.T, doc string) []string {
	t.Helper()
	const head = "## What the picture shows\n"
	_, rest, ok := strings.Cut(doc, head)
	if !ok {
		t.Fatal(`docs/screenshot.md has no "What the picture shows" section`)
	}
	if before, _, ok := strings.Cut(rest, "\n## "); ok {
		rest = before
	}
	cell := regexp.MustCompile("(?m)^\\|\\s*`([^`]+)`\\s*\\|")
	var out []string
	for _, m := range cell.FindAllStringSubmatch(rest, -1) {
		out = append(out, m[1])
	}
	return out
}

// bundle is every asset the binary actually serves: web/dist as committed.
func bundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "web", "dist")
	var b strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".js", ".css", ".html":
			c, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.Write(c)
			b.WriteByte('\n')
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading web/dist: %v", err)
	}
	if b.Len() == 0 {
		t.Fatal("web/dist holds no js, css or html; this test is looking in the wrong place")
	}
	return b.String()
}

// TestScreenshotShowsWhatTheBundleCanPaint is the guard the drift needed.
//
// The strings are read off the picture by whoever takes it and written into
// docs/screenshot.md. If they took it from a dev server running ahead of the
// bundle, the labels they record are the new ones and the committed web/dist
// cannot paint them, which is what this reports. Checked against the bundle at
// c17d3a3 - the commit whose picture was wrong - web/dist held "Filter paths"
// and no "Search paths and lines", so this goes red exactly there.
func TestScreenshotShowsWhatTheBundleCanPaint(t *testing.T) {
	doc := repoFile(t, "docs", "screenshot.md")
	shipped := bundle(t)

	strs := shotStrings(t, doc)
	if len(strs) < 5 {
		t.Fatalf("read %d strings out of docs/screenshot.md; the table must have moved", len(strs))
	}
	for _, s := range strs {
		t.Run(s, func(t *testing.T) {
			if !strings.Contains(shipped, s) {
				t.Errorf("docs/screenshot.md says the picture shows %q, and the committed web/dist cannot paint it.\n"+
					"Either the picture was taken from a bundle that is not the committed one - which is the thing "+
					"docs/screenshot.md forbids - or web/dist moved and the picture was not retaken with it.", s)
			}
		})
	}
}

// The rule is only useful where it is found. The README is where the picture
// is used, so it has to point at the file that says how the picture is made.
func TestREADMEPointsAtTheScreenshotRule(t *testing.T) {
	readme := readREADME(t)
	if !strings.Contains(readme, "docs/screenshot.png") {
		t.Fatal("README.md no longer shows docs/screenshot.png; this test needs rewriting")
	}
	if !strings.Contains(readme, "docs/screenshot.md") {
		t.Error("README.md shows docs/screenshot.png and never names docs/screenshot.md,\n" +
			"so whoever retakes the picture has no way to find the conditions it has to be taken under.")
	}
}
