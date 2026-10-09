package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The page reloads the whole group on every change event, and that response
// is the size of the review: 6.1 MB at 400 files, all of it JSON text that
// gzip takes down to a few percent (#391). The compression is done here, for
// the JSON API only, so every client benefits without knowing: a browser and
// the Go client both send Accept-Encoding: gzip on their own and undo the
// encoding on their own.

// compressedPrefix is where compression applies. The event stream
// (/_/events) and the pages and scripts of the UI are outside it.
const compressedPrefix = "/_/api/"

// gzipPool keeps the gzip writers, whose state is large enough to matter at
// one reload per event.
var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// withGzip compresses the text bodies of the API for a client that accepts
// gzip.
//
// It must never touch the event stream: a compressor holds output back until
// it has enough to emit, and an event that waits for the next one is an event
// that does not arrive. The stream lives outside compressedPrefix, and
// gzipResponseWriter.Flush forwards to the compressor first, so a handler
// that flushes still reaches the client when its response is compressed.
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, compressedPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		// The response differs by Accept-Encoding whether or not this request
		// asked for gzip, so a cache has to be told either way.
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r.Header) || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// gzipResponseWriter decides on the first header or write whether the
// response is compressed, because only then are the status and the
// Content-Type known.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (g *gzipResponseWriter) decide(status int) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	if !compressible(status, h) {
		return
	}
	// The length of the plain body is no longer the length on the wire.
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	g.gz = gzipPool.Get().(*gzip.Writer)
	g.gz.Reset(g.ResponseWriter)
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	g.decide(status)
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	g.decide(http.StatusOK)
	if g.gz == nil {
		return g.ResponseWriter.Write(b)
	}
	return g.gz.Write(b)
}

// Flush pushes what has been written so far to the client. A wrapper that
// swallowed it would turn a stream into a buffered response that never
// arrives, so it reaches the compressor and then the connection.
func (g *gzipResponseWriter) Flush() {
	// Flushing sends the header, so the decision cannot wait for a write.
	g.decide(http.StatusOK)
	if g.gz != nil {
		g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipResponseWriter) close() {
	if g.gz == nil {
		return
	}
	g.gz.Close()
	g.gz.Reset(nil)
	gzipPool.Put(g.gz)
	g.gz = nil
}

// compressible reports whether a response is worth compressing: it has a
// body, nobody encoded it already, and it is text. Images and the like are
// compressed formats already.
func compressible(status int, h http.Header) bool {
	switch {
	case status < 200, status == http.StatusNoContent, status == http.StatusNotModified:
		return false
	case h.Get("Content-Encoding") != "":
		return false
	}
	mt, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return false
	}
	switch {
	case strings.HasPrefix(mt, "text/"):
		return mt != "text/event-stream"
	case mt == "application/json", mt == "application/xml",
		strings.HasSuffix(mt, "+json"), strings.HasSuffix(mt, "+xml"):
		return true
	}
	return false
}

// acceptsGzip reads Accept-Encoding the way RFC 9110 says: gzip is accepted
// when it is listed with a non-zero quality, or when "*" is and gzip is not
// refused by name.
func acceptsGzip(h http.Header) bool {
	gzipQ, starQ := -1.0, -1.0
	for _, field := range h.Values("Accept-Encoding") {
		for part := range strings.SplitSeq(field, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			q := 1.0
			if k, v, ok := strings.Cut(strings.TrimSpace(params), "="); ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				if err != nil {
					continue
				}
				q = f
			}
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "gzip", "x-gzip":
				gzipQ = q
			case "*":
				starQ = q
			}
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return starQ > 0
}

// bodyETag is a validator for a response body. It is weak because the same
// representation goes out gzipped to one client and plain to another.
func bodyETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + hex.EncodeToString(sum[:12]) + `"`
}

// etagMatches implements the weak comparison of If-None-Match.
func etagMatches(h http.Header, etag string) bool {
	want := strings.TrimPrefix(etag, "W/")
	for _, field := range h.Values("If-None-Match") {
		for part := range strings.SplitSeq(field, ",") {
			part = strings.TrimSpace(part)
			if part == "*" || strings.TrimPrefix(part, "W/") == want {
				return true
			}
		}
	}
	return false
}

// encodeJSON renders v the way writeJSON does.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := newJSONEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// newJSONEncoder is the one place the API's JSON formatting is decided, for
// writeJSON and for the bodies that are hashed before they are sent.
func newJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}
