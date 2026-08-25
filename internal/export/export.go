// Package export writes a review as a single self-contained HTML page.
//
// The page carries the same UI as the sbnn server, but with the diff frozen
// into it: no server, no mo, no network. It is what you hand to someone who
// should look at a change without running sbnn, and it is what makes a review
// publishable as an artifact.
package export

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// PayloadVersion is the schema version of the embedded data.
//
// It is bumped when the absence of a field stops meaning what it used to.
// Version 2 added the review fields: in a version 1 page an absent verdict
// could equally mean "not reviewed" and "exported by a binary that did not
// carry the verdict", and a reader has no way to tell those apart. From
// version 2 on, an absent verdict means the review was not submitted.
const PayloadVersion = 2

// Preview is the Markdown or notebook JSON of one file, frozen at export
// time.
type Preview struct {
	Content  string `json:"content"`
	Source   string `json:"source"`
	Complete bool   `json:"complete"`
	Path     string `json:"path,omitempty"`
}

// Image is one image file's content, frozen at export time as a data URL so
// the exported page needs no server to show it.
type Image struct {
	DataURL string `json:"dataUrl"`
	Path    string `json:"path,omitempty"`
}

// Payload is the data the exported page reads out of window.__SBNN_DATA__.
type Payload struct {
	Version     int                `json:"version"`
	SaVersion   string             `json:"saVersion,omitempty"`
	GeneratedAt time.Time          `json:"generatedAt"`
	Group       string             `json:"group"`
	Diffs       []*model.Diff      `json:"diffs"`
	Comments    []*model.Comment   `json:"comments"`
	Previews    map[string]Preview `json:"previews"`
	Images      map[string]Image   `json:"images"`

	// ReviewedAt, ReviewNote and ReviewVerdict say how the review ended.
	// Without them the page can only show the comments, and renders a
	// submitted review as though it had never been submitted - no banner,
	// no verdict on the button, and a prompt that tells an agent to address
	// comments that in fact came with an approval.
	//
	// The names match model.Group, so a page reads a frozen review exactly
	// the way it reads a live one.
	ReviewedAt    time.Time     `json:"reviewedAt,omitzero"`
	ReviewNote    string        `json:"reviewNote,omitempty"`
	ReviewVerdict model.Verdict `json:"reviewVerdict,omitempty"`
}

// Build freezes a group into a payload. Markdown, notebook and image files
// are resolved the same way the live preview does: the working tree file
// when it is still there, the new side rebuilt from the diff otherwise - and
// for a binary image, only the working tree copy can be shown at all.
func Build(g *model.Group, saVersion string, now time.Time) *Payload {
	p := &Payload{
		Version:     PayloadVersion,
		SaVersion:   saVersion,
		GeneratedAt: now,
		Group:       g.Name,
		Diffs:       make([]*model.Diff, 0, len(g.Diffs)),
		Comments:    g.Comments,
		Previews:    map[string]Preview{},
		Images:      map[string]Image{},

		ReviewedAt:    g.ReviewedAt,
		ReviewNote:    g.ReviewNote,
		ReviewVerdict: g.ReviewVerdict,
	}
	if p.Comments == nil {
		p.Comments = []*model.Comment{}
	}
	for _, d := range g.Diffs {
		// The raw diff text is already represented by the parsed files;
		// dropping it keeps the page roughly half the size.
		frozen := *d
		frozen.Raw = ""
		p.Diffs = append(p.Diffs, &frozen)

		for _, f := range d.Files {
			key := d.ID + ":" + f.ID
			switch {
			case (f.IsMarkdown || f.IsNotebook) && !f.IsBinary && f.Status != model.StatusDeleted:
				got := source.NewSide(d.BaseDir, f)
				if strings.TrimSpace(got.Content) == "" {
					continue
				}
				p.Previews[key] = Preview{
					Content:  got.Content,
					Source:   string(got.Kind),
					Complete: got.Complete,
					Path:     got.Path,
				}
			case f.IsImage && f.Status != model.StatusDeleted:
				got := source.NewSide(d.BaseDir, f)
				if got.Kind != source.FromWorktree || got.Content == "" {
					continue
				}
				p.Images[key] = Image{
					DataURL: "data:" + diff.ImageContentType(f.Path()) + ";base64," +
						base64.StdEncoding.EncodeToString([]byte(got.Content)),
					Path: got.Path,
				}
			}
		}
	}
	return p
}

// Options tunes the generated page.
type Options struct {
	// Title is the document title; a default is derived from the group.
	Title string
	// Fragment writes only the page body, for embedding into a host page
	// that supplies its own <html> and <head> (a Claude Artifact, a static
	// site, a mail...).
	Fragment bool
}

// Render writes the page. assets is the built UI (the dist tree embedded in
// the sbnn binary).
func Render(payload *Payload, assets fs.FS, opts Options) (string, error) {
	css, js, err := readAssets(assets)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("cannot serialise the review: %w", err)
	}
	title := opts.Title
	if title == "" {
		title = "sbnn review: " + payload.Group
	}

	var b strings.Builder
	if !opts.Fragment {
		b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
		b.WriteString("<meta charset=\"utf-8\">\n")
		b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
		fmt.Fprintf(&b, "<title>%s</title>\n", escapeHTML(title))
		fmt.Fprintf(&b, "<style>\n%s\n</style>\n", css)
		b.WriteString("</head>\n<body>\n")
	} else {
		fmt.Fprintf(&b, "<style>\n%s\n</style>\n", css)
	}

	b.WriteString("<div id=\"root\"></div>\n")
	fmt.Fprintf(&b, "<script>window.__SBNN_DATA__ = %s;</script>\n", escapeJSONForScript(data))
	fmt.Fprintf(&b, "<script type=\"module\">\n%s\n</script>\n", js)

	if !opts.Fragment {
		b.WriteString("</body>\n</html>\n")
	}
	return b.String(), nil
}

