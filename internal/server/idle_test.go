package server

// Tests for the idle timeout: what counts as having nothing left to review,
// that any sign of life resets the clock, and how often the check runs. They
// sit in their own file rather than at the end of server_test.go so that
// other work on internal/server does not have to land in the same place.

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// A detached server was never ended by anything: no idle timeout, no lifetime
// bound, no cleanup on logout, so a review from months ago could still hold a
// port, a session file and its parsed diffs until the machine rebooted. The
// collection has to be conservative - a review waiting for a human is exactly
// what the hooks exist for and must not be collected.
func TestIdleShutdown(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, ts *httptest.Server, srv *Server)
		want  bool // the server should end itself
	}{
		{"nothing to review", func(*testing.T, *httptest.Server, *Server) {}, true},
		{"a diff is still open", func(t *testing.T, ts *httptest.Server, _ *Server) {
			postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
		}, false},
		{"a review already submitted, with the diff still there", func(t *testing.T, ts *httptest.Server, _ *Server) {
			postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
			postJSON(t, ts.URL+"/_/api/groups/default/review", SubmitReviewRequest{Note: "done"}, nil)
		}, false},
		{"a hook is waiting to fire", func(t *testing.T, ts *httptest.Server, srv *Server) {
			if _, err := srv.Store().AddHook(DefaultGroup, &model.Hook{Command: "echo hi"}); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"a page is watching the event stream", func(t *testing.T, _ *httptest.Server, srv *Server) {
			srv.broker.subscribe() // left connected on purpose
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, srv := newTestServer(t, func(o *Options) { o.IdleTimeout = 60 * time.Millisecond })
			tc.setup(t, ts, srv)

			if got := srv.idle(); got != tc.want {
				t.Fatalf("idle() = %v, want %v", got, tc.want)
			}

			ctx := t.Context()
			go srv.watchIdle(ctx, 10*time.Millisecond)

			select {
			case <-srv.shutdown:
				if !tc.want {
					t.Error("the server collected itself with work still open")
				}
			case <-time.After(time.Second):
				if tc.want {
					t.Error("the server stayed up with nothing to review")
				}
			}
		})
	}
}

// The timeout is continuous: work arriving part-way through resets it, so a
// server is only collected after being useless for the whole stretch.
func TestIdleTimeoutIsResetByWork(t *testing.T) {
	ts, srv := newTestServer(t, func(o *Options) { o.IdleTimeout = 150 * time.Millisecond })
	ctx := t.Context()
	go srv.watchIdle(ctx, 10*time.Millisecond)

	// Let it get most of the way to the timeout, then send a diff.
	time.Sleep(100 * time.Millisecond)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)

	select {
	case <-srv.shutdown:
		t.Fatal("the server collected itself after a diff arrived")
	case <-time.After(400 * time.Millisecond):
	}
}

func TestIdleCheckInterval(t *testing.T) {
	cases := []struct {
		timeout time.Duration
		want    time.Duration
	}{
		{30 * time.Minute, 30 * time.Second},
		{2 * time.Minute, 30 * time.Second},
		{40 * time.Second, 10 * time.Second},
		{time.Nanosecond, time.Millisecond},
	}
	for _, tc := range cases {
		if got := idleCheckInterval(tc.timeout); got != tc.want {
			t.Errorf("idleCheckInterval(%s) = %s, want %s", tc.timeout, got, tc.want)
		}
	}
}
