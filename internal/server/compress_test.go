package server

// Tests for the transfer of the group (#391): gzip for the JSON API, the
// event stream left alone, and the ETag that lets an unchanged reload answer
// 304.

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// rawClient does not add Accept-Encoding nor undo a Content-Encoding, so a
// test sees exactly what the server put on the wire.
var rawClient = &http.Client{Transport: &http.Transport{DisableCompression: true}}

func rawGet(t *testing.T, url string, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := rawClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, b
}

func TestGroupIsGzippedByAcceptEncoding(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/g/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	url := ts.URL + "/_/api/groups/g"

	_, plain := rawGet(t, url, nil)
	if !strings.HasPrefix(string(plain), "{") {
		t.Fatalf("plain body is not JSON: %.40q", plain)
	}

	cases := map[string]struct {
		acceptEncoding string
		wantGzip       bool
	}{
		"none":            {"", false},
		"gzip":            {"gzip", true},
		"several":         {"deflate, gzip, br", true},
		"upper case":      {"GZIP", true},
		"quality":         {"gzip;q=0.5", true},
		"refused":         {"gzip;q=0", false},
		"identity":        {"identity", false},
		"other only":      {"deflate, br", false},
		"star":            {"*", true},
		"star refused":    {"*;q=0", false},
		"gzip over star":  {"gzip;q=0, *", false},
		"star beside gzp": {"*;q=0, gzip", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			header := map[string]string{}
			if c.acceptEncoding != "" {
				header["Accept-Encoding"] = c.acceptEncoding
			}
			resp, body := rawGet(t, url, header)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %s", resp.Status)
			}
			if got := resp.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
				t.Errorf("Vary = %q, want Accept-Encoding", got)
			}
			if !c.wantGzip {
				if got := resp.Header.Get("Content-Encoding"); got != "" {
					t.Errorf("Content-Encoding = %q, want none", got)
				}
				if string(body) != string(plain) {
					t.Errorf("body differs from the plain one")
				}
				return
			}
			if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", got)
			}
			if resp.Header.Get("Content-Length") != "" && resp.ContentLength != int64(len(body)) {
				t.Errorf("Content-Length %d does not match the %d bytes sent", resp.ContentLength, len(body))
			}
			zr, err := gzip.NewReader(strings.NewReader(string(body)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(zr)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(plain) {
				t.Errorf("decompressed body differs from the plain one")
			}
		})
	}
}

// The standard client asks for gzip and undoes it on its own, which is what
// the Go client and a browser rely on.
func TestStandardClientReadsGzipTransparently(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/g/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	resp, err := http.Get(ts.URL + "/_/api/groups/g")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !resp.Uncompressed {
		t.Errorf("response was not gzip encoded for a client that accepts it")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "{") {
		t.Errorf("body is not JSON: %.40q", b)
	}
}

// What is not JSON text is left as it is, and so is everything outside the
// API.
func TestOnlyTextOfTheAPIIsCompressed(t *testing.T) {
	cases := map[string]struct {
		path        string
		contentType string
		want        bool
	}{
		"json api":    {"/_/api/groups/g", "application/json; charset=utf-8", true},
		"html api":    {"/_/api/groups/g/diffs/d1/files/f1/preview", "text/html; charset=utf-8", true},
		"plain error": {"/_/api/groups/g", "text/plain; charset=utf-8", true},
		"image api":   {"/_/api/groups/g/diffs/d1/files/f1/image", "image/png", false},
		"svg api":     {"/_/api/groups/g/diffs/d1/files/f1/image", "image/svg+xml", true},
		"stream":      {"/_/api/events", "text/event-stream", false},
		"page":        {"/index.html", "text/html; charset=utf-8", false},
		"script":      {"/assets/app.js", "text/javascript", false},
		"no type":     {"/_/api/groups/g", "", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.contentType != "" {
					w.Header().Set("Content-Type", c.contentType)
				}
				io.WriteString(w, strings.Repeat("x", 2000))
			}))
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, _ := rawGet(t, srv.URL+c.path, map[string]string{"Accept-Encoding": "gzip"})
			if got := resp.Header.Get("Content-Encoding") == "gzip"; got != c.want {
				t.Errorf("compressed = %v, want %v", got, c.want)
			}
		})
	}
}

