package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// newHookServer is a server that is never listened on: the hook tests only
// need one to hang the hook methods off.
func newHookServer(t *testing.T) *Server {
	t.Helper()
	srv, err := New(Options{
		Port:        6280,
		SessionFile: filepath.Join(t.TempDir(), "session.json"),
		CacheDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

// captureLogs points the default logger at a buffer for the length of the
// test, so that a warning meant for a human can be asserted on.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// A hook URL that is not http or https is one postHook can never deliver
// to, so it has to be recognisable as such without trying.
func TestValidateHookURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"http", "http://x/y", true},
		{"https", "https://x/y", true},
		{"host and port", "http://localhost:9000/hooks", true},
		{"prose", "not a url", false},
		{"file", "file:///etc/passwd", false},
		{"no host", "http://", false},
		{"empty", "", false},
		{"scheme relative", "//example.com/hooks", false},
		{"path only", "/hooks", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHookURL(tc.url)
			if tc.ok && err != nil {
				t.Errorf("validateHookURL(%q) = %v, want no error", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("validateHookURL(%q) = nil, want an error", tc.url)
			}
		})
	}
}

// A URL that cannot be delivered to is not worth building a request for.
// The warning is the only trace it leaves, so it has to name which hook and
// which URL.
func TestPostHookRefusesAUrlItCannotDeliver(t *testing.T) {
	logs := captureLogs(t)
	s := newHookServer(t)
	h := &model.Hook{ID: "h5", URL: "file:///etc/passwd"}

	s.postHook(t.Context(), h, ReviewEvent{Group: "api"})

	got := logs.String()
	if !strings.Contains(got, "unusable url") {
		t.Errorf("no warning for an undeliverable hook url:\n%s", got)
	}
	for _, want := range []string{"h5", "file:///etc/passwd"} {
		if !strings.Contains(got, want) {
			t.Errorf("the warning does not name %q:\n%s", want, got)
		}
	}
}

// The response body is read before it is closed so that the connection goes
// back to the pool. Two deliveries to the same endpoint should therefore
// arrive on one connection, not two.
func TestPostHookDrainsSoTheConnectionIsReused(t *testing.T) {
	peers := make(chan string, 2)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		peers <- r.RemoteAddr
		w.Write([]byte(`{"received":true}`))
	}))
	defer ts.Close()

	s := newHookServer(t)
	h := &model.Hook{ID: "h1", URL: ts.URL}
	s.postHook(t.Context(), h, ReviewEvent{Group: "api"})
	s.postHook(t.Context(), h, ReviewEvent{Group: "api"})

	var first, second string
	for _, into := range []*string{&first, &second} {
		select {
		case *into = <-peers:
		case <-time.After(10 * time.Second):
			t.Fatal("the hook was never delivered")
		}
	}
	if first != second {
		t.Errorf("the hook connection was not reused (%s then %s): "+
			"the response body is closed without being read", first, second)
	}
}

// Draining must not mean reading whatever the far end feels like sending.
// A hook endpoint answering with megabytes should cost sbnn one bounded
// read, not all of them.
func TestPostHookBoundsTheResponseItReads(t *testing.T) {
	const maxBody = 16 << 20
	chunk := bytes.Repeat([]byte("x"), 32<<10)
	written := make(chan int, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		n := 0
		for n < maxBody {
			c, err := w.Write(chunk)
			n += c
			if err != nil {
				break
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		written <- n
	}))
	defer ts.Close()

	s := newHookServer(t)
	h := &model.Hook{ID: "h1", URL: ts.URL}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.postHook(t.Context(), h, ReviewEvent{Group: "api"})
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("postHook is still reading a response body it should have stopped reading")
	}

	select {
	case n := <-written:
		// Socket buffers mean the far end gets a good deal further
		// than the bound before its write fails; what matters is
		// that it is nowhere near having sent the lot.
		if n >= maxBody/2 {
			t.Errorf("the hook endpoint got to send %d bytes of %d: the response is not bounded", n, maxBody)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the hook endpoint is still writing")
	}
}
