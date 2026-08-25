package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testAssets stands in for the built UI so that the handler is tested
// against file names this test knows, not against whatever the last
// "pnpm build" fingerprinted.
var testAssets = fstest.MapFS{
	"index.html":              &fstest.MapFile{Data: []byte("<!doctype html><title>sbnn</title>\n")},
	"assets/index-abc123.js":  &fstest.MapFile{Data: []byte("console.log(1)\n")},
	"assets/index-abc123.css": &fstest.MapFile{Data: []byte(".a{}\n")},
	"favicon.svg":             &fstest.MapFile{Data: []byte("<svg/>\n")},
}

// useTestAssets points the SPA handler at testAssets for one test.
func useTestAssets(t *testing.T) {
	t.Helper()
	old := spaAssets
	spaAssets = func() (fs.FS, bool) { return testAssets, true }
	t.Cleanup(func() { spaAssets = old })
}

func TestSpaHandler(t *testing.T) {
	useTestAssets(t)
	h := (&Server{}).spaHandler()

	// A path longer than a group name can be, to show it is not treated as
	// an asset just because it is long.
	long := "/" + strings.Repeat("g", 80)

	for _, tt := range []struct {
		name        string
		path        string
		wantStatus  int
		wantType    string
		wantCache   string
		wantBodyHas string
	}{
		{
			name:        "the root renders the SPA",
			path:        "/",
			wantStatus:  http.StatusOK,
			wantType:    "text/html; charset=utf-8",
			wantCache:   "no-cache",
			wantBodyHas: "<!doctype html>",
		},
		{
			name:        "a group renders the SPA",
			path:        "/default",
			wantStatus:  http.StatusOK,
			wantType:    "text/html; charset=utf-8",
			wantBodyHas: "<!doctype html>",
		},
		{
			name:        "an image next to a previewed file is a 404, not the page",
			path:        "/diagram.png",
			wantStatus:  http.StatusNotFound,
			wantType:    "text/plain; charset=utf-8",
			wantBodyHas: "not found: /diagram.png is not part of the sbnn UI",
		},
		{
			name:        "a sibling document is a 404 too",
			path:        "/other.md",
			wantStatus:  http.StatusNotFound,
			wantType:    "text/plain; charset=utf-8",
			wantBodyHas: "/other.md",
		},
		{
			name:       "a missing asset under assets/ is a 404",
			path:       "/assets/index-gone.js",
			wantStatus: http.StatusNotFound,
			wantType:   "text/plain; charset=utf-8",
		},
		{
			name:        "a built asset is served with its long cache",
			path:        "/assets/index-abc123.js",
			wantStatus:  http.StatusOK,
			wantCache:   "public, max-age=31536000, immutable",
			wantBodyHas: "console.log(1)",
		},
		{
			name:        "a built asset outside assets/ is served without it",
			path:        "/favicon.svg",
			wantStatus:  http.StatusOK,
			wantCache:   "",
			wantBodyHas: "<svg/>",
		},
		{
			// Paths without an extension are left alone by this change;
			// which of them the SPA answers is decided separately.
			name:       "a path without an extension still renders the SPA",
			path:       "/a/b/c",
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
		{
			name:       "a long path without an extension still renders the SPA",
			path:       long,
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, res.StatusCode, tt.wantStatus)
			}
			if tt.wantType != "" {
				if got := res.Header.Get("Content-Type"); got != tt.wantType {
					t.Errorf("GET %s content-type = %q, want %q", tt.path, got, tt.wantType)
				}
			}
			if got := res.Header.Get("Cache-Control"); tt.wantCache != "" && got != tt.wantCache {
				t.Errorf("GET %s cache-control = %q, want %q", tt.path, got, tt.wantCache)
			}
			if tt.wantBodyHas != "" && !strings.Contains(rec.Body.String(), tt.wantBodyHas) {
				t.Errorf("GET %s body = %q, want it to contain %q", tt.path, rec.Body.String(), tt.wantBodyHas)
			}
		})
	}
}

func TestSpaHandlerNotBuilt(t *testing.T) {
	old := spaAssets
	spaAssets = func() (fs.FS, bool) { return fstest.MapFS{}, false }
	t.Cleanup(func() { spaAssets = old })

	rec := httptest.NewRecorder()
	(&Server{}).spaHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when the UI is not built in", rec.Code)
	}
}

func TestHasFileExtension(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"diagram.png", true},
		{"docs/diagram.png", true},
		{"other.md", true},
		{"assets/index-abc123.js", true},
		{"default", false},
		{"a/b/c", false},
		{".env", false},
		{"docs/.env", false},
		{"trailing.", false},
		{"", false},
	} {
		if got := hasFileExtension(tt.in); got != tt.want {
			t.Errorf("hasFileExtension(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
