package server

// Tests for deleting hooks: an id that names no hook is a miss, while
// clearing a whole group is a success whatever the count. They sit in their
// own file rather than at the end of server_test.go so that other work on
// internal/server does not have to land in the same place.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

// deleteAndCount performs a DELETE and returns its status and, for a 200,
// the "removed" count from the body.
func deleteAndCount(t *testing.T, url string) (int, int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, 0
	}
	var body map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body["removed"]
}

// DELETE .../hooks/{id} answered 200 {"removed":0} for an id that matched
// nothing, so a typo'd or already-deleted id looked exactly like a
// success. Every other by-id delete in the API answers 404.
func TestHandleDeleteHooksReportsAnUnknownID(t *testing.T) {
	ts, _ := newTestServer(t)
	var hook model.Hook
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "echo one"}, &hook)
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "echo two"}, nil)

	// An id that names no hook is a miss, not a no-op success.
	if status, _ := deleteAndCount(t, ts.URL+"/_/api/groups/default/hooks/nosuchhook"); status != http.StatusNotFound {
		t.Errorf("deleting an unknown id: status = %d, want 404", status)
	}
	// Nothing was removed by that attempt.
	var hooks []*model.Hook
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 2 {
		t.Fatalf("got %d hooks after a failed delete, want 2", len(hooks))
	}

	// A real id still deletes exactly one and says so.
	status, removed := deleteAndCount(t, ts.URL+"/_/api/groups/default/hooks/"+hook.ID)
	if status != http.StatusOK || removed != 1 {
		t.Errorf("deleting a real id: status = %d removed = %d, want 200 and 1", status, removed)
	}
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 1 {
		t.Errorf("got %d hooks after deleting one, want 1", len(hooks))
	}

	// Deleting the same id again is now a miss too.
	if status, _ := deleteAndCount(t, ts.URL+"/_/api/groups/default/hooks/"+hook.ID); status != http.StatusNotFound {
		t.Errorf("deleting the same id twice: status = %d, want 404", status)
	}
}

// The clear-all route keeps its 200 and its count, including when there
// is nothing to clear: removing nothing from an empty list is what was
// asked for, not a miss.
func TestHandleDeleteHooksClearAllStillReportsACount(t *testing.T) {
	ts, _ := newTestServer(t)

	// Clearing an empty group is a success with a count of zero.
	status, removed := deleteAndCount(t, ts.URL+"/_/api/groups/default/hooks")
	if status != http.StatusOK || removed != 0 {
		t.Errorf("clearing an empty group: status = %d removed = %d, want 200 and 0", status, removed)
	}

	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{Command: "echo one"}, nil)
	postJSON(t, ts.URL+"/_/api/groups/default/hooks", model.Hook{URL: "http://example.com/x"}, nil)

	status, removed = deleteAndCount(t, ts.URL+"/_/api/groups/default/hooks")
	if status != http.StatusOK || removed != 2 {
		t.Errorf("clearing two hooks: status = %d removed = %d, want 200 and 2", status, removed)
	}
	var hooks []*model.Hook
	getJSON(t, ts.URL+"/_/api/groups/default/hooks", &hooks)
	if len(hooks) != 0 {
		t.Errorf("got %d hooks after clearing, want 0", len(hooks))
	}
}
