package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// newHookServer is a server that is never listened on: the hook tests only
// need one to read BaseURL and the port out of.
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

func reviewedGroup(v model.Verdict, hooks ...*model.Hook) *model.Group {
	return &model.Group{
		Name:          "api",
		ReviewedAt:    time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		ReviewNote:    "one thing",
		ReviewVerdict: v,
		Comments: []*model.Comment{
			{Path: "main.go", Side: "new", StartLine: 4, EndLine: 4, Body: "a remark"},
		},
		Hooks: hooks,
	}
}

// The verdict is the whole reason a hook does not have to count comments,
// so it has to survive the trip into the JSON a URL hook is posted.
func TestRunHooksPostsTheVerdict(t *testing.T) {
	cases := []struct {
		name    string
		verdict model.Verdict
		want    string
	}{
		{"approved", model.VerdictApproved, "approved"},
		{"changes requested", model.VerdictChangesRequested, "changes-requested"},
		{"commented", model.VerdictCommented, "commented"},
		{"none", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodies := make(chan []byte, 1)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				bodies <- b
			}))
			defer ts.Close()

			s := newHookServer(t)
			s.runHooks(reviewedGroup(tc.verdict, &model.Hook{ID: "h1", URL: ts.URL}))

			var body []byte
			select {
			case body = <-bodies:
			case <-time.After(10 * time.Second):
				t.Fatal("hook was never delivered")
			}

			// The field is always present, even when there is no
			// verdict: a hook should not have to tell "absent"
			// from "the server is too old to send it".
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("event is not JSON: %v\n%s", err, body)
			}
			if _, ok := raw["verdict"]; !ok {
				t.Errorf("event has no \"verdict\" field:\n%s", body)
			}
			var event ReviewEvent
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatal(err)
			}
			if string(event.Verdict) != tc.want {
				t.Errorf("verdict = %q, want %q", event.Verdict, tc.want)
			}
		})
	}
}

// SBNN_BLOCKING is the rule every hook would otherwise re-implement, and
// SBNN_VERDICT is spelled the way the JSON spells it so that a script can
// compare the two without a translation table.
//
// SBNN_BLOCKING first shipped as Verdict.Blocking(), which is only half the
// rule: it says a review that merely commented never blocks, while sbnn
// itself exits 1 for that review as long as it left a comment open. A hook
// branching on SBNN_BLOCKING and a pipeline branching on sbnn's exit status
// then disagreed about the same review. Both now go through model.Blocks,
// and this table is the table model.Blocks is tested against.
func TestHookEnvCarriesTheVerdict(t *testing.T) {
	open := []*model.Comment{{Body: "a remark"}}
	settled := []*model.Comment{{Body: "a remark", Resolved: true}}

	cases := []struct {
		name         string
		verdict      model.Verdict
		comments     []*model.Comment
		wantVerdict  string
		wantBlocking string
	}{
		{"approved", model.VerdictApproved, nil, "approved", "0"},
		{"an approval with a remark on it is still an approval",
			model.VerdictApproved, open, "approved", "0"},
		{"changes requested", model.VerdictChangesRequested, nil, "changes-requested", "1"},
		{"changes requested blocks with nothing left open",
			model.VerdictChangesRequested, settled, "changes-requested", "1"},
		// The one the two rules disagreed about.
		{"commented, with a comment still open",
			model.VerdictCommented, open, "commented", "1"},
		{"commented, everything resolved", model.VerdictCommented, settled, "commented", "0"},
		{"commented, nothing said at all", model.VerdictCommented, nil, "commented", "0"},
		{"no verdict, with a comment still open", "", open, "", "1"},
		{"no verdict, nothing left open", "", settled, "", "0"},
	}
	s := newHookServer(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envMap(s.hookEnv(ReviewEvent{
				Group:    "api",
				URL:      "http://localhost:6280/g/api",
				Note:     "one thing",
				Verdict:  tc.verdict,
				Comments: tc.comments,
			}))
			if got := env["SBNN_VERDICT"]; got != tc.wantVerdict {
				t.Errorf("SBNN_VERDICT = %q, want %q", got, tc.wantVerdict)
			}
			if got := env["SBNN_BLOCKING"]; got != tc.wantBlocking {
				t.Errorf("SBNN_BLOCKING = %q, want %q", got, tc.wantBlocking)
			}
			// What a hook is told and what sbnn itself ends on are the
			// same question, so they are answered by the same call.
			want := "0"
			if model.Blocks(tc.verdict, tc.comments) {
				want = "1"
			}
			if got := env["SBNN_BLOCKING"]; got != want {
				t.Errorf("SBNN_BLOCKING = %q, but model.Blocks says %q", got, want)
			}
		})
	}
}

