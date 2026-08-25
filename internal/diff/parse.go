// Package diff parses unified diff text into the sbnn data model.
//
// The parser accepts both git style diffs (with "diff --git" headers and
// extended headers such as "new file mode") and plain diffs produced by
// "diff -u". sbnn never runs git itself: everything it knows about a change
// comes from the diff text handed to it on stdin.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/tenntenn/sbnn/internal/model"
)

// markdownExts are the extensions previewed with mo.
var markdownExts = map[string]bool{
	".md":       true,
	".markdown": true,
	".mdown":    true,
	".mkd":      true,
	".mdx":      true,
}

// IsMarkdown reports whether p looks like a Markdown file.
func IsMarkdown(p string) bool {
	return markdownExts[strings.ToLower(path.Ext(p))]
}

// imageContentTypes maps the extensions sbnn previews as images to the MIME
// type served for them. It is limited to what a browser actually renders in
// an <img> tag - heic, heif, tif and tiff are image formats too, but not ones
// mainstream browsers display.
var imageContentTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
}

// IsImage reports whether p looks like an image sbnn can preview.
func IsImage(p string) bool {
	_, ok := imageContentTypes[strings.ToLower(path.Ext(p))]
	return ok
}

// ImageContentType returns the MIME type to serve p's content as, or "" if p
// is not a previewable image.
func ImageContentType(p string) string {
	return imageContentTypes[strings.ToLower(path.Ext(p))]
}

// IsNotebook reports whether p is a Jupyter notebook.
func IsNotebook(p string) bool {
	return strings.ToLower(path.Ext(p)) == ".ipynb"
}

// Parse parses unified diff text into files.
func Parse(src string) []*model.File {
	p := &parser{lines: splitLines(src)}
	p.run()
	for i, f := range p.files {
		finalize(f, i)
	}
	return p.files
}

