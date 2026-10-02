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

func TestReplaceDropsPreviousDiffs(t *testing.T) {
	cases := []struct {
		name    string
		groups  []server.GroupSummary
		held    []*model.Diff
		want    []string
		wantDel []string
	}{
		{
			name:   "unknown group holds nothing",
			groups: nil,
		},
		{
			name:   "empty group holds nothing",
			groups: []server.GroupSummary{{Name: "api", Diffs: 0}},
		},
		{
			name:    "every earlier diff is dropped",
			groups:  []server.GroupSummary{{Name: "api", Diffs: 2}},
			held:    []*model.Diff{{ID: "d1"}, {ID: "d2"}},
			want:    []string{"d1", "d2"},
			wantDel: []string{"/_/api/groups/api/diffs/d1", "/_/api/groups/api/diffs/d2"},
		},
		{
			name:   "another group is left alone",
			groups: []server.GroupSummary{{Name: "other", Diffs: 1}},
			held:   []*model.Diff{{ID: "x"}},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var deleted []string
			mux := http.NewServeMux()
			mux.HandleFunc("GET /_/api/groups/api", func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(model.Group{Name: "api", Diffs: tt.held})
			})
			mux.HandleFunc("DELETE /_/api/groups/api/diffs/{diff}", func(w http.ResponseWriter, r *http.Request) {
				deleted = append(deleted, r.URL.Path)
				w.WriteHeader(http.StatusNoContent)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			c := client.New(strings.TrimPrefix(srv.URL, "http://"), 0)

			ids, err := diffIDs(context.Background(), c, &server.Status{Groups: tt.groups}, "api")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids, tt.want) {
				t.Fatalf("diffIDs = %v, want %v", ids, tt.want)
			}
			if err := dropDiffs(context.Background(), c, "api", ids); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(deleted, tt.wantDel) {
				t.Errorf("deleted %v, want %v", deleted, tt.wantDel)
			}
		})
	}
}