// The six that were there before are what every existing hook is written
// against. Adding to them must not rename or drop any of them.
func TestHookEnvKeepsTheOlderVariables(t *testing.T) {
	s := newHookServer(t)
	env := envMap(s.hookEnv(ReviewEvent{
		Group:    "api",
		URL:      "http://localhost:6280/g/api",
		Note:     "one thing",
		Verdict:  model.VerdictApproved,
		Comments: []*model.Comment{{Body: "a"}, {Body: "b"}},
	}))
	want := map[string]string{
		"SBNN_GROUP":       "api",
		"SBNN_URL":         "http://localhost:6280/g/api",
		"SBNN_SERVER":      s.BaseURL(),
		"SBNN_PORT":        "6280",
		"SBNN_COMMENTS":    "2",
		"SBNN_REVIEW_NOTE": "one thing",
	}
	for k, w := range want {
		if got, ok := env[k]; !ok {
			t.Errorf("%s is gone from the hook environment", k)
		} else if got != w {
			t.Errorf("%s = %q, want %q", k, got, w)
		}
	}
}

// The environment is only useful if it reaches the shell, so run one.
func TestRunHookCommandExportsTheVerdict(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command hook is run through cmd on windows")
	}
	out := filepath.Join(t.TempDir(), "env.txt")
	s := newHookServer(t)
	h := &model.Hook{ID: "h1", Command: "env > '" + out + "'"}
	s.runHookCommand(t.Context(), h, ReviewEvent{
		Group:   "api",
		Verdict: model.VerdictChangesRequested,
	})

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the hook command did not run: %v", err)
	}
	env := envMap(strings.Split(strings.TrimRight(string(b), "\n"), "\n"))
	if got := env["SBNN_VERDICT"]; got != "changes-requested" {
		t.Errorf("SBNN_VERDICT = %q, want %q", got, "changes-requested")
	}
	if got := env["SBNN_BLOCKING"]; got != "1" {
		t.Errorf("SBNN_BLOCKING = %q, want %q", got, "1")
	}
}

func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[k] = v
		}
	}
	return out
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
		{"a bare host, which is what a typo looks like", "localhost:9000/hooks", false},
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

// Registration is the last moment anyone is there to read a mistake. A URL
// that cannot be delivered to was answered 200 and stored forever, so the
// listing showed a healthy hook and every review failed into a log file.
func TestAddHookRefusesAUrlItCannotDeliver(t *testing.T) {
	cases := []struct {
		name string
		hook model.Hook
		want int
	}{
		{"prose", model.Hook{URL: "not a url"}, http.StatusBadRequest},
		{"a scheme postHook cannot speak", model.Hook{URL: "file:///etc/passwd"}, http.StatusBadRequest},
		{"no host", model.Hook{URL: "http://"}, http.StatusBadRequest},
		{"a path, not a url", model.Hook{URL: "/hooks"}, http.StatusBadRequest},
		{"http", model.Hook{URL: "http://localhost:9000/hooks"}, http.StatusOK},
		{"https", model.Hook{URL: "https://example.com/hooks"}, http.StatusOK},
		// The command half cannot be checked without running it, so it
		// stays as permissive as it was.
		{"a command alone", model.Hook{Command: "echo one"}, http.StatusOK},
		{"nothing at all", model.Hook{}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, _ := newTestServer(t)
			resp := postJSON(t, ts.URL+"/_/api/groups/default/hooks", tc.hook, nil)
			if resp.StatusCode != tc.want {
				t.Errorf("POST hooks %+v: status = %d, want %d", tc.hook, resp.StatusCode, tc.want)
			}
			// A refusal must not leave the hook behind either.
			var hooks []*model.Hook
			getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
			wantStored := 0
			if tc.want == http.StatusOK {
				wantStored = 1
			}
			if len(hooks) != wantStored {
				t.Errorf("%d hooks stored, want %d", len(hooks), wantStored)
			}
		})
	}
}

