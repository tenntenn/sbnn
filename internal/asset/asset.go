// Package asset resolves the images a previewed Markdown file points at.
//
// A Markdown document under review routinely draws a picture that sits next
// to it in the tree - "![](diagram.png)" - and that reference resolves
// against nothing useful in a review page: the live page is served from the
// server root, and an exported page has no server at all. Both therefore
// need the reference turned into something the browser can draw before the
// Markdown is rendered, and they need to turn it into the *same* thing, or a
// review looks one way on screen and another way in the file that was mailed
// around.
//
// So the decision is made here, once, out of nothing but the document and
// the directory the diff was sent from: which references name an image, which
// of those are inside that directory, which are small enough to carry, and
// which are not. The server hands the answers out as URLs of its own and the
// exporter hands them out as data URLs, but the answers themselves - and so
// what the reader sees - are the same on both sides.
package asset

import (
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// MaxBytes bounds one image, and MaxTotalBytes bounds all the images of one
// previewed document together.
//
// The numbers come from opening pages with images inlined as data URLs in
// Chromium and timing them, because "too large" only means anything as a
// number of seconds:
//
//	image     page      open      JS heap
//	 0.5MB    0.67MB     58ms      10MB
//	 2MB      2.67MB    157ms      12MB
//	 8MB     10.67MB    578ms      45MB
//	16MB     21.35MB   1216ms      87MB
//	32MB     42.68MB   3214ms     179MB
//
// Base64 is the reason a page is a third larger than the bytes in it.
//
// 2MB per image is where a single picture still appears in well under a
// fifth of a second and adds under 3MB to the page; for scale, the screenshot
// in this repository's own README is 190KB, so the cap is an order of
// magnitude above what a diagram in a review actually weighs. 8MB for the
// whole document keeps one preview under about six tenths of a second and its
// contribution to the page under 11MB, which still leaves room to mail the
// result - and a page past that is not one anybody opens twice.
//
// Both caps are properties of a single document, deliberately: a budget for
// the whole page could only be spent at export time, and the live preview,
// which renders one file at a time and knows nothing of the others, would
// then decide differently from the exported page for the same file.
const (
	MaxBytes      = 2 << 20 // 2MiB
	MaxTotalBytes = 8 << 20 // 8MiB
)

// Status says what became of one image reference. Every reference gets one,
// including the ones that were not inlined: a reader is owed the difference
// between a picture that is too big to carry and a file that is not there.
type Status string

const (
	// StatusOK means the image is available and named by URL.
	StatusOK Status = "ok"
	// StatusTooLarge means the file is past MaxBytes on its own.
	StatusTooLarge Status = "too-large"
	// StatusOverBudget means the document had already spent MaxTotalBytes
	// on the images before this one.
	StatusOverBudget Status = "over-budget"
	// StatusOutside means the path leaves the directory the diff was sent
	// from - see source.AbsPath for why that is refused rather than read.
	StatusOutside Status = "outside"
	// StatusMissing means there is no such file in the working tree.
	StatusMissing Status = "missing"
	// StatusUnsupported means the path is not one of the image types a
	// browser draws in an <img>.
	StatusUnsupported Status = "unsupported"
)

// Ref is one image reference of a document, resolved.
type Ref struct {
	// Src is the reference exactly as the Markdown wrote it, which is what
	// the renderer puts in the src attribute and therefore what the page
	// looks the image up by.
	Src string
	// Rel is the path relative to the directory the diff was sent from,
	// empty when the reference did not resolve to one.
	Rel string
	// Path is the working tree file, empty unless there is one.
	Path string
	// Size is that file's size in bytes.
	Size int64
	// Status is what became of the reference.
	Status Status
}

// Label is what to call the image on screen when there is no picture to show.
func (r Ref) Label() string {
	if r.Rel != "" {
		return r.Rel
	}
	return r.Src
}

// Entry is one image as the page receives it: either a URL to draw, or a
// status saying why there is none.
type Entry struct {
	// URL is what the <img> is pointed at - an endpoint of the server on a
	// live page, a data URL in an exported one. Empty unless Status is
	// StatusOK.
	URL string `json:"url,omitempty"`
	// Path names the file, for the placeholder that stands in for it.
	Path string `json:"path,omitempty"`
	// Status is why there is no URL, when there is none.
	Status Status `json:"status"`
	// Size is the file's size in bytes, so a placeholder can say how big
	// the picture that did not fit was.
	Size int64 `json:"size,omitempty"`
}

// Refs returns, in document order, the images markdown points at with a
// relative path. filePath is the document's own path in the diff and baseDir
// the directory the diff was sent from.
//
// A reference that names an absolute or remote URL is not returned at all:
// the browser resolves those by itself and does so identically on both kinds
// of page, so there is nothing here to decide about them.
func Refs(baseDir, filePath, markdown string) []Ref {
	dir := path.Dir(path.Clean(filePath))
	var (
		out   []Ref
		seen  = map[string]bool{}
		total int64
	)
	for _, src := range sources(markdown) {
		if seen[src] {
			continue
		}
		seen[src] = true
		rel, ok := relPath(dir, src)
		if !ok {
			continue
		}
		r := Ref{Src: src, Rel: rel}
		switch {
		case rel == "":
			r.Status = StatusOutside
		case !diff.IsImage(rel):
			r.Status = StatusUnsupported
		default:
			r.Status = statAsset(baseDir, &r, &total)
		}
		out = append(out, r)
	}
	return out
}

// statAsset fills in Path and Size and reports the status of a reference
// that named an image inside the tree, spending r.Size of the budget when it
// is carried.
func statAsset(baseDir string, r *Ref, total *int64) Status {
	abs := source.AbsPath(baseDir, r.Rel)
	if abs == "" {
		return StatusOutside
	}
	st, err := os.Stat(abs)
	if err != nil || !st.Mode().IsRegular() {
		return StatusMissing
	}
	r.Path = abs
	r.Size = st.Size()
	switch {
	case r.Size > MaxBytes:
		return StatusTooLarge
	case *total+r.Size > MaxTotalBytes:
		return StatusOverBudget
	}
	*total += r.Size
	return StatusOK
}

// relPath turns the reference src, written inside a document living in dir,
// into a path relative to the directory the diff was sent from. ok is false
// for the references this package has nothing to say about: the remote ones,
// and the ones that name no path at all.
//
// The reference is percent-decoded first. "%2E%2E/x.png" is "../x.png"
// written in a way that would otherwise sail past a check for "..", and a
// document is free to spell an ordinary file name that way too.
func relPath(dir, src string) (rel string, ok bool) {
	s := strings.TrimSpace(src)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", false
	}
	if strings.HasPrefix(s, "//") || schemeRe.MatchString(s) {
		return "", false
	}
	// A query or a fragment is addressed to whatever serves the URL, and
	// there is no server here to address.
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "/") {
		// Rooted at the server, which is sbnn itself and not the tree.
		return "", true
	}
	// Joined against the document's own directory, an escape shows up as a
	// leading "..". It is reported here rather than left to source.AbsPath
	// so that the reason a picture is missing is the true one, and so that
	// nothing about the path outside is looked up on disk at all.
	rel = path.Join(dir, s)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", true
	}
	return rel, true
}

var schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

var (
	// An inline image: ![alt](dest) or ![alt](<dest with spaces> "title").
	inlineRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*(<[^<>\n]*>|[^\s()]+)`)
	// A reference image: ![alt][label], ![alt][] or the shortcut ![label].
	refRe = regexp.MustCompile(`!\[([^\]\n]*)\](\[([^\]\n]*)\])?`)
	// The definition a reference image points at.
	defRe = regexp.MustCompile(`(?m)^ {0,3}\[([^\]\n]+)\]:[ \t]*(<[^<>\n]*>|\S+)`)
	// Raw HTML, which the Markdown renderer passes through untouched.
	htmlRe = regexp.MustCompile(`(?is)<img\b[^>]*?\bsrc\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

// sources returns the destination of every image in markdown, in the order
// they appear.
//
// Fenced code is dropped first: an image written inside a fence is shown as
// the text it is, and reading the file it names would spend the document's
// budget on a picture nobody is going to see.
func sources(markdown string) []string {
	md := stripFences(markdown)
	defs := definitions(md)

	type hit struct {
		at  int
		src string
	}
	var hits []hit
	add := func(at int, src string) {
		if src = unbracket(src); src != "" {
			hits = append(hits, hit{at, src})
		}
	}
	for _, m := range inlineRe.FindAllStringSubmatchIndex(md, -1) {
		add(m[0], md[m[2]:m[3]])
	}
	for _, m := range htmlRe.FindAllStringSubmatchIndex(md, -1) {
		add(m[0], unquote(md[m[2]:m[3]]))
	}
	for _, m := range refRe.FindAllStringSubmatchIndex(md, -1) {
		// An inline image matches this too - "![a](x.png)" ends in a "]"
		// followed by a "(" - and it is already accounted for above.
		if m[1] < len(md) && md[m[1]] == '(' {
			continue
		}
		label := md[m[2]:m[3]]
		if m[6] >= 0 && md[m[6]:m[7]] != "" {
			label = md[m[6]:m[7]]
		}
		if dest, ok := defs[foldLabel(label)]; ok {
			add(m[0], dest)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at < hits[j].at })

	srcs := make([]string, 0, len(hits))
	for _, h := range hits {
		srcs = append(srcs, h.src)
	}
	return srcs
}

