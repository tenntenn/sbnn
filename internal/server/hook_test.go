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
