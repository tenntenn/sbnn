package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/model"
)

// hub is the event fan-out of the real server, small enough to see: a
// notice published while nobody is subscribed goes nowhere, and there is no
// replay for whoever subscribes next. That is what makes the order of
// subscribing and asking matter.
type hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newHub() *hub { return &hub{subs: map[chan string]struct{}{}} }

func (h *hub) subscribe() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

func (h *hub) publish(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// promptBody is what the stub server prints for the review, so the test can
// count how many times the review was reported.
const promptBody = "the review, printed"

// reviewServer stands in for sbnn with the two endpoints wait reads and the
// event stream it listens on. When reviewedFromStart is false, the review
// is submitted in the exact window the issue describes: the group is asked
// about, the review lands, and only then is the answer - "not reviewed" -
// sent back. Either way the notice is published, so both orders of
// subscribing and asking are exercised against a server that never replays.
func reviewServer(t *testing.T, group string, reviewedFromStart bool) *hub {
	t.Helper()
	h := newHub()

	reviewed := &model.Group{
		Name:          group,
		ReviewedAt:    time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		ReviewVerdict: model.VerdictApproved,
	}
	notice, err := json.Marshal(map[string]any{
		"type": "review", "group": group,
		"reviewedAt": reviewed.ReviewedAt, "comments": 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	submitted := reviewedFromStart

	mux := http.NewServeMux()
	mux.HandleFunc("/_/api/status", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"app": "sbnn"})
	})
	mux.HandleFunc("GET /_/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		// Registered before the headers go out, so a subscriber is live by
		// the time Subscribe returns and the test cannot race itself.
		ch := h.subscribe()
		defer h.unsubscribe(ch)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			}
		}
	})
	mux.HandleFunc("GET /_/api/groups/"+group, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !submitted {
			// The human presses Submit here: after the question, before
			// the answer. Whoever is subscribed hears about it, and this
			// answer still says the review has not happened.
			h.publish(string(notice))
			submitted = true
			json.NewEncoder(w).Encode(&model.Group{Name: group})
			return
		}
		// A review already on the books is announced again, so a client
		// that reads both the group and the stream would report it twice.
		h.publish(string(notice))
		json.NewEncoder(w).Encode(reviewed)
	})
	mux.HandleFunc("GET /_/api/groups/"+group+"/prompt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, promptBody)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	oldBind, oldPort, oldTarget := bind, port, target
	t.Cleanup(func() { bind, port, target = oldBind, oldPort, oldTarget })
	bind, port, target = host, p, group
	return h
}

// A review submitted between the "already reviewed?" question and its
// answer used to be lost: the client was not subscribed when the notice was
// published, nothing is replayed, and sbnn wait then blocked forever on a
// review that had already happened. Subscribing first is what closes it.
func TestWaitDoesNotMissAReviewSubmittedWhileItAsks(t *testing.T) {
	t.Setenv(TargetEnv, "")
	reviewServer(t, "default", false)
	withWaitState(t)

	out := runWaitWithin(t, 10*time.Second)
	if strings.Count(out, promptBody) != 1 {
		t.Errorf("the review was printed %d time(s):\n%s", strings.Count(out, promptBody), out)
	}
}

// The promise the docs make - "a review that was already submitted for the
// diffs sbnn holds returns straight away, so waiting is safe to retry" -
// still holds. The stream is holding a notice for the same review here, so
// this is also where reporting it twice would show.
func TestWaitReturnsForAReviewAlreadySubmitted(t *testing.T) {
	t.Setenv(TargetEnv, "")
	reviewServer(t, "default", true)
	withWaitState(t)

	out := runWaitWithin(t, 10*time.Second)
	if strings.Count(out, promptBody) != 1 {
		t.Errorf("the review was printed %d time(s):\n%s", strings.Count(out, promptBody), out)
	}
}

// runWaitWithin runs the wait command with its stdout captured and fails if
// it has not finished within limit - which is what a lost notice looks like
// from the outside.
func runWaitWithin(t *testing.T, limit time.Duration) string {
	t.Helper()
	read := captureStdout(t)
	// The context is there for the failing case: a wait that never ends
	// holds the event stream open, and httptest.Server.Close waits for its
	// handlers, so the run has to be stopped before the test can report.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		done <- runWait(cmd, nil)
	}()
	select {
	case err := <-done:
		out := read()
		if err != nil {
			t.Fatalf("sbnn wait: %v", err)
		}
		return out
	case <-time.After(limit):
		cancel()
		<-done
		read()
		t.Fatal("sbnn wait blocked on a review that had already been submitted")
		return ""
	}
}

// captureStdout redirects os.Stdout, which is where the review is printed.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	var buf bytes.Buffer
	copied := make(chan struct{})
	go func() {
		io.Copy(&buf, r)
		close(copied)
	}()
	var once sync.Once
	return func() string {
		once.Do(func() {
			os.Stdout = old
			w.Close()
			<-copied
			r.Close()
		})
		return buf.String()
	}
}

// withWaitState sets the flag variables the wait command reads and puts
// them back afterwards.
func withWaitState(t *testing.T) {
	t.Helper()
	oldTimeout, oldFormat, oldJSON, oldExit, oldQuiet := waitTimeout, waitFormat, waitJSON, waitExitCode, waitQuiet
	t.Cleanup(func() {
		waitTimeout, waitFormat, waitJSON, waitExitCode, waitQuiet = oldTimeout, oldFormat, oldJSON, oldExit, oldQuiet
	})
	waitTimeout, waitFormat, waitJSON, waitExitCode, waitQuiet = 0, "prompt", false, false, false
}
