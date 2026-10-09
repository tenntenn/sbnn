package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

func TestReplaceWithEmptyDiffSaysSo(t *testing.T) {
	cases := []struct {
		name    string
		replace bool
		content string
		wantIDs []string
		wantMsg bool
	}{
		{name: "empty diff is reported and nothing is listed", replace: true, wantMsg: true},
		{name: "empty diff without --replace is silent"},
		{name: "a diff is replaced quietly", replace: true, content: "diff", wantIDs: []string{"d1"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				if r.Method == http.MethodGet {
					json.NewEncoder(w).Encode(model.Group{Name: "api", Diffs: []*model.Diff{{ID: "d1"}}})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			c := client.New(strings.TrimPrefix(srv.URL, "http://"), 0)
			st := &server.Status{Groups: []server.GroupSummary{{Name: "api", Diffs: 1}}}

			var stderr strings.Builder
			ids, err := previousDiffs(context.Background(), c, st, "api", tt.replace, tt.content, &stderr)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tt.wantIDs)
			}
			if got := strings.Contains(stderr.String(), "the diff is empty, so nothing was replaced"); got != tt.wantMsg {
				t.Errorf("message printed = %v (%q), want %v", got, stderr.String(), tt.wantMsg)
			}
			if tt.content == "" && len(requests) != 0 {
				t.Errorf("requests = %v, want none", requests)
			}
			for _, r := range requests {
				if strings.HasPrefix(r, "DELETE") {
					t.Errorf("unexpected delete request %q", r)
				}
			}
		})
	}
}
