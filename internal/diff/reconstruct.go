package diff

import (
	"fmt"
	"strings"

	"github.com/tenntenn/sbnn/internal/model"
)

// GapMarker is what a hole in a reconstruction is written as. Unified diffs
// only carry the changed hunks and their context, so a modified file that is
// no longer on disk can only be rebuilt partially, and whoever reads the
// result needs to be told where the missing lines are.
//
// It is Markdown - a horizontal rule and a line of italics - and it is only
// ever written into Markdown. It used to be an HTML comment, which was
// invisible in the one format it was meant for and was foreign markup in
// every other: the same reconstruction is served for notebooks, where the
// comment made the JSON unparseable and the browser said so instead of
// showing the notebook, is written to a file for mo, and is baked into the
// page sbnn export produces. A line saying how many lines are missing is the
// most useful thing a partial preview can say, so it is now written in a form
// that is actually shown.
const GapMarker = "---\n\n*sbnn: %d line(s) not included in the diff*"

// Reconstruct rebuilds the new side of a file from its hunks.
//
// complete reports whether the result is the whole file. It is true for added
// files (the diff contains every line) and for diffs whose hunks happen to
// cover the file from line 1 without gaps. It is the only thing said about a
// file that is not Markdown: nothing is inserted into content with a syntax
// of its own, and the preview draws its "partial" badge from this flag.
func Reconstruct(f *model.File) (content string, complete bool) {
	var b strings.Builder
	complete = true
	next := 1 // the next new-side line number we expect to write
	var fence fenceState

	for _, h := range f.Hunks {
		if h.NewStart > next {
			complete = false
			writeGap(&b, f, h.NewStart-next, fence)
		}
		for _, l := range h.Lines {
			if l.Kind == model.LineDelete {
				continue
			}
			fence.track(l.Content)
			b.WriteString(l.Content)
			b.WriteString("\n")
			if l.NewNumber > 0 {
				next = l.NewNumber + 1
			}
		}
	}
	if f.Status == model.StatusAdded && len(f.Hunks) > 0 {
		// An added file is fully described by its hunks.
		complete = true
	}
	return b.String(), complete
}

// writeGap marks n missing lines in a reconstruction.
//
// Only a Markdown file gets a marker. A notebook is JSON that a renderer
// parses, and everything else - Go, YAML, plain text - has a syntax of its
// own too; a sentence sbnn made up is not part of any of them, and the
// reconstruction is handed on to mo and to an exported page as if it were
// the file. Those readers learn about the holes from complete instead.
//
// Inside a fenced code block the marker would be shown as code rather than
// read as Markdown, so the fence is closed around it and opened again after
// with the line that opened it, which keeps the following lines code and the
// marker visible.
func writeGap(b *strings.Builder, f *model.File, n int, fence fenceState) {
	if !f.IsMarkdown || f.IsNotebook {
		return
	}
	if fence.open {
		b.WriteString(strings.Repeat(string(fence.char), fence.length))
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "\n"+GapMarker+"\n\n", n)
	if fence.open {
		b.WriteString(fence.line)
		b.WriteString("\n")
	}
}

// fenceState is whether the lines written so far have left a fenced code
// block open, and what it would take to close and reopen it.
type fenceState struct {
	open   bool
	char   byte   // the fence character, ` or ~
	length int    // how many of it the fence opened with
	line   string // the opening line itself, info string and all
}

// track follows one line of Markdown in or out of a fenced code block.
func (s *fenceState) track(line string) {
	char, length, rest, ok := fenceAt(line)
	if !ok {
		return
	}
	if !s.open {
		s.open, s.char, s.length, s.line = true, char, length, line
		return
	}
	// A fence closes only on the character it opened with, with at least as
	// many of them, and with nothing else on the line.
	if char == s.char && length >= s.length && strings.TrimSpace(rest) == "" {
		s.open = false
	}
}

// fenceAt reports the code fence a line consists of: which character, how
// many of them, and what follows - the info string of an opening fence, or
// nothing at all for a closing one.
func fenceAt(line string) (char byte, length int, rest string, ok bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		// Indented four spaces or more: that is a code block, not a fence.
		return 0, 0, "", false
	}
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return 0, 0, "", false
	}
	char = t[0]
	n := 0
	for n < len(t) && t[n] == char {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	// The info string of a backtick fence may not itself contain a backtick,
	// so a line like `` `a` `` is text and not a fence.
	if char == '`' && strings.Contains(t[n:], "`") {
		return 0, 0, "", false
	}
	return char, n, t[n:], true
}

// Snippet returns the lines of a file between start and end on the given
// side, each keeping its diff marker. It is what a comment stores so that it
// still says something outside the browser.
func Snippet(f *model.File, side string, start, end int) string {
	var b strings.Builder
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			num := l.NewNumber
			if side == "old" {
				num = l.OldNumber
			}
			if num < start || num > end {
				continue
			}
			if side == "old" && l.Kind == model.LineAdd {
				continue
			}
			if side != "old" && l.Kind == model.LineDelete {
				continue
			}
			b.WriteString(marker(l.Kind))
			b.WriteString(l.Content)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func marker(kind model.LineKind) string {
	switch kind {
	case model.LineAdd:
		return "+"
	case model.LineDelete:
		return "-"
	default:
		return " "
	}
}
