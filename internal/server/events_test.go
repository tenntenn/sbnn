package server

// Tests for the event stream: the subscriber cap, the delivery guarantee the
// review notice now has, and the Last-Event-ID replay. They sit in their own
// file rather than at the end of server_test.go so that other work on
// internal/server does not have to land in the same place.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The event stream had no cap and no delivery guarantee. Both bite the same
// way: /_/events is a GET, so the cross-origin guard deliberately lets it
// through - CORS stops another page reading the events, not holding the
// connection open - and every accepted connection costs a goroutine, a
// channel and a ticker until something gives.
func TestEventSubscribersAreCapped(t *testing.T) {
	ts, srv := newTestServer(t)

	for i := range maxSubscribers {
		if _, ok := srv.broker.subscribe(); !ok {
			t.Fatalf("subscriber %d refused below the cap of %d", i, maxSubscribers)
		}
	}
	if _, ok := srv.broker.subscribe(); ok {
		t.Errorf("subscriber %d accepted, want a refusal at the cap", maxSubscribers+1)
	}

	// Over HTTP the refusal is a 503, not a 200 that ends immediately.
	resp, err := http.Get(ts.URL + "/_/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %s, want 503", resp.Status)
	}
}

// A review notice is what wakes `sbnn wait`, and it is typically the last
// event of a burst. Dropping it because the subscriber is behind meant the
// waiter blocked forever on a review that had already been submitted - the
// old "a slow browser refetches on the next event anyway" reasoning only
// holds while more events are still coming.
func TestReviewNoticeSurvivesABehindSubscriber(t *testing.T) {
	b := newBroker()
	ch, ok := b.subscribe()
	if !ok {
		t.Fatal("subscribe refused on an empty broker")
	}

	// Fall far enough behind that the queue is full of change notices.
	for range cap(ch) * 2 {
		b.publishChange([]byte(`{"type":"change","group":"default"}`))
	}
	if len(ch) != cap(ch) {
		t.Fatalf("queued %d change notices, want the queue full at %d", len(ch), cap(ch))
	}

	b.publishReview("default", []byte(`{"type":"review","group":"default"}`))

	var got *event
	for range cap(ch) {
		select {
		case ev := <-ch:
			if ev.id != 0 {
				got = &ev
			}
		default:
		}
	}
	if got == nil {
		t.Fatal("the review notice was dropped; `sbnn wait` would block on a review that already happened")
	}
	if !strings.Contains(string(got.data), `"type":"review"`) {
		t.Errorf("event data = %q", got.data)
	}
}

// The other half of the guarantee: a client that missed the notice while
// catching up gets it when it reconnects, which is what SSE id:/Last-Event-ID
// is for.
func TestMissedReviewNoticesAreReplayed(t *testing.T) {
	b := newBroker()
	b.publishReview("default", []byte(`{"type":"review","group":"default"}`))
	b.publishReview("api", []byte(`{"type":"review","group":"api"}`))

	fresh := b.missedReviews(0)
	if len(fresh) != 2 {
		t.Fatalf("got %d notices for a client that has seen nothing, want both groups", len(fresh))
	}
	if fresh[0].id >= fresh[1].id {
		t.Errorf("ids = %d, %d, want oldest first", fresh[0].id, fresh[1].id)
	}

	if caught := b.missedReviews(fresh[1].id); len(caught) != 0 {
		t.Errorf("got %d notices for a caught-up client, want none", len(caught))
	}
	if partial := b.missedReviews(fresh[0].id); len(partial) != 1 || partial[0].id != fresh[1].id {
		t.Errorf("partial replay = %+v, want only the newer notice", partial)
	}

	// A later review in a group replaces the stored one rather than piling up.
	b.publishReview("default", []byte(`{"type":"review","group":"default","again":true}`))
	if all := b.missedReviews(0); len(all) != 2 {
		t.Errorf("got %d stored notices, want one per group", len(all))
	}
}

// The stream announces its own reconnect delay and numbers review notices, so
// a browser that drops the connection resumes from where it left off. The
// replay is keyed on Last-Event-ID: the browser says what it already has.
func TestEventStreamReplaysForAReconnectingClient(t *testing.T) {
	ts, srv := newTestServer(t)
	srv.broker.publishReview("default", []byte(`{"type":"review","group":"default"}`))
	srv.broker.publishReview("api", []byte(`{"type":"review","group":"api"}`))

	// The client already has notice 1, so only notice 2 is news.
	got := readEventStream(t, ts.URL, "1", `"group":"api"`)
	if !strings.Contains(got, "retry: 2000") {
		t.Errorf("stream = %q, want a retry: field", got)
	}
	if !strings.Contains(got, "id: 2") {
		t.Errorf("stream = %q, want the missed review notice replayed with its id", got)
	}
	if strings.Contains(got, `"group":"default"`) {
		t.Errorf("stream = %q, want no replay of the notice the client already had", got)
	}
}

// A client opening the stream for the first time sends no Last-Event-ID and
// must be replayed nothing.
//
// Regression: replaying every group's last review to a fresh subscriber made
// `sbnn wait` return a review that was submitted before it was asked to wait.
// Approve a group, send another diff, and `sbnn wait --timeout 8s` returned
// exit 0 with the old approval in 8ms instead of blocking for 8s and exiting
// 2 - so a pipeline of `sbnn wait && git commit` committed a diff nobody had
// looked at.
func TestEventStreamReplaysNothingToAFreshClient(t *testing.T) {
	ts, srv := newTestServer(t)
	srv.broker.publishReview("default", []byte(`{"type":"review","group":"default"}`))

	// No Last-Event-ID. Read until the ping deadline rather than until a
	// notice arrives, because the point is that none does.
	got := readEventStream(t, ts.URL, "", "")
	if !strings.Contains(got, "retry: 2000") {
		t.Errorf("stream = %q, want the stream to have opened", got)
	}
	if strings.Contains(got, `"type":"review"`) {
		t.Errorf("stream = %q, want no review notice: this client has missed nothing", got)
	}
}

// The stored notice goes away with the group, so a reconnecting browser is
// not told about a review of something that no longer exists.
func TestClosingAGroupForgetsItsReviewNotice(t *testing.T) {
	ts, srv := newTestServer(t)
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, nil)
	srv.broker.publishReview("default", []byte(`{"type":"review","group":"default"}`))
	if got := len(srv.broker.missedReviews(0)); got != 1 {
		t.Fatalf("stored notices = %d, want 1", got)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/_/api/groups/default", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %s, want 204", resp.Status)
	}

	if got := srv.broker.missedReviews(0); len(got) != 0 {
		t.Errorf("stored notices after closing the group = %+v, want none", got)
	}
}

// readEventStream opens /_/events, optionally reporting lastEventID, and
// returns what the server wrote. It stops as soon as want appears, or after a
// short deadline when want is empty.
func readEventStream(t *testing.T, base, lastEventID, want string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/_/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// The handler flushes retry: as soon as the stream opens and any replay
	// right after it, so a short read window sees everything there is.
	if want == "" {
		time.AfterFunc(300*time.Millisecond, cancel)
	}
	var got string
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		got += string(buf[:n])
		if err != nil {
			return got
		}
		if want != "" && strings.Contains(got, want) {
			return got
		}
	}
}
