package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenntenn/sbnn/internal/asset"
	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/source"
)

// errNotPreviewable is returned for files there is no preview of.
var errNotPreviewable = errors.New("no preview for this file")

// errNoDeepLink is returned when mo ran without complaining but reported no
// page for the file it was asked to open.
var errNoDeepLink = errors.New("mo gave this file no URL")

// PreviewSource tells where the previewed Markdown came from.
type PreviewSource string

const (
	// SourceWorktree means the file was read from the working tree, so the
	// preview shows the whole file.
	SourceWorktree PreviewSource = "worktree"
	// SourceReconstructed means the Markdown was rebuilt from the diff
	// itself, which is complete only for added files.
	SourceReconstructed PreviewSource = "reconstructed"
)

// PreviewResponse is the payload of the preview endpoint.
type PreviewResponse struct {
	// URL is the frameable URL of the mo page, served through sbnn's preview
	// proxy. It is empty when the proxy could not be started.
	URL string `json:"url"`
	// MoURL is the URL of the same page on the mo server itself, for
	// opening the preview in its own window.
	MoURL string `json:"moUrl"`
	// Path is the file mo was pointed at.
	Path string `json:"path"`
	// Source says whether the preview shows the working tree file or
	// Markdown rebuilt from the diff.
	Source PreviewSource `json:"source"`
	// Complete reports whether the previewed Markdown is the whole file.
	Complete bool `json:"complete"`
}

// FileContentResponse is the payload of the file content endpoint: the new
// side of a file, for a client that renders Markdown itself.
type FileContentResponse struct {
	Path     string        `json:"path"`
	Source   PreviewSource `json:"source"`
	Complete bool          `json:"complete"`
	Content  string        `json:"content"`
	// Assets is the sibling images the Markdown points at, keyed by the
	// reference as the document wrote it. A relative src resolves against
	// the server root, where there is no such file, so the page needs to be
	// told where each one really is before it renders the Markdown - and it
	// is told by internal/asset, which answers the same question for an
	// exported page so that the two draw the same document alike.
	Assets map[string]asset.Entry `json:"assets,omitempty"`
}

// content returns the text of a file without involving mo: the Markdown or
// notebook JSON of a file, for a client that renders it itself. A phone has
// no room for mo's own chrome inside the preview pane, so the browser
// renders Markdown there instead of using mo, and a notebook is never
// something mo can show at all.
func (p *previewer) content(group string, d *model.Diff, f *model.File) (*FileContentResponse, error) {
	if err := previewableText(f); err != nil {
		return nil, err
	}
	got := newSide(d, f)
	if strings.TrimSpace(got.Content) == "" {
		return nil, fmt.Errorf("%w: nothing to preview for %s", errNotPreviewable, f.Path())
	}
	kind := SourceReconstructed
	if got.Kind == source.FromWorktree {
		kind = SourceWorktree
	}
	res := &FileContentResponse{
		Path:     got.Path,
		Source:   kind,
		Complete: got.Complete,
		Content:  got.Content,
	}
	if f.IsMarkdown {
		res.Assets = assetEntries(group, d, f, got.Content)
	}
	return res, nil
}

// assetEntries points each image of a document at the endpoint that hands out
// its bytes, and says of the rest why there is nothing to point at.
//
// The URL carries the path rather than an id of its own: the endpoint resolves
// it through internal/asset again, against the same document, so the only
// paths it will ever serve are the ones this document named and that were
// found inside the directory the diff was sent from.
func assetEntries(group string, d *model.Diff, f *model.File, content string) map[string]asset.Entry {
	refs := asset.Refs(d.BaseDir, f.Path(), content)
	if len(refs) == 0 {
		return nil
	}
	base := "/_/api/groups/" + url.PathEscape(group) +
		"/diffs/" + url.PathEscape(d.ID) +
		"/files/" + url.PathEscape(f.ID) + "/asset?path="
	out := make(map[string]asset.Entry, len(refs))
	for _, r := range refs {
		e := asset.Entry{Path: r.Label(), Status: r.Status, Size: r.Size}
		if r.Status == asset.StatusOK {
			e.URL = base + url.QueryEscape(r.Rel)
		}
		out[r.Src] = e
	}
	return out
}

// asset returns the bytes of one image a Markdown file points at, and the
// content type to serve them as.
//
// rel is trusted for nothing: it is matched against the references of the
// document itself, so a request for a path the document never mentioned -
// or one that internal/asset refused, whether for leaving the tree or for
// being too heavy to show - is a request for a file this endpoint does not
// have.
//
// The gate is previewableMarkdown rather than previewableText because
// resolving a relative image is a Markdown concern and nothing else: it is
// what a document needs to be drawn, and /content lists assets for a
// document only. previewableText now admits every text file - a .go, a .ts,
// a config file - so sharing it here would have made any comment or string
// literal anywhere in the tree that happens to spell out an image reference
// into a way of asking the server for that file's bytes.
func (p *previewer) asset(d *model.Diff, f *model.File, rel string) (data []byte, contentType string, err error) {
	if err := previewableMarkdown(f); err != nil {
		return nil, "", err
	}
	got := newSide(d, f)
	for _, r := range asset.Refs(d.BaseDir, f.Path(), got.Content) {
		if r.Rel != rel || r.Status != asset.StatusOK {
			continue
		}
		b, err := os.ReadFile(r.Path)
		if err != nil {
			return nil, "", fmt.Errorf("%w: cannot read %s", errNotPreviewable, r.Rel)
		}
		return b, diff.ImageContentType(r.Rel), nil
	}
	return nil, "", fmt.Errorf("%w: %s points at no image %q that sbnn can show", errNotPreviewable, f.Path(), rel)
}

