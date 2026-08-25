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
// read as Markdown, so the fence is closed around it and opened again after,
// which keeps the following lines code and the marker visible.
//
// The closing fence, the marker and the reopened fence all carry the prefix
// the opening fence was written with. Without it a fence opened inside a list
// item ("   ```console") was closed by a "```" in column 0, which is outside
// the item and so closes nothing: everything after it stayed code, and the
// marker, the reopened fence and the rest of the file were shown literally
// inside the code block with the list broken in half.
func writeGap(b *strings.Builder, f *model.File, n int, fence fenceState) {
	if !f.IsMarkdown || f.IsNotebook {
		return
	}
	var indent string
	if fence.open {
		indent = fence.indent
		b.WriteString(indent)
		b.WriteString(strings.Repeat(string(fence.char), fence.length))
		b.WriteString("\n")
	}
	b.WriteString(blankIn(indent))
	for line := range strings.SplitSeq(fmt.Sprintf(GapMarker, n), "\n") {
		if line == "" {
			b.WriteString(blankIn(indent))
			continue
		}
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(blankIn(indent))
	if fence.open {
		b.WriteString(indent)
		b.WriteString(fence.fence)
		b.WriteString("\n")
	}
}

// blankIn is a blank line that stays inside the container indent belongs to.
// Indentation is dropped, because a line of spaces is trailing whitespace,
// but a blockquote marker is kept: a truly empty line would end the quote.
func blankIn(indent string) string {
	return strings.TrimRight(indent, " ") + "\n"
}

// fenceState is whether the lines written so far have left a fenced code
// block open, and what it would take to close and reopen it.
type fenceState struct {
	open   bool
	char   byte   // the fence character, ` or ~
	length int    // how many of it the fence opened with
	indent string // what continues the fence's container on a following line
	fence  string // the fence itself, info string and all, without the indent
}

// track follows one line of Markdown in or out of a fenced code block.
func (s *fenceState) track(line string) {
	indent, rest := splitContainers(line)
	char, length, info, ok := fenceAt(rest)
	if !ok {
		return
	}
	if !s.open {
		s.open, s.char, s.length = true, char, length
		s.indent, s.fence = indent, rest
		return
	}
	// A fence closes only on the character it opened with, with at least as
	// many of them, and with nothing else on the line.
	if char == s.char && length >= s.length && strings.TrimSpace(info) == "" {
		s.open = false
	}
}

// splitContainers peels the block containers a line opens - blockquote
// markers, list markers and up to three spaces of indentation at each level -
// off its front. It returns the prefix that continues those containers on a
// following line, and what is left of the line.
//
// A list marker becomes spaces of its own width, because a following line
// continues the item rather than starting a new one; a blockquote marker
// stays as it is. This is what lets a fence opened as "- ```go" or "> ```go"
// be seen at all: fenceAt only ever sees the fence itself, and the prefix it
// was found behind is what the closing and reopened fences are written with.
func splitContainers(line string) (cont, rest string) {
	var b strings.Builder
	rest = line
	for {
		t := strings.TrimLeft(rest, " ")
		if n := len(rest) - len(t); n > 3 {
			// Four spaces or more: indented code, not another container.
			break
		} else {
			b.WriteString(strings.Repeat(" ", n))
			rest = t
		}
		switch {
		case strings.HasPrefix(rest, ">"):
			b.WriteString(">")
			rest = rest[1:]
			if strings.HasPrefix(rest, " ") {
				b.WriteString(" ")
				rest = rest[1:]
			}
		default:
			n := listMarkerLen(rest)
			if n == 0 {
				return b.String(), rest
			}
			b.WriteString(strings.Repeat(" ", n))
			rest = rest[n:]
		}
	}
	return b.String(), rest
}

// listMarkerLen returns the width of the list marker a line starts with,
// trailing spaces included, or 0 if it does not start with one. A marker with
// nothing after it opens an empty item and is not a container for this line.
func listMarkerLen(s string) int {
	i := 0
	switch {
	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "*"), strings.HasPrefix(s, "+"):
		i = 1
	default:
		for i < len(s) && i < 9 && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) || (s[i] != '.' && s[i] != ')') {
			return 0
		}
		i++
	}
	spaces := 0
	for i+spaces < len(s) && s[i+spaces] == ' ' {
		spaces++
	}
	if spaces == 0 || spaces > 4 {
		// No space at all is not a marker; five or more starts indented
		// code inside the item, which is not this line's container.
		return 0
	}
	return i + spaces
}

// fenceAt reports the code fence a line consists of: which character, how
// many of them, and what follows - the info string of an opening fence, or
// nothing at all for a closing one. It is given a line with its containers
// already peeled off by splitContainers, so any space left in front of the
// fence is indentation of four or more, which is a code block and not a
// fence.
func fenceAt(t string) (char byte, length int, rest string, ok bool) {
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
