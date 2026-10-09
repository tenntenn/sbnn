package server

// Tests for the scope a change event carries (#403): a comment event says it
// touched only comments, so the page need not fetch every diff again.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// nextChange waits for one change event and returns its scope.
func nextChange(t *testing.T, ch chan event) string {
	t.Helper()
	select {
	case ev := <-ch:
		var got struct {
			Type  string `json:"type"`
			Scope string `json:"scope"`
		}
		if err := json.Unmarshal(ev.data, &got); err != nil {
			t.Fatalf("event %q is not JSON: %v", ev.data, err)
		}
		if got.Type != "change" {
			t.Fatalf("event = %s, want a change", ev.data)
		}
		return got.Scope
	case <-time.After(5 * time.Second):
		t.Fatal("no change event within 5s")
		return ""
	}
}

func TestChangeEventScope(t *testing.T) {
	ts, srv := newTestServer(t)
	ch, ok := srv.broker.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	defer srv.broker.unsubscribe(ch)

	var added AddDiffResponse
	postJSON(t, ts.URL+"/_/api/groups/default/diffs", AddDiffRequest{Content: sampleDiff}, &added)
	if got := nextChange(t, ch); got != "" {
		t.Fatalf("scope of a new diff = %q, want none", got)
	}
	file := added.Diff.Files[0]
	var c model.Comment
	postJSON(t, ts.URL+"/_/api/groups/default/comments", AddCommentRequest{
		DiffID: added.Diff.ID, FileID: file.ID, Path: file.Path(),
		Side: "new", StartLine: 2, EndLine: 2, Body: "x", Snippet: "+new line",
	}, &c)
	if got := nextChange(t, ch); got != scopeComments {
		t.Fatalf("scope of a new comment = %q, want %q", got, scopeComments)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   string
	}{
		{"update a comment", http.MethodPatch, "/comments/" + c.ID, `{"resolved":true}`, scopeComments},
		{"delete a comment", http.MethodDelete, "/comments/" + c.ID, "", scopeComments},
		{"clear comments", http.MethodDelete, "/comments", "", scopeComments},
		{"delete a diff", http.MethodDelete, "/diffs/" + added.Diff.ID, "", ""},
	}
	for _, tc := range cases {
		req, err := http.NewRequest(tc.method, ts.URL+"/_/api/groups/default"+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := nextChange(t, ch); got != tc.want {
			t.Errorf("%s: scope = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A subscriber that lost a notice cannot be told "comments only" any more: the
// one it lost may have been a diff. It gets the full notice until one is
// queued, and then goes back to the scoped ones.
func TestLaggingSubscriberGetsTheFullNotice(t *testing.T) {
	b := newBroker()
	ch, ok := b.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	full := []byte(`{"type":"change","group":"g"}`)
	scoped := []byte(`{"type":"change","group":"g","scope":"comments"}`)

	b.publishChange(full, scoped)
	if got := (<-ch).data; string(got) != string(scoped) {
		t.Fatalf("a subscriber that missed nothing got %s, want the scoped notice", got)
	}

	for range cap(ch) {
		b.publishChange(full, nil)
	}
	b.publishChange(full, nil) // dropped: the queue is full
	for range cap(ch) {
		<-ch
	}

	b.publishChange(full, scoped)
	if got := (<-ch).data; string(got) != string(full) {
		t.Fatalf("after a dropped notice got %s, want the full one", got)
	}
	b.publishChange(full, scoped)
	if got := (<-ch).data; string(got) != string(scoped) {
		t.Fatalf("after catching up got %s, want the scoped one again", got)
	}
}

// Making room for a review notice throws the queued change notices away, and
// the page ignores a review of another group, so what they said changed is
// lost as well: the next notice has to be the full one.
func TestReviewDrainMarksTheSubscriberLagging(t *testing.T) {
	b := newBroker()
	ch, ok := b.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}
	full := []byte(`{"type":"change","group":"g"}`)
	scoped := []byte(`{"type":"change","group":"g","scope":"comments"}`)

	for range cap(ch) {
		b.publishChange(full, nil)
	}
	b.publishReview("other", []byte(`{"type":"review","group":"other"}`))
	for len(ch) > 0 {
		<-ch
	}

	b.publishChange(full, scoped)
	if got := (<-ch).data; string(got) != string(full) {
		t.Fatalf("after a drain got %s, want the full notice", got)
	}
}