// image returns the raw bytes of an image file and the content type to serve
// them as. Unlike Markdown and notebooks, a missing binary cannot be rebuilt
// from the diff - only the working tree copy is ever shown.
func (p *previewer) image(d *model.Diff, f *model.File) (data []byte, contentType string, err error) {
	if err := previewableImage(f); err != nil {
		return nil, "", err
	}
	// The page is told before it asks - model.File.ImageStatus carries the
	// same verdict - so this is the second line of defence rather than the
	// first: a page held open since before the file grew, or anything else
	// asking directly, must not be able to pull bytes past the cap either.
	if status, size := asset.InDiff(d.BaseDir, f); status == asset.StatusTooLarge {
		return nil, "", fmt.Errorf("%w: %s is %s, past the %s an image of the diff is drawn up to",
			errNotPreviewable, f.Path(), byteLimit(size), byteLimit(asset.MaxBytes))
	}
	got := source.NewSide(d.BaseDir, f)
	if got.Kind != source.FromWorktree || got.Content == "" {
		return nil, "", fmt.Errorf("%w: nothing to preview for %s", errNotPreviewable, f.Path())
	}
	return []byte(got.Content), diff.ImageContentType(f.Path()), nil
}

// previewableMarkdown rejects the files mo cannot show: mo is a Markdown
// viewer only.
func previewableMarkdown(f *model.File) error {
	switch {
	case !f.IsMarkdown:
		return fmt.Errorf("%w: %s is not Markdown", errNotPreviewable, f.Path())
	case f.IsBinary:
		return fmt.Errorf("%w: %s is binary", errNotPreviewable, f.Path())
	case f.Status == model.StatusDeleted:
		return fmt.Errorf("%w: %s was deleted", errNotPreviewable, f.Path())
	}
	return nil
}

// previewableText rejects the files sbnn's own renderer has no text preview
// for.
//
// What is left is every text file. Markdown and notebook JSON are rendered
// by the client; anything else - a .go, a .ts, a config file, a script with
// no extension at all - is shown as its own lines. All three want the same
// thing from the server, which is the new side of the file as text, and the
// server has no way to tell a language it has a renderer for from one it
// does not: that is the client's question, and asking it here only meant
// refusing to hand out a file that had already been read from disk.
func previewableText(f *model.File) error {
	switch {
	case f.IsBinary:
		return fmt.Errorf("%w: %s is binary", errNotPreviewable, f.Path())
	case f.Status == model.StatusDeleted:
		return fmt.Errorf("%w: %s was deleted", errNotPreviewable, f.Path())
	}
	return nil
}

// previewableImage rejects the files there is no image preview for. Images
// are expected to be binary, unlike Markdown and notebooks, so IsBinary is
// not itself a reason to refuse one.
func previewableImage(f *model.File) error {
	switch {
	case !f.IsImage:
		return fmt.Errorf("%w: %s is not an image", errNotPreviewable, f.Path())
	case f.Status == model.StatusDeleted:
		return fmt.Errorf("%w: %s was deleted", errNotPreviewable, f.Path())
	}
	return nil
}

// previewer turns a file of a diff into a mo preview.
type previewer struct {
	mo       *mo.Runner
	proxy    *moProxy
	cacheDir string
}

// preview hands the Markdown of f to mo and returns the URLs of the
// resulting page.
func (p *previewer) preview(ctx context.Context, group string, d *model.Diff, f *model.File) (*PreviewResponse, error) {
	if err := previewableMarkdown(f); err != nil {
		return nil, err
	}

	path, source, complete, err := p.resolve(group, d, f)
	if err != nil {
		return nil, err
	}
	res, err := p.mo.Open(ctx, moGroupName(group), path)
	if err != nil {
		return nil, err
	}
	moURL := res.URLFor(path)
	if moURL == "" {
		// mo ran and answered, but listed no page for this file: it
		// skipped it, or it is reporting a path sbnn cannot match up
		// with the one it asked about. Answering 200 with an empty URL
		// would leave the reviewer with a blank frame and leave no
		// trace on the server, so say so instead.
		return nil, fmt.Errorf("%w: %s", errNoDeepLink, path)
	}
	out := &PreviewResponse{
		MoURL:    moURL,
		Path:     path,
		Source:   source,
		Complete: complete,
	}
	if p.proxy != nil {
		out.URL = p.proxy.rewrite(moURL)
	}
	return out, nil
}

