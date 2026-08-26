package web

import (
	"os"
	"strings"
	"testing"
)

// An image that is part of the diff had no size cap at all, so internal/export
// froze a 32MiB PNG into a page meant to be mailed around and made it 45MB
// (#323). The cap it is held to now is internal/asset's, the same one a
// sibling image of a Markdown preview has had since #305, and the verdict is
// decided in Go and travels on the file as imageStatus.
//
// That is what makes the live page and an exported page agree, and it only
// holds while the section actually reads it: a live page fetches the bytes
// from an endpoint and would happily draw whatever the browser downloads, so
// an <img> rendered without consulting the verdict puts the picture back on
// screen while the exported page shows a plate. Neither the TypeScript nor
// the Go can state that pairing alone, which is why it is checked here.
//
// Every top level name here starts with "diffImageCap" on purpose: the other
// tests in this package are merged separately and must not collide over a
// shared helper name.

const diffImageCapSection = "src/components/PreviewFileSection.tsx"

func diffImageCapSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(diffImageCapSection)
	if err != nil {
		t.Fatalf("read %s: %v", diffImageCapSection, err)
	}
	return string(b)
}

func TestDiffImageCapIsReadBeforeThePictureIsDrawn(t *testing.T) {
	src := diffImageCapSource(t)
	if !strings.Contains(src, "file.imageStatus") {
		t.Errorf("%s never reads file.imageStatus, so it draws an image of the diff whatever it weighs "+
			"while an exported page shows a placeholder for the same file", diffImageCapSection)
	}
	if !strings.Contains(src, "assetTrouble(") {
		t.Errorf("%s does not word the refusal with assetTrouble, so an image of the diff and a sibling image "+
			"of a preview would be refused in different words for the same reason", diffImageCapSection)
	}
}

// The plate is the one the sibling images use, so it inherits their styles
// and reads the same way. A class of its own would be a second design for one
// thing.
func TestDiffImageCapUsesTheSamePlateAsASiblingImage(t *testing.T) {
	src := diffImageCapSource(t)
	for _, class := range []string{"preview-asset-missing", "preview-asset-name", "preview-asset-why"} {
		if !strings.Contains(src, class) {
			t.Errorf("%s does not use %q, which is what a sibling image that did not fit is drawn as",
				diffImageCapSection, class)
		}
	}
}