// definitions collects the link definitions of a document, keyed the way a
// reference to one is matched: case-insensitively, on folded whitespace.
func definitions(md string) map[string]string {
	defs := map[string]string{}
	for _, m := range defRe.FindAllStringSubmatch(md, -1) {
		key := foldLabel(m[1])
		if _, seen := defs[key]; !seen {
			defs[key] = unbracket(m[2])
		}
	}
	return defs
}

func foldLabel(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// unbracket unwraps the <...> form of a link destination.
func unbracket(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// stripFences blanks out fenced code blocks, keeping the line count so that
// nothing else moves.
func stripFences(md string) string {
	lines := strings.Split(md, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if fence == "" {
			if open := fenceOf(trimmed); open != "" {
				fence = open
				lines[i] = ""
			}
			continue
		}
		if close := fenceOf(trimmed); close != "" &&
			close[0] == fence[0] && len(close) >= len(fence) {
			fence = ""
		}
		lines[i] = ""
	}
	return strings.Join(lines, "\n")
}

// fenceOf returns the run of backticks or tildes a line opens or closes a
// fence with, or "" when it is not a fence line.
func fenceOf(line string) string {
	if line == "" {
		return ""
	}
	c := line[0]
	if c != '`' && c != '~' {
		return ""
	}
	n := 0
	for n < len(line) && line[n] == c {
		n++
	}
	if n < 3 {
		return ""
	}
	return line[:n]
}

// InDiff decides what becomes of an image that is part of the diff itself,
// rather than one a previewed Markdown file points at.
//
// The two were decided differently and should not have been. A sibling image
// has been capped at MaxBytes since #305; an image of the diff was frozen
// into an exported page whatever it weighed, so one 32MiB PNG turned a page
// meant to be mailed around into 45MB of base64. The rule is the same rule,
// applied to the same kind of thing, and lives here so that both sides of it
// - the live page, which draws the picture from an endpoint, and the exported
// page, which carries the bytes - reach one answer for one file.
//
// It is deliberately per file and not per page. A budget spent across the
// page can only be spent at export time: the live preview draws one file at a
// time and knows nothing of the others, so the same file would be shown on
// screen and left out of the export, which is exactly the divergence #305
// exists to prevent. MaxTotalBytes is a property of one document for the same
// reason. So the page total stays unbounded in the number of files, and every
// individual picture in it is bounded.
//
// baseDir is the directory the diff was sent from; f must be the file itself.
// A file that is not an image, or was deleted, has nothing to decide and gets
// no status at all.
func InDiff(baseDir string, f *model.File) (Status, int64) {
	if !f.IsImage || f.Status == model.StatusDeleted {
		return "", 0
	}
	abs := source.AbsPath(baseDir, f.Path())
	if abs == "" {
		return StatusOutside, 0
	}
	st, err := os.Stat(abs)
	if err != nil || !st.Mode().IsRegular() {
		return StatusMissing, 0
	}
	if st.Size() > MaxBytes {
		return StatusTooLarge, st.Size()
	}
	return StatusOK, st.Size()
}

// RecordInDiff writes InDiff's answer onto every file of a diff, so that the
// page can tell whether there is a picture to draw before it asks for one.
func RecordInDiff(d *model.Diff) {
	for _, f := range d.Files {
		status, size := InDiff(d.BaseDir, f)
		f.ImageStatus, f.ImageSize = string(status), size
	}
}