// readAssets collects the stylesheet and the script Vite produced.
//
// index.html is the source of truth for what the page loads and in which
// order: the file names are content hashed, and a directory listing says
// nothing about which of them is the entry module. Only what index.html
// references is inlined, and a build that emitted more than one script is
// rejected - see requireSingleChunk.
func readAssets(assets fs.FS) (css, js string, err error) {
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return "", "", fmt.Errorf("the sbnn UI is not built into this binary: %w", err)
	}
	cssRefs, jsRefs := assetRefs(string(index))
	if len(jsRefs) == 0 {
		return "", "", fmt.Errorf("no script found in the embedded UI")
	}
	if err := requireSingleChunk(assets, jsRefs); err != nil {
		return "", "", err
	}

	read := func(ref string) (string, error) {
		name, err := assetPath(ref)
		if err != nil {
			return "", err
		}
		b, err := fs.ReadFile(assets, name)
		if err != nil {
			return "", fmt.Errorf("the embedded UI references %s, which is not in the binary: %w", ref, err)
		}
		return string(b), nil
	}

	var cssParts []string
	for _, ref := range cssRefs {
		part, err := read(ref)
		if err != nil {
			return "", "", err
		}
		cssParts = append(cssParts, part)
	}
	if js, err = read(jsRefs[0]); err != nil {
		return "", "", err
	}
	return strings.Join(cssParts, "\n"), js, nil
}

// requireSingleChunk rejects a code split build.
//
// The exported page inlines the script into one <script type="module">. That
// module resolves no relative import and fetches no chunk, so a second .js
// file - a vendor chunk, a lazily imported route - cannot be reached from it.
// Concatenating the chunks instead only moves the failure to the browser, so
// the export fails here, with the names that made it fail.
func requireSingleChunk(assets fs.FS, jsRefs []string) error {
	seen := map[string]bool{}
	var chunks []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		chunks = append(chunks, name)
	}
	for _, ref := range jsRefs {
		if name, err := assetPath(ref); err == nil {
			add(name)
		}
	}
	// A chunk that index.html does not mention is just as unreachable: it is
	// pulled in by an import inside the entry, which the inlined module has
	// no way to satisfy.
	if entries, err := fs.ReadDir(assets, "assets"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
				add("assets/" + e.Name())
			}
		}
	}
	if len(chunks) > 1 {
		sort.Strings(chunks)
		return fmt.Errorf("the embedded UI is built as %d scripts (%s); the exported page inlines a single module and cannot load a chunk, so the UI has to be built as one chunk",
			len(chunks), strings.Join(chunks, ", "))
	}
	return nil
}

var (
	assetTagRe  = regexp.MustCompile(`(?is)<(script|link)\b([^>]*)>`)
	assetAttrRe = regexp.MustCompile(`(?is)([a-z0-9-]+)\s*=\s*("[^"]*"|'[^']*'|[^\s"'>]+)`)
)

// assetRefs returns the stylesheets and the scripts index.html loads, in
// document order.
func assetRefs(html string) (cssRefs, jsRefs []string) {
	for _, tag := range assetTagRe.FindAllStringSubmatch(html, -1) {
		attrs := assetAttrs(tag[2])
		switch strings.ToLower(tag[1]) {
		case "script":
			// An inline script carries the page's own code, not an asset.
			if src := attrs["src"]; src != "" {
				jsRefs = append(jsRefs, src)
			}
		case "link":
			if !strings.EqualFold(attrs["rel"], "stylesheet") {
				continue
			}
			if href := attrs["href"]; href != "" {
				cssRefs = append(cssRefs, href)
			}
		}
	}
	return cssRefs, jsRefs
}

func assetAttrs(s string) map[string]string {
	attrs := map[string]string{}
	for _, m := range assetAttrRe.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
			v = v[1 : len(v)-1]
		}
		attrs[strings.ToLower(m[1])] = v
	}
	return attrs
}

// assetPath turns a URL from index.html into a path inside the embedded
// tree. An absolute URL cannot be inlined, and the exported page must not
// reach out to the network for it.
func assetPath(ref string) (string, error) {
	clean := ref
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i]
	}
	if strings.HasPrefix(clean, "//") || strings.Contains(clean, "://") {
		return "", fmt.Errorf("the embedded UI references %s, which is not part of the binary", ref)
	}
	clean = strings.TrimPrefix(strings.TrimPrefix(clean, "./"), "/")
	if clean == "" {
		return "", fmt.Errorf("the embedded UI references an empty asset URL")
	}
	return path.Clean(clean), nil
}

// escapeJSONForScript makes JSON safe to inline in a <script> element.
// encoding/json already escapes <, > and & for us; U+2028 and U+2029 are
// valid JSON but terminate a JavaScript string.
func escapeJSONForScript(b []byte) string {
	s := string(b)
	s = strings.ReplaceAll(s, "\u2028", `\u2028`)
	s = strings.ReplaceAll(s, "\u2029", `\u2029`)
	return s
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
