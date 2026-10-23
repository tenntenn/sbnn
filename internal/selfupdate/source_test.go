package selfupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub is an httptest stand-in for both the API and the release
// downloads, holding one release.
type fakeGitHub struct {
	tag      string
	files    map[string][]byte // asset name -> content
	redirect string            // when set, downloads redirect to this base

	mu   sync.Mutex
	auth map[string]string // path -> Authorization header seen
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if f.auth == nil {
		f.auth = map[string]string{}
	}
	f.auth[r.URL.Path] = r.Header.Get("Authorization")
	f.mu.Unlock()

	switch {
	case r.URL.Path == "/repos/"+Repo+"/releases/latest",
		r.URL.Path == "/repos/"+Repo+"/releases/tags/"+f.tag:
		fmt.Fprintf(w, `{"tag_name":%q,"draft":false}`, f.tag)
	case strings.HasPrefix(r.URL.Path, "/"+Repo+"/releases/download/"+f.tag+"/"):
		name := strings.TrimPrefix(r.URL.Path, "/"+Repo+"/releases/download/"+f.tag+"/")
		if f.redirect != "" {
			http.Redirect(w, r, f.redirect+"/assets/"+name+"?sig=secret", http.StatusFound)
			return
		}
		f.serveAsset(w, name)
	case strings.HasPrefix(r.URL.Path, "/assets/"):
		f.serveAsset(w, strings.TrimPrefix(r.URL.Path, "/assets/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGitHub) serveAsset(w http.ResponseWriter, name string) {
	b, ok := f.files[name]
	if !ok {
		http.Error(w, "no such asset", http.StatusNotFound)
		return
	}
	w.Write(b)
}

func (f *fakeGitHub) authFor(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth[path]
}

// testSource points a Source at srv, allowing that server's host and no
// other.
func testSource(srv *httptest.Server, token string) *Source {
	host := strings.TrimPrefix(srv.URL, "http://")
	return &Source{
		APIBase:      srv.URL,
		DownloadBase: srv.URL,
		Token:        token,
		Allow:        func(u *url.URL) bool { return u.Scheme == "http" && u.Host == host },
	}
}

func TestDownload(t *testing.T) {
	const name = "sbnn_Linux_x86_64.tar.gz"
	archive := []byte("archive bytes")
	for _, tt := range []struct {
		name    string
		files   map[string][]byte
		wantErr string
	}{
		{"verified", map[string][]byte{name: archive, "checksums.txt": []byte(sumLine(archive, name))}, ""},
		{"mismatch", map[string][]byte{name: archive, "checksums.txt": []byte(sumLine([]byte("other"), name))}, "does not match"},
		{"not listed", map[string][]byte{name: archive, "checksums.txt": []byte(sumLine(archive, "sbnn_Darwin_arm64.tar.gz"))}, "no entry"},
		{"no checksums.txt", map[string][]byte{name: archive}, "no checksums.txt"},
		{"no archive", map[string][]byte{"checksums.txt": []byte(sumLine(archive, name))}, "has no " + name},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(&fakeGitHub{tag: "v1.0.0", files: tt.files})
			defer srv.Close()
			src := testSource(srv, "")
			rel, err := src.Latest(t.Context())
			if err != nil {
				t.Fatalf("Latest() = %v", err)
			}
			if rel.Tag != "v1.0.0" {
				t.Fatalf("Latest() = %q, want v1.0.0", rel.Tag)
			}
			got, err := src.Download(t.Context(), rel, name)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Download() = %v, want an error containing %q", err, tt.wantErr)
				}
				if got != nil {
					t.Error("Download() returned an archive along with its error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Download() = %v", err)
			}
			if string(got) != string(archive) {
				t.Errorf("Download() = %q", got)
			}
		})
	}
}

func TestTokenOnlyGoesToTheAPI(t *testing.T) {
	const name = "sbnn_Linux_x86_64.tar.gz"
	archive := []byte("archive bytes")
	files := map[string][]byte{name: archive, "checksums.txt": []byte(sumLine(archive, name))}

	assets := &fakeGitHub{tag: "v1.0.0", files: files}
	assetSrv := httptest.NewServer(assets)
	defer assetSrv.Close()
	api := &fakeGitHub{tag: "v1.0.0", files: files}
	apiSrv := httptest.NewServer(api)
	defer apiSrv.Close()

	hosts := map[string]bool{
		strings.TrimPrefix(apiSrv.URL, "http://"):   true,
		strings.TrimPrefix(assetSrv.URL, "http://"): true,
	}
	src := &Source{
		APIBase:      apiSrv.URL,
		DownloadBase: assetSrv.URL,
		Token:        "secret-token",
		Allow:        func(u *url.URL) bool { return hosts[u.Host] },
	}
	rel, err := src.Latest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Download(t.Context(), rel, name); err != nil {
		t.Fatal(err)
	}
	if got := api.authFor("/repos/" + Repo + "/releases/latest"); got != "Bearer secret-token" {
		t.Errorf("the API was sent Authorization %q, want the token", got)
	}
	for _, p := range []string{"checksums.txt", name} {
		if got := assets.authFor("/" + Repo + "/releases/download/v1.0.0/" + p); got != "" {
			t.Errorf("the download of %s was sent Authorization %q", p, got)
		}
	}
}