func splitLines(src string) []string {
	src = strings.TrimSuffix(src, "\n")
	if src == "" {
		return nil
	}
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

type parser struct {
	lines []string
	i     int
	files []*model.File
	// git reports whether the current file entry came from a "diff --git"
	// header, which tells us the a/ and b/ prefixes are synthetic.
	git bool
	// headersRead reports whether the current file entry has already
	// consumed a "--- / +++" pair, so that a second one starts a new file.
	// A plain diff of several files is nothing but such pairs.
	headersRead bool
	// pathsKnown reports whether the paths were taken from a rename or copy
	// header, which are the real ones: the "--- / +++" pair of the same
	// entry must not overwrite them, but it does not start a new file
	// either.
	pathsKnown bool
}

func (p *parser) run() {
	for p.i < len(p.lines) {
		line := p.lines[p.i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			p.startGitFile(strings.TrimPrefix(line, "diff --git "))
			p.i++
		case strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined "):
			// Combined diff of a merge commit. Line numbers are ambiguous, so
			// the content is shown without them. The two spellings are not the
			// same length, so each is trimmed as itself.
			name := strings.TrimPrefix(line, "diff --cc ")
			name = strings.TrimSpace(strings.TrimPrefix(name, "diff --combined "))
			p.push(&model.File{OldPath: name, NewPath: name, Status: model.StatusModified})
			p.git = true
			p.i++
		case strings.HasPrefix(line, "--- ") && p.i+1 < len(p.lines) && strings.HasPrefix(p.lines[p.i+1], "+++ "):
			if p.current() == nil || len(p.current().Hunks) > 0 || p.headersRead {
				p.push(&model.File{})
				p.git = false
			}
			p.readFileHeaders()
		case strings.HasPrefix(line, "@@"):
			p.readHunk()
		default:
			if !p.readExtendedHeader(line) {
				p.i++
			}
		}
	}
}

func (p *parser) current() *model.File {
	if len(p.files) == 0 {
		return nil
	}
	return p.files[len(p.files)-1]
}

// file returns the file entry the parser is currently filling, creating one
// for diffs that start straight with a hunk.
func (p *parser) file() *model.File {
	if f := p.current(); f != nil {
		return f
	}
	f := &model.File{}
	p.push(f)
	return f
}

func (p *parser) push(f *model.File) {
	p.files = append(p.files, f)
	p.headersRead = false
	p.pathsKnown = false
}

func (p *parser) startGitFile(rest string) {
	old, new, ok := splitGitHeaderPaths(rest)
	f := &model.File{}
	if ok {
		f.OldPath, f.NewPath = old, new
	}
	p.push(f)
	p.git = true
}

// readExtendedHeader consumes a git extended header line and reports whether
// it did.
func (p *parser) readExtendedHeader(line string) bool {
	f := p.current()
	switch {
	case strings.HasPrefix(line, "old mode "):
		if f != nil {
			f.OldMode = strings.TrimSpace(strings.TrimPrefix(line, "old mode "))
		}
	case strings.HasPrefix(line, "new mode "):
		if f != nil {
			f.NewMode = strings.TrimSpace(strings.TrimPrefix(line, "new mode "))
		}
	case strings.HasPrefix(line, "new file mode "):
		if f != nil {
			f.Status = model.StatusAdded
			f.NewMode = strings.TrimSpace(strings.TrimPrefix(line, "new file mode "))
		}
	case strings.HasPrefix(line, "deleted file mode "):
		if f != nil {
			f.Status = model.StatusDeleted
			f.OldMode = strings.TrimSpace(strings.TrimPrefix(line, "deleted file mode "))
		}
	case strings.HasPrefix(line, "rename from "):
		if f != nil {
			f.Status = model.StatusRenamed
			f.OldPath = unquotePath(strings.TrimPrefix(line, "rename from "))
			p.pathsKnown = true
		}
	case strings.HasPrefix(line, "rename to "):
		if f != nil {
			f.Status = model.StatusRenamed
			f.NewPath = unquotePath(strings.TrimPrefix(line, "rename to "))
			p.pathsKnown = true
		}
	case strings.HasPrefix(line, "copy from "):
		if f != nil {
			f.Status = model.StatusCopied
			f.OldPath = unquotePath(strings.TrimPrefix(line, "copy from "))
			p.pathsKnown = true
		}
	case strings.HasPrefix(line, "copy to "):
		if f != nil {
			f.Status = model.StatusCopied
			f.NewPath = unquotePath(strings.TrimPrefix(line, "copy to "))
			p.pathsKnown = true
		}
	case strings.HasPrefix(line, "index "):
		if f != nil {
			f.Index = strings.TrimSpace(strings.TrimPrefix(line, "index "))
		}
	case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") ||
		(strings.HasPrefix(line, "Files ") && strings.HasSuffix(line, " differ")):
		if f != nil {
			f.IsBinary = true
		}
	default:
		return false
	}
	p.i++
	return true
}

func (p *parser) readFileHeaders() {
	f := p.file()
	oldRaw := trimTimestamp(strings.TrimPrefix(p.lines[p.i], "--- "))
	newRaw := trimTimestamp(strings.TrimPrefix(p.lines[p.i+1], "+++ "))
	p.i += 2

	strip := p.git || (hasPrefixPath(oldRaw, "a") && hasPrefixPath(newRaw, "b"))
	old := normalizePath(oldRaw, strip)
	new := normalizePath(newRaw, strip)

	// rename/copy headers already recorded the real paths.
	if !p.pathsKnown {
		if old != "" {
			f.OldPath = old
		}
		if new != "" {
			f.NewPath = new
		}
	}
	switch {
	case oldRaw == model.DevNull:
		f.Status = model.StatusAdded
		f.OldPath = ""
	case newRaw == model.DevNull:
		f.Status = model.StatusDeleted
		f.NewPath = ""
	}
	p.headersRead = true
}

func (p *parser) readHunk() {
	line := p.lines[p.i]
	// Combined diffs use @@@ and carry two old sides; their line numbers are
	// not meaningful for a two-way view, so they are shown without numbers.
	if strings.HasPrefix(line, "@@@") {
		p.readCombinedHunk()
		return
	}
	h, ok := parseHunkHeader(line)
	if !ok {
		p.i++
		return
	}
	p.i++
	f := p.file()

	oldNo, newNo := h.OldStart, h.NewStart
	oldLeft, newLeft := h.OldLines, h.NewLines
	// Index one past the last surplus body line, computed once the counts run
	// out. -1 means "not computed yet".
	surplusEnd := -1
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if strings.HasPrefix(l, "\\") { // "\ No newline at end of file"
			if n := len(h.Lines); n > 0 {
				h.Lines[n-1].NoNewline = true
			}
			p.i++
			continue
		}
		if oldLeft <= 0 && newLeft <= 0 {
			if surplusEnd < 0 {
				surplusEnd = p.surplusBodyEnd()
			}
			if p.i >= surplusEnd {
				break
			}
		}
		var kind model.LineKind
		var content string
		switch {
		case l == "":
			// git emits a bare empty line for an empty context line.
			kind, content = model.LineContext, ""
		case l[0] == ' ':
			kind, content = model.LineContext, l[1:]
		case l[0] == '+':
			kind, content = model.LineAdd, l[1:]
		case l[0] == '-':
			kind, content = model.LineDelete, l[1:]
		default:
			// Anything else ends the hunk (next file header, trailing text...).
			oldLeft, newLeft = 0, 0
			continue
		}
		ln := model.Line{Kind: kind, Content: content}
		switch kind {
		case model.LineContext:
			ln.OldNumber, ln.NewNumber = oldNo, newNo
			oldNo++
			newNo++
			oldLeft--
			newLeft--
		case model.LineAdd:
			ln.NewNumber = newNo
			newNo++
			newLeft--
		case model.LineDelete:
			ln.OldNumber = oldNo
			oldNo++
			oldLeft--
		}
		h.Lines = append(h.Lines, ln)
		p.i++
	}
	f.Hunks = append(f.Hunks, h)
}

