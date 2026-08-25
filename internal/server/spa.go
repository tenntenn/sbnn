package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/web"
)

// uiNotBuiltMessage is served when the review UI was not built into this
// binary. It is the one moment the reader most needs a command that works,
// so it names the build system the repository actually has: there is no
// Makefile, and Taskfile.yml defines the "build" task.
const uiNotBuiltMessage = "the sbnn web UI is not built into this binary.\n" +
	"Run `task build` (it runs `pnpm build` in web/) and reinstall sbnn.\n"

// spaHandler serves the review UI. Every path that is not an asset renders
// the SPA, so that "/" and "/<group>" both work.
func (s *Server) spaHandler() http.Handler {
	assets := web.FS()
	if !web.Built() {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, uiNotBuiltMessage)
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