func TestRedirects(t *testing.T) {
	const name = "sbnn_Linux_x86_64.tar.gz"
	archive := []byte("archive bytes")
	files := map[string][]byte{name: archive, "checksums.txt": []byte(sumLine(archive, name))}

	storage := httptest.NewServer(&fakeGitHub{tag: "v1.0.0", files: files})
	defer storage.Close()
	gh := httptest.NewServer(&fakeGitHub{tag: "v1.0.0", files: files, redirect: storage.URL})
	defer gh.Close()

	ghHost := strings.TrimPrefix(gh.URL, "http://")
	storageHost := strings.TrimPrefix(storage.URL, "http://")

	t.Run("to an allowed host", func(t *testing.T) {
		src := testSource(gh, "")
		src.Allow = func(u *url.URL) bool { return u.Host == ghHost || u.Host == storageHost }
		if _, err := src.Download(t.Context(), Release{Tag: "v1.0.0"}, name); err != nil {
			t.Fatalf("Download() = %v", err)
		}
	})
	t.Run("to any other host", func(t *testing.T) {
		src := testSource(gh, "")
		_, err := src.Download(t.Context(), Release{Tag: "v1.0.0"}, name)
		if err == nil || !strings.Contains(err.Error(), "refusing to follow a redirect") {
			t.Fatalf("Download() = %v, want the redirect refused", err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("the error repeats the signed query: %v", err)
		}
	})
}

func TestAllowGitHub(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want bool
	}{
		{"https://api.github.com/repos/tenntenn/sbnn/releases/latest", true},
		{"https://github.com/tenntenn/sbnn/releases/download/v1.0.0/checksums.txt", true},
		{"https://objects.githubusercontent.com/x", true},
		{"https://release-assets.githubusercontent.com/x?sig=1", true},
		{"https://github.com:443/x", true},
		{"http://github.com/x", false},
		{"https://github.com:8443/x", false},
		{"https://evil.example/x", false},
		{"https://github.com.evil.example/x", false},
		{"https://user:pass@github.com/x", false},
		{"file:///etc/passwd", false},
	} {
		u, err := url.Parse(tt.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := allowGitHub(u); got != tt.want {
			t.Errorf("allowGitHub(%s) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestDefaultSourceRefusesOtherHosts(t *testing.T) {
	// A Source whose bases were changed but whose Allow was not still talks
	// to nothing but GitHub: the httptest server is never reached.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached %s", r.URL)
	}))
	defer srv.Close()
	src := NewSource("")
	src.APIBase = srv.URL
	if _, err := src.Latest(t.Context()); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("Latest() = %v, want a refusal", err)
	}
}

func TestReleaseValidation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		status  int
		wantErr string
	}{
		{"tag is not a version", `{"tag_name":"../../evil"}`, 200, "not a version"},
		{"tag without v", `{"tag_name":"1.0.0"}`, 200, "not a version"},
		{"draft", `{"tag_name":"v1.0.0","draft":true}`, 200, "draft"},
		{"not JSON", `<html>`, 200, "reading the release"},
		{"rate limited", `{}`, http.StatusForbidden, "rate limit"},
		{"too large", `{"tag_name":"v1.0.0","body":"` + strings.Repeat("x", maxAPIResponse) + `"}`, 200, "allowed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()
			_, err := testSource(srv, "").Latest(t.Context())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Latest() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestTagged(t *testing.T) {
	srv := httptest.NewServer(&fakeGitHub{tag: "v1.0.0"})
	defer srv.Close()
	src := testSource(srv, "")
	if rel, err := src.Tagged(t.Context(), "v1.0.0"); err != nil || rel.Tag != "v1.0.0" {
		t.Fatalf("Tagged(v1.0.0) = %v, %v", rel, err)
	}
	if _, err := src.Tagged(t.Context(), "v9.9.9"); err == nil || !strings.Contains(err.Error(), "no release v9.9.9") {
		t.Fatalf("Tagged(v9.9.9) = %v, want no such release", err)
	}
	if _, err := src.Tagged(context.Background(), "v1.0.0/../../x"); err == nil {
		t.Fatal("Tagged() of a path accepted it")
	}
}
