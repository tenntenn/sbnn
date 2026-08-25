package server

// Tests for the request log: it stays quiet unless SBNN_LOG or --verbose asks
// for it, and what it writes when it does. They sit in their own file rather
// than at the end of server_test.go so that other work on internal/server
// does not have to land in the same place.

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The server runs detached and its log holds one line - "serving at ..." - and
// nothing else, forever. Nothing records that a request arrived, so a
// background process that misbehaves leaves no artefact to diagnose it from.
// The log has to stay quiet by default, because nothing rotates the file.
func TestRequestLog(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		verbose bool
		want    bool
	}{
		{"quiet by default", "", false, false},
		{"a level that a per-request line would drown", "warn", false, false},
		{"SBNN_LOG=info", "info", false, true},
		{"SBNN_LOG=debug", "debug", false, true},
		{"SBNN_LOG is not case sensitive", "Info", false, true},
		{"the flag, on a server started with it", "", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SBNN_LOG", tc.env)
			var buf strings.Builder
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
			t.Cleanup(func() { slog.SetDefault(prev) })

			ts, _ := newTestServer(t, func(o *Options) { o.Verbose = tc.verbose })
			getJSON(t, ts.URL+"/_/api/status", nil)

			got := buf.String()
			if logged := strings.Contains(got, "/_/api/status"); logged != tc.want {
				t.Fatalf("request logged = %v, want %v: %q", logged, tc.want, got)
			}
			if !tc.want {
				return
			}
			for _, field := range []string{"method=GET", "path=/_/api/status", "status=200", "duration="} {
				if !strings.Contains(got, field) {
					t.Errorf("log line is missing %s: %q", field, got)
				}
			}
		})
	}
}

// A cross-origin refusal is exactly the kind of thing the log exists for, so
// the request line has to sit outside the guard rather than inside it.
func TestRequestLogRecordsRefusals(t *testing.T) {
	t.Setenv("SBNN_LOG", "info")
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ts, _ := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/_/api/groups/default/hooks",
		strings.NewReader(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %s, want 403", resp.Status)
	}
	if got := buf.String(); !strings.Contains(got, "status=403") {
		t.Errorf("the refusal was not logged as a request: %q", got)
	}
}

// The wrapper the request log puts around the ResponseWriter must not break
// the event stream: handleEvents asserts http.Flusher and gives up without
// one, so a wrapper that swallowed Flush would leave the stream buffered and
// never delivered.
func TestRequestLogKeepsTheEventStreamFlushing(t *testing.T) {
	t.Setenv("SBNN_LOG", "info")
	ts, srv := newTestServer(t)
	go func() {
		for range 40 {
			if srv.broker.count() > 0 {
				srv.broker.publishReview("default", []byte(`{"type":"review","group":"default"}`))
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/_/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s, want 200", resp.Status)
	}

	var got string
	buf := make([]byte, 4096)
	for !strings.Contains(got, `"type":"review"`) {
		n, err := resp.Body.Read(buf)
		got += string(buf[:n])
		if err != nil {
			break
		}
	}
	if !strings.Contains(got, `"type":"review"`) {
		t.Errorf("nothing was flushed down the stream: %q", got)
	}
}