// The stream must arrive event by event: a compressor holding an event back
// until the next one is a page that never learns of a change.
func TestEventStreamIsNotCompressedAndStillStreams(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		name := "plain"
		if verbose {
			name = "with request log"
		}
		t.Run(name, func(t *testing.T) {
			ts, srv := newTestServer(t, func(o *Options) { o.Verbose = verbose })
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/_/events", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := rawClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding = %q, want none for the event stream", got)
			}
			if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
				t.Fatalf("Content-Type = %q", got)
			}

			lines := make(chan string)
			go func() {
				sc := bufio.NewScanner(resp.Body)
				for sc.Scan() {
					lines <- sc.Text()
				}
				close(lines)
			}()
			next := func(want string) {
				t.Helper()
				timeout := time.After(5 * time.Second)
				for {
					select {
					case l, ok := <-lines:
						if !ok {
							t.Fatalf("stream ended before %q", want)
						}
						if strings.Contains(l, want) {
							return
						}
					case <-timeout:
						t.Fatalf("no %q within 5s: the stream is not flushing", want)
					}
				}
			}
			next("retry: 2000")
			srv.notify("g")
			next(`"group":"g"`)
		})
	}
}

// A handler under the compressing wrapper that flushes must reach the client
// at the flush, not when the response ends.
func TestFlushReachesTheClientThroughGzip(t *testing.T) {
	release := make(chan struct{})
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "second")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/_/api/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := rawClient.Do(req)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" {
		close(release)
		t.Fatalf("Content-Encoding = %q, want gzip", resp.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	got := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(zr, buf)
		got <- err
	}()
	select {
	case err := <-got:
		if err != nil || string(buf) != "first" {
			t.Errorf("first chunk = %q, %v", buf, err)
		}
	case <-time.After(5 * time.Second):
		t.Error("the flushed chunk did not arrive before the handler finished")
	}
	close(release)
	rest, err := io.ReadAll(zr)
	if err != nil || string(rest) != "second" {
		t.Errorf("rest = %q, %v", rest, err)
	}
}

// A flush before the first write sends the header, so the response has to be
// settled as compressed or not at that point.
func TestFlushBeforeAnyWriteStillCompresses(t *testing.T) {
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.(http.Flusher).Flush()
		io.WriteString(w, `{"ok":true}`)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, body := rawGet(t, srv.URL+"/_/api/x", map[string]string{"Accept-Encoding": "gzip"})
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(zr)
	if err != nil || string(got) != `{"ok":true}` {
		t.Errorf("body = %q, %v", got, err)
	}
}

func TestGroupETag(t *testing.T) {
	ts, _ := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/g/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	url := ts.URL + "/_/api/groups/g"

	first, _ := rawGet(t, url, nil)
	etag := first.Header.Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("ETag = %q, want a weak validator", etag)
	}
	if got := first.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	cases := map[string]struct {
		ifNoneMatch string
		acceptGzip  bool
		want        int
	}{
		"same":               {etag, false, http.StatusNotModified},
		"same with gzip":     {etag, true, http.StatusNotModified},
		"strong form":        {strings.TrimPrefix(etag, "W/"), false, http.StatusNotModified},
		"in a list":          {`"other", ` + etag, false, http.StatusNotModified},
		"star":               {"*", false, http.StatusNotModified},
		"different":          {`W/"0000"`, false, http.StatusOK},
		"no validator":       {"", false, http.StatusOK},
		"different with gzp": {`W/"0000"`, true, http.StatusOK},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			header := map[string]string{}
			if c.ifNoneMatch != "" {
				header["If-None-Match"] = c.ifNoneMatch
			}
			if c.acceptGzip {
				header["Accept-Encoding"] = "gzip"
			}
			resp, body := rawGet(t, url, header)
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.want)
			}
			if got := resp.Header.Get("ETag"); got != etag {
				t.Errorf("ETag = %q, want %q", got, etag)
			}
			if c.want == http.StatusNotModified {
				if len(body) != 0 {
					t.Errorf("a 304 carried %d bytes", len(body))
				}
				if got := resp.Header.Get("Content-Encoding"); got != "" {
					t.Errorf("a 304 has Content-Encoding %q", got)
				}
			}
		})
	}

	// A change makes the old validator miss: a stale 304 would leave the page
	// showing a group that no longer exists.
	postJSON(t, ts.URL+"/_/api/groups/g/comments",
		AddCommentRequest{DiffID: "d1", Path: "docs/new.md", Body: "hello", StartLine: 1}, nil)
	resp, _ := rawGet(t, url, map[string]string{"If-None-Match": etag})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status after a comment = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("ETag") == etag {
		t.Errorf("ETag did not change with the comment")
	}
}
