package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Resolving a preview's relative links and looking the result up are two
// halves of one operation, and they live in two files. PreviewStack builds
// linkTargets keyed by the diff-relative path of every file of the review;
// PreviewFileSection resolves each relative href against a base and looks the
// answer up in that table. The two have to speak one language, and neither
// file can say so on its own - which is why this is checked here rather than
// in either of them, the same way the sidebar path pairing is.
//
// It went wrong the quiet way (#339). The base was `preview.path ||
// filePath(file)`, and preview.path is the file on disk the bytes were read
// from: absolute, and empty only when the content was rebuilt from the diff.
// So the correct half of that expression was reached exactly when the file
// was *missing* from the working tree, and the ordinary case - `git diff |
// sbnn` inside the repository the change was made in - resolved ./b.md into
// /home/you/repo/docs/b.md, which is never a key of linkTargets. A link
// between two files of the same review stopped being a link and was drawn as
// a dead span labelled with the reviewer's own absolute path.
//
// Measured in Chromium against the fixture of #339 before the fix:
//
//	OUTSIDE ["/…/linkrepo/docs/b.md - not part of this review", …]
//
// Every top level name here starts with "previewLinkBase" on purpose: the
// other tests in this package are merged separately and must not collide over
// a shared helper name.

const (
	previewLinkBaseSection = "src/components/PreviewFileSection.tsx"
	previewLinkBaseStack   = "src/components/PreviewStack.tsx"
)

func previewLinkBaseRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// previewLinkBaseCall is the resolvePreviewLinks(...) call, arguments and all.
var previewLinkBaseCall = regexp.MustCompile(`(?s)resolvePreviewLinks\((.*?)\)\s*:`)

func TestPreviewLinkBaseIsTheDiffRelativePath(t *testing.T) {
	src := previewLinkBaseRead(t, previewLinkBaseSection)
	m := previewLinkBaseCall.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("no resolvePreviewLinks(...) call in %s", previewLinkBaseSection)
	}
	args := m[1]
	if !strings.Contains(args, "filePath(file)") {
		t.Errorf("%s does not resolve preview links against filePath(file), which is what PreviewStack keys linkTargets by:\n%s",
			previewLinkBaseSection, args)
	}
	if strings.Contains(args, "preview.path") {
		t.Errorf("%s resolves preview links against preview.path, which is the absolute working tree file the bytes were read from.\n"+
			"linkTargets is keyed by diff-relative paths, so a link between two files of the review can only miss - and the\n"+
			"\"not part of this review\" label it falls back to carries the reviewer's absolute path:\n%s",
			previewLinkBaseSection, args)
	}
}

// The other half: if PreviewStack ever keys the table by something else, the
// assertion above stops meaning anything.
func TestPreviewLinkBaseMatchesHowTargetsAreKeyed(t *testing.T) {
	src := previewLinkBaseRead(t, previewLinkBaseStack)
	if !strings.Contains(src, "targets[path] = ") || !strings.Contains(src, "const path = filePath(file)") {
		t.Errorf("%s no longer keys linkTargets by filePath(file), so the base %s resolves against is no longer the matching one",
			previewLinkBaseStack, previewLinkBaseSection)
	}
}