// surplusBodyEnd returns the index one past the last line that still belongs
// to the hunk being read, scanning from the line the counts ran out on.
//
// The counts used to be trusted absolutely, so a header that promised fewer
// lines than its body carried - a patch cut short by a pipe, a mail client
// that reflowed it, a generator that miscounted - had its surplus lines
// dropped without a word. That is the worst thing a review tool can do with
// input it was handed: the reviewer approves the change they were shown, and
// the change they were shown was smaller than the one that arrived.
//
// Past the counts the parser has no length to lean on, so it has to tell a
// body line from whatever the patch is wrapped in, and the wrapping starts
// with the same characters a body line does. Only a change line - a '+' or '-'
// that is not a file header and not a run of dashes - extends the hunk, and it
// carries along any context that sits between it and the counted body. What
// trails the last change line is left alone, because a diffstat row and a
// trailing blank are indistinguishable from context.
//
// That is what keeps `git format-patch` output whole: every one of its patches
// ends with the "-- " signature, and taking that as a deletion both invents a
// change nobody made and, since Deletions then stops being zero, flips an
// add-only file out of the unified view.
func (p *parser) surplusBodyEnd() int {
	end := p.i
	for i := p.i; i < len(p.lines); i++ {
		l := p.lines[i]
		switch {
		case l == "" || l[0] == ' ' || l[0] == '\\':
			// Context, an empty context line, or a "\ No newline" marker.
			// Ambiguous on its own; kept only if a change line follows.
		case l[0] == '+' && !strings.HasPrefix(l, "+++ "):
			end = i + 1
		case l[0] == '-' && !strings.HasPrefix(l, "--- ") && !isDashRun(l):
			end = i + 1
		default:
			return end
		}
	}
	return end
}

// isDashRun reports whether l is nothing but dashes, optionally with trailing
// spaces: the "-- " signature git format-patch ends with, the "---" that
// separates a commit message from its diffstat, and the "--" some mailers
// leave behind.
func isDashRun(l string) bool {
	l = strings.TrimRight(l, " ")
	return l != "" && strings.TrimLeft(l, "-") == ""
}

func (p *parser) readCombinedHunk() {
	f := p.file()
	h := &model.Hunk{Header: p.lines[p.i]}
	p.i++
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, "diff ") {
			break
		}
		if strings.HasPrefix(l, "\\") {
			p.i++
			continue
		}
		kind := model.LineContext
		content := l
		if l != "" {
			switch l[0] {
			case '+':
				kind, content = model.LineAdd, l[1:]
			case '-':
				kind, content = model.LineDelete, l[1:]
			case ' ':
				content = l[1:]
			}
		}
		h.Lines = append(h.Lines, model.Line{Kind: kind, Content: content})
		p.i++
	}
	f.Hunks = append(f.Hunks, h)
}

type hunkHeader = model.Hunk

// parseHunkHeader parses "@@ -1,3 +1,4 @@ optional section".
func parseHunkHeader(line string) (*hunkHeader, bool) {
	rest := strings.TrimPrefix(line, "@@")
	end := strings.Index(rest, "@@")
	if end < 0 {
		return nil, false
	}
	ranges := strings.Fields(rest[:end])
	if len(ranges) < 2 {
		return nil, false
	}
	oldStart, oldLines, ok := parseRange(ranges[0], '-')
	if !ok {
		return nil, false
	}
	newStart, newLines, ok := parseRange(ranges[1], '+')
	if !ok {
		return nil, false
	}
	return &hunkHeader{
		Header:   line,
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Section:  strings.TrimSpace(rest[end+2:]),
	}, true
}