// A session file written before registration checked URLs still holds one,
// and it is loaded back on start. Delivery keeps its own guard, and says
// which hook and which URL, because the log line is all it leaves.
func TestPostHookRefusesAUrlItCannotDeliver(t *testing.T) {
	logs := captureLogs(t)
	s := newHookServer(t)
	h := &model.Hook{ID: "h5", URL: "file:///etc/passwd"}

	run := s.postHook(t.Context(), h, ReviewEvent{Group: "api"})

	if run.OK {
		t.Error("postHook reported an undeliverable url as delivered")
	}
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

// The outcome has to survive to the listing, because that is where the
// reviewer looks. A hook that has failed every round since it was registered
// used to list back looking exactly like one that works.
func TestRunHookRecordsTheOutcome(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Write([]byte(`{"received":true}`))
	}))
	defer ok.Close()
	refuses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer refuses.Close()
	// A URL that parses and has a host, but nothing is listening on it.
	gone := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	goneURL := gone.URL
	gone.Close()

	cases := []struct {
		name       string
		hook       model.Hook
		wantPostOK *bool
		wantCmdOK  *bool
		wantDetail string
	}{
		{name: "a url that answers", hook: model.Hook{URL: ok.URL},
			wantPostOK: new(true), wantDetail: "200"},
		{name: "a url that refuses", hook: model.Hook{URL: refuses.URL},
			wantPostOK: new(false), wantDetail: "500"},
		{name: "a url with nothing listening", hook: model.Hook{URL: goneURL},
			wantPostOK: new(false)},
		{name: "a command that works", hook: model.Hook{Command: "echo fine"},
			wantCmdOK: new(true), wantDetail: "fine"},
		{name: "a command that fails", hook: model.Hook{Command: "echo broken >&2; exit 3"},
			wantCmdOK: new(false), wantDetail: "exit status 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.hook.Command != "" && runtime.GOOS == "windows" {
				t.Skip("the command hook is run through cmd on windows")
			}
			ts, srv := newTestServer(t)
			var added model.Hook
			postJSON(t, ts.URL+"/_/api/groups/api/hooks", tc.hook, &added)

			srv.runHook(&added, ReviewEvent{Group: "api"})

			// Read it back the way a client does, not out of the store:
			// the outcome is only useful if it reaches the listing.
			var hooks []*model.Hook
			getJSON(t, ts.URL+"/_/api/groups/api/hooks", &hooks)
			if len(hooks) != 1 {
				t.Fatalf("got %d hooks, want 1", len(hooks))
			}
			got := hooks[0]

			check := func(what string, run *model.HookRun, want *bool) {
				if want == nil {
					if run != nil {
						t.Errorf("%s was recorded for a hook that has no %s: %+v", what, what, run)
					}
					return
				}
				if run == nil {
					t.Fatalf("no %s outcome was recorded", what)
				}
				if run.OK != *want {
					t.Errorf("%s ok = %v, want %v (detail %q)", what, run.OK, *want, run.Detail)
				}
				if run.At.IsZero() {
					t.Errorf("%s outcome has no time", what)
				}
				if tc.wantDetail != "" && !strings.Contains(run.Detail, tc.wantDetail) {
					t.Errorf("%s detail = %q, want it to mention %q", what, run.Detail, tc.wantDetail)
				}
			}
			check("lastPost", got.LastPost, tc.wantPostOK)
			check("lastCommandRun", got.LastCommandRun, tc.wantCmdOK)
		})
	}
}

// The outcome is written to the session file every round, so a command that
// fails with a wall of output must not grow it by a wall of output a round.
func TestHookRunDetailIsOneShortLine(t *testing.T) {
	run := hookRun(false, "exit status 1: "+strings.Repeat("noise ", 500)+"\nand more\n")
	if len(run.Detail) > maxHookDetail+len(" ...") {
		t.Errorf("detail is %d bytes, want at most %d", len(run.Detail), maxHookDetail+len(" ..."))
	}
	if strings.ContainsAny(run.Detail, "\r\n") {
		t.Errorf("detail spans lines: %q", run.Detail)
	}
	if !strings.HasPrefix(run.Detail, "exit status 1") {
		t.Errorf("detail = %q, want it to start with the reason", run.Detail)
	}
}
