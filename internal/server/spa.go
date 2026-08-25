package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/web"
)

// spaAssets returns the built review UI and whether it was built into this
// binary at all. It is a variable so that a test can serve the handler a
// known set of files instead of whatever the last "pnpm build" produced.
var spaAssets = func() (fs.FS, bool) { return web.FS(), web.Built() }

// spaHandler serves the review UI: the built assets, and the SPA itself for
// the paths the SPA is responsible for - "/" and "/<group>".
func (s *Server) spaHandler() http.Handler {
	assets, built := spaAssets()
	if !built {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "the sbnn web UI is not built into this binary.\n"+
				"Run `make build` (it runs `pnpm build` in web/) and reinstall sbnn.\n")
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if f, err := assets.Open(name); err == nil {
				defer f.Close()
				if st, err := f.Stat(); err == nil && !st.IsDir() {
					if strings.HasPrefix(name, "assets/") {
						// Vite fingerprints these file names.
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					http.ServeContent(w, r, name, st.ModTime(), f.(io.ReadSeeker))
					return
				}
			}
			// A trailing slash names the same page: groupFromLocation
			// in web/src/api.ts strips leading and trailing slashes, so
			// the SPA reads "/default/" as the group "default" and the
			// server has to hand it the page to say so.
			path := strings.TrimSuffix(name, "/")
			// A path that names a file is a request for that file. The
			// SPA is not it, and answering with the page hides a broken
			// link behind "200 text/html": the browser asked for an
			// image, was handed a document, and drew a broken image icon
			// with nothing in devtools to explain it.
			//
			// Everything else is the page only if the SPA can make sense
			// of it. It reads a single segment as a group name, so a
			// relative link to ./notes or ../a/b used to boot a second
			// copy of sbnn showing "Waiting for a diff" - a plausible
			// page with nothing to do with the link that was clicked.
			if path != "" && (hasFileExtension(path) || !isSpaGroupPath(path)) {
				spaNotFound(w, r)
				return
			}
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "index.html is missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(index)))
	})
}

// hasFileExtension reports whether the last segment of a request path names a
// file: it carries a dot that is neither the first nor the last character of
// the segment, and what follows that dot looks like an extension - at most
// eight ASCII letters and digits, at least one of them a letter. A dotfile
// ("/.env") and a segment that merely ends in a dot are not extensions.
//
// The "at least one letter" rule is what keeps a group name off this path.
// ValidateGroupName accepts a dot, so "v1.2", "rel-1.0", "release.2" and
// "release-2024.01" are names sbnn itself prints review URLs for, and a plain
// dot test would answer every one of those pages - and every GroupURL and
// group-switcher href built from them - with a 404. What separates them from
// "diagram.png" is that their final dot is followed by a version number
// rather than by a file type.
//
// The ambiguity is real and cannot be resolved from the path alone: a group
// literally named "notes.md" is unreachable this way. Version-shaped names
// are the ones that occur, so they are the ones this resolves in favour of.
func hasFileExtension(name string) bool {
	last := name[strings.LastIndex(name, "/")+1:]
	dot := strings.LastIndex(last, ".")
	if dot <= 0 || dot == len(last)-1 {
		return false
	}
	ext := last[dot+1:]
	if len(ext) > 8 {
		return false
	}
	letter := false
	for i := 0; i < len(ext); i++ {
		switch c := ext[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			letter = true
		case c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return letter
}

// isSpaGroupPath reports whether a path is one the SPA is responsible for: a
// single segment, with an optional trailing slash, that ValidateGroupName
// accepts.
//
// It asks ValidateGroupName rather than matching a pattern of its own. The
// two had drifted: a copied regexp here still accepted names this handler
// then refused, so the answer to "is this a group?" depended on whether the
// caller was /_/api/groups/{group} or the page. There is now one answer.
//
// Whether that group exists is deliberately not asked. A page can be opened
// before any diff has been sent to it, and refusing the path until a diff
// arrives would break that.
func isSpaGroupPath(name string) bool {
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	_, err := ValidateGroupName(name)
	return err == nil
}

// spaNotFound says, in a line a person reading devtools can act on, that the
// path is not part of the UI.
func spaNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, "not found: "+r.URL.Path+" is not part of the sbnn UI\n")
}