func parseRange(s string, sign byte) (start, count int, ok bool) {
	if len(s) == 0 || s[0] != sign {
		return 0, 0, false
	}
	s = s[1:]
	count = 1
	if i := strings.IndexByte(s, ','); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return 0, 0, false
		}
		count = n
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return n, count, true
}

// finalize fills in the derived fields of a parsed file.
func finalize(f *model.File, index int) {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case model.LineAdd:
				f.Additions++
			case model.LineDelete:
				f.Deletions++
			}
		}
	}
	if f.Status == "" {
		switch {
		case f.OldPath == "" && f.NewPath != "":
			f.Status = model.StatusAdded
		case f.NewPath == "" && f.OldPath != "":
			f.Status = model.StatusDeleted
		case len(f.Hunks) == 0 && !f.IsBinary && (f.OldMode != "" || f.NewMode != ""):
			f.Status = model.StatusMode
		default:
			f.Status = model.StatusModified
		}
	}
	// A new file has nothing to put on the left hand side, so it is always
	// shown as unified. The same is true for deletions and binary blobs.
	switch {
	case f.Status == model.StatusAdded, f.Status == model.StatusDeleted, f.IsBinary, f.Deletions == 0:
		f.ViewMode = model.ViewUnified
	default:
		f.ViewMode = model.ViewSplit
	}
	f.IsMarkdown = IsMarkdown(f.Path())
	f.IsImage = IsImage(f.Path())
	f.IsNotebook = IsNotebook(f.Path())
	f.ID = fileID(f, index)
}

func fileID(f *model.File, index int) string {
	sum := sha256.Sum256([]byte(f.Path()))
	return fmt.Sprintf("f%d-%s", index+1, hex.EncodeToString(sum[:])[:8])
}

// splitGitHeaderPaths splits the "a/foo b/foo" part of a "diff --git" header.
// Paths may contain spaces, so every possible split point is tried and the
// one where both sides carry the expected prefix wins; equal paths (the
// common case) are preferred.
func splitGitHeaderPaths(rest string) (old, new string, ok bool) {
	if strings.HasPrefix(rest, `"`) {
		// Quoted paths are unambiguous: "a/x" "b/y".
		if o, r, k := cutQuoted(rest); k {
			n := strings.TrimSpace(r)
			return normalizePath(o, true), normalizePath(unquotePath(n), true), true
		}
	}
	var fallback [2]string
	found := false
	for i := 0; i < len(rest); i++ {
		if rest[i] != ' ' {
			continue
		}
		l, r := rest[:i], rest[i+1:]
		if !hasPrefixPath(l, "a") || !hasPrefixPath(r, "b") {
			continue
		}
		lp, rp := normalizePath(l, true), normalizePath(r, true)
		if lp == rp {
			return lp, rp, true
		}
		if !found {
			fallback = [2]string{lp, rp}
			found = true
		}
	}
	if found {
		return fallback[0], fallback[1], true
	}
	return "", "", false
}

func cutQuoted(s string) (quoted, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", s, false
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			return unquotePath(s[:i+1]), s[i+1:], true
		}
	}
	return "", s, false
}

func hasPrefixPath(s, prefix string) bool {
	return s == model.DevNull || strings.HasPrefix(s, prefix+"/") ||
		(strings.HasPrefix(s, `"`+prefix+"/") && strings.HasSuffix(s, `"`))
}

// trimTimestamp drops the timestamp "diff -u" appends after a tab.
func trimTimestamp(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		return s[:i]
	}
	return strings.TrimRight(s, " ")
}

// normalizePath unquotes a path and optionally removes the a/ or b/ prefix.
func normalizePath(s string, strip bool) string {
	s = unquotePath(strings.TrimSpace(s))
	if s == model.DevNull {
		return ""
	}
	if strip {
		if len(s) > 2 && (s[0] == 'a' || s[0] == 'b') && s[1] == '/' {
			s = s[2:]
		}
	}
	return s
}

// unquotePath undoes the C style quoting git applies to unusual paths.
func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return s
	}
	if unquoted, err := strconv.Unquote(s); err == nil {
		return unquoted
	}
	return s
}
