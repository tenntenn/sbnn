package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A record that parses but cannot be replayed is as broken as one that does
// not parse: the session file is kept aside and the server starts a new one,
// instead of running on half a session and appending behind the bad record.
func TestAnUnreplayableRecordIsBroken(t *testing.T) {
	cases := []struct {
		name   string
		record string
	}{
		{"an unknown operation", `{"op":"teleport"}`},
		{"a comment for a group that is not there", `{"op":"comment","group":"ghost","comment":{"id":"c9"}}`},
		{"a diff that is not there", `{"op":"diff","group":"default"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			s := NewStore(path)
			mutate(t, s, "add diff")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			damaged := string(b) + tc.record + "\n" + `{"op":"delAll"}` + "\n"
			if err := os.WriteFile(path, []byte(damaged), 0o600); err != nil {
				t.Fatal(err)
			}

			restarted := NewStore(path)
			if err := restarted.Load(); err == nil {
				t.Fatal("Load() = nil, want an error")
			}
			if names := restarted.GroupNames(); len(names) != 0 {
				t.Errorf("groups after the failed load = %v, want none", names)
			}
			if kept, err := os.ReadFile(path + ".broken"); err != nil || string(kept) != damaged {
				t.Errorf("the damaged file was not kept: %v", err)
			}
		})
	}
}

// The log of a group the snapshot would not have accepted is replayed with the
// rest and the group is dropped afterwards, and the file stops carrying it at
// the next mutation.
func TestRecordsOfAnInvalidGroupAreReplayedThenDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	snapshot := `{"version":2,"seq":1,"seqs":{"d":1},"groups":[{"name":"default","diffs":[{"id":"d1"}],"comments":[]},` +
		`{"name":"a/evil","diffs":[{"id":"d2"}],"comments":[]}],"rounds":{"default":1}}` + "\n"
	record := `{"op":"comment","group":"a/evil","comment":{"id":"c1","group":"a/evil","diffId":"d2"}}` + "\n"
	if err := os.WriteFile(path, []byte(snapshot+record), 0o600); err != nil {
		t.Fatal(err)
	}
	s := reloaded(t, path)
	if names := s.GroupNames(); len(names) != 1 || names[0] != DefaultGroup {
		t.Fatalf("groups = %v, want only %q", names, DefaultGroup)
	}
	mutate(t, s, "add diff")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "evil") {
		t.Errorf("the session file still holds the dropped group:\n%s", b)
	}
}

// A log in a format this build cannot read is refused as a newer sbnn's file,
// not called broken and moved aside.
func TestANewerLogIsRefusedNotBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{"version":99,"seq":1,"groups":[]}` + "\n" + "<a record in some other encoding>\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path)
	err := s.Load()
	if err == nil || !strings.Contains(err.Error(), "newer sbnn") {
		t.Fatalf("Load() = %v, want it to say the file is from a newer sbnn", err)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != raw {
		t.Errorf("the file was touched: %v", readErr)
	}
	mutate(t, s, "add diff")
	if got, _ := os.ReadFile(path); string(got) != raw {
		t.Error("a mutation wrote over the file of a newer sbnn")
	}
}