// moGroupName keeps sbnn's previews in their own mo group so that they never
// mix with the files the user opened in mo directly.
func moGroupName(group string) string {
	return "sbnn-" + group
}

// resolve returns the path of the Markdown handed to mo. The working tree
// file wins because it is the complete document; when it is missing, the new
// side is rebuilt from the diff and written to the cache directory so that mo
// has a file to open.
func (p *previewer) resolve(group string, d *model.Diff, f *model.File) (path string, src PreviewSource, complete bool, err error) {
	rel := f.Path()
	got := newSide(d, f)
	if got.Kind == source.FromWorktree {
		return got.Path, SourceWorktree, got.Complete, nil
	}
	if strings.TrimSpace(got.Content) == "" {
		return "", "", false, fmt.Errorf("%w: nothing to preview for %s", errNotPreviewable, rel)
	}
	if p.cacheDir == "" {
		return "", "", false, fmt.Errorf("no cache directory to write the reconstructed preview of %s", rel)
	}
	dst := filepath.Join(p.cacheDir, "preview", safeSegment(group), safeSegment(d.ID), safeRelPath(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", "", false, err
	}
	// Rewriting an unchanged file would make mo reload the preview for
	// nothing, so only write when the content actually differs.
	if old, err := os.ReadFile(dst); err != nil || string(old) != got.Content {
		if err := os.WriteFile(dst, []byte(got.Content), 0o600); err != nil {
			return "", "", false, err
		}
	}
	return dst, SourceReconstructed, got.Complete, nil
}

// newSide returns the new side of f, without believing a working tree file
// that is not actually the new side.
//
// The working tree is the better source only when the diff has been applied
// to it. It has not been for the patch sbnn is handed on stdin without ever
// touching the repository - "cat change.patch | sbnn" - and there the file on
// disk is the old side. Handing that back as the whole new side is the one
// answer sbnn must not give: it is wrong and it says so with confidence.
func newSide(d *model.Diff, f *model.File) source.Result {
	got := source.NewSide(d.BaseDir, f)
	if got.Kind != source.FromWorktree || worktreeMatchesNewSide(got.Content, f) {
		return got
	}
	// Fall back to the diff. The result may only be partial, which the
	// caller reports as "rebuilt" and "partial" - an honest half-answer
	// beats a confident wrong one.
	content, complete := diff.Reconstruct(f)
	return source.Result{Content: content, Kind: source.FromDiff, Complete: complete}
}

// worktreeMatchesNewSide reports whether content, read from the working tree,
// looks like the new side of f.
//
// Every context and addition line of every hunk must sit at the new-side line
// number the hunk gives it, counting from Hunk.NewStart. One line out of
// place is enough to conclude that the patch was never applied here.
//
// The question can also have no answer. A hunk that carries no new-side
// numbering says nothing about where its lines belong, and a diff made only
// of such hunks leaves the working tree neither confirmed nor contradicted.
// Not knowing is not the same as knowing it is wrong, so an unanswerable
// check reports a match and the working tree is used, exactly as it was
// before this check existed.
//
// Binary files and files without hunks are accepted as they are: there is
// nothing in the diff to check them against.
func worktreeMatchesNewSide(content string, f *model.File) bool {
	if f == nil || f.IsBinary || len(f.Hunks) == 0 {
		return true
	}
	lines := strings.Split(content, "\n")
	for _, h := range f.Hunks {
		if h.NewStart < 1 {
			// A combined diff - what "git show <merge>" prints for a
			// merge commit - has "@@@ -1,2 -1,2 +1,2 @@@" headers that
			// this parser deliberately does not read numbers out of,
			// because they do not describe a two-way view. NewStart is
			// then 0 and there is no line to compare against any line.
			// A deleted file's "+0,0" hunk lands here too, and has no
			// new side to check either.
			continue
		}
		num := h.NewStart
		for _, l := range h.Lines {
			if l.Kind == model.LineDelete {
				// A deleted line is not on the new side at all.
				continue
			}
			if num < 1 || num > len(lines) {
				return false
			}
			if trimLineEnd(lines[num-1]) != trimLineEnd(l.Content) {
				return false
			}
			num++
		}
	}
	return true
}

// trimLineEnd drops the carriage return of a CRLF file so that the line
// endings alone never decide that a patch was not applied.
func trimLineEnd(line string) string {
	return strings.TrimRight(line, "\r")
}

// safeRelPath makes a diff path usable inside the cache directory.
func safeRelPath(rel string) string {
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	parts := strings.Split(cleaned, string(filepath.Separator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".", "..":
			continue
		}
		if filepath.VolumeName(part) != "" {
			continue
		}
		out = append(out, safeSegment(part))
	}
	if len(out) == 0 {
		return "preview.md"
	}
	return filepath.Join(out...)
}

// safeSegment strips characters that must not end up in a path component.
func safeSegment(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == filepath.Separator, r == '/', r == '\\', r == ':':
			return '_'
		case r < 0x20:
			return '_'
		}
		return r
	}, s)
}
