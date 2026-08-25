package diff

import (
	"regexp"
	"strings"

	"github.com/tenntenn/sbnn/internal/model"
)

// generatedTop is how far into a file the declaration is looked for. The
// convention puts it first; a licence header or a shebang can push it down a
// little, and past that it is no longer a declaration anybody reads.
const generatedTop = 10

// generatedMarkers are the ways a file says it was generated.
//
// Every one of them is the file speaking about itself: a generator wrote the
// line so that tools would leave the file alone. That is what makes folding
// such a file honest, and it is why sbnn looks for nothing else - not a size,
// not a directory, not a name. Those would be sbnn guessing about a project it
// knows nothing about, and a wrong guess hides code from a review.
var generatedMarkers = []*regexp.Regexp{
	// The Go convention, also emitted by many tools outside Go.
	regexp.MustCompile(`Code generated .* DO NOT EDIT\.?`),
	// The @generated tag, understood by GitHub, Phabricator and others.
	regexp.MustCompile(`@generated\b`),
	// The same statement in the words other generators use.
	regexp.MustCompile(`(?i)(automatically |auto-?)?generated( file)?[ ,.:;!-]*.{0,40}do not (edit|modify)`),
	regexp.MustCompile(`(?i)do not (edit|modify)[ ,.:;!-]*.{0,40}(automatically |auto-?)?generated`),
	// The same statement in Japanese. A file whose header says 自動生成 is
	// speaking about itself just as plainly as one that says @generated, and a
	// project cannot be asked to write that header in English before its diff
	// can be reviewed properly. These are the phrases generators actually
	// emit; each still has to appear near the top of the file, and the line it
	// matched is still what gets reported.
	regexp.MustCompile(`自動生成|自動的に生成|編集しないで|編集不可|手動で編集しない`),
}

// GeneratedMarker returns the line by which a file declares itself
// generated, or "" when it does not. The line is returned rather than a
// yes: whatever acts on it can then show why, and be argued with.
func GeneratedMarker(content string) string {
	for i, line := range strings.SplitN(content, "\n", generatedTop+1) {
		if i >= generatedTop {
			break
		}
		for _, re := range generatedMarkers {
			if re.MatchString(line) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

// VisibleTop returns the beginning of the new side of a file as the diff
// shows it, or "" when the diff does not reach that far.
//
// A unified diff carries only the hunks that changed, so the top of a
// modified file is usually not in it. Nothing is inferred from that absence:
// no top, no declaration, and the file is left alone.
func VisibleTop(f *model.File) string {
	if len(f.Hunks) == 0 || f.Hunks[0].NewStart != 1 {
		return ""
	}
	lines := make([]string, 0, generatedTop)
	for _, l := range f.Hunks[0].Lines {
		if l.Kind == model.LineDelete {
			continue
		}
		lines = append(lines, l.Content)
		if len(lines) == generatedTop {
			break
		}
	}
	return strings.Join(lines, "\n")
}
