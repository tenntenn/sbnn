package model

import (
	"slices"
	"strings"
	"testing"
)

// shape writes threads as "root[reply reply] root[]", which is easier to
// compare than the structs.
func shape(threads []Thread) string {
	var parts []string
	for _, t := range threads {
		ids := make([]string, 0, len(t.Replies))
		for _, r := range t.Replies {
			ids = append(ids, r.ID)
		}
		parts = append(parts, t.Root.ID+"["+strings.Join(ids, " ")+"]")
	}
	return strings.Join(parts, " ")
}

func TestThreads(t *testing.T) {
	for _, tt := range []struct {
		name     string
		comments []*Comment
		want     string
		wantOpen int
	}{
		{name: "none", want: "", wantOpen: 0},
		{
			name:     "comments without replies are a thread each",
			comments: []*Comment{{ID: "c1"}, {ID: "c2", Resolved: true}},
			want:     "c1[] c2[]",
			wantOpen: 1,
		},
		{
			name: "replies follow their comment, whenever they were written",
			comments: []*Comment{
				{ID: "c1"}, {ID: "c2"}, {ID: "c3", ReplyTo: "c1"}, {ID: "c4", ReplyTo: "c2"}, {ID: "c5", ReplyTo: "c1"},
			},
			want:     "c1[c3 c5] c2[c4]",
			wantOpen: 2,
		},
		{
			name:     "a reply whose comment is gone stands on its own",
			comments: []*Comment{{ID: "c1"}, {ID: "c3", ReplyTo: "c2"}},
			want:     "c1[] c3[]",
			wantOpen: 2,
		},
		{
			name:     "a resolved thread is one, however many replies it has",
			comments: []*Comment{{ID: "c1", Resolved: true}, {ID: "c2", ReplyTo: "c1", Resolved: true}, {ID: "c3"}},
			want:     "c1[c2] c3[]",
			wantOpen: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := shape(Threads(tt.comments)); got != tt.want {
				t.Errorf("Threads() = %q, want %q", got, tt.want)
			}
			if got := OpenThreads(tt.comments); got != tt.wantOpen {
				t.Errorf("OpenThreads() = %d, want %d", got, tt.wantOpen)
			}
		})
	}
}

// Old session files and clients know nothing of replyTo: a comment without
// one marshals exactly as it did, and one with it carries the key.
func TestReplyToIsOmittedWhenEmpty(t *testing.T) {
	b, err := (&Comment{ID: "c1"}).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "replyTo") {
		t.Errorf("a comment that opens a thread marshals %s", b)
	}
	b, err = (&Comment{ID: "c2", ReplyTo: "c1"}).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"replyTo":"c1"`) {
		t.Errorf("a reply marshals %s, without what it answers", b)
	}
	if !slices.Equal([]bool{(&Comment{ReplyTo: "c1"}).IsReply(), (&Comment{}).IsReply()}, []bool{true, false}) {
		t.Error("IsReply does not tell a reply from a comment")
	}
}
