package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// TestReplyComment pins what `sbnn comment --reply-to` sends. A place given
// beside it goes along for the server to check against the thread; --side
// only when it was typed, since its default would contradict a thread on the
// old side.
func TestReplyComment(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		flags   []string
		want    server.AddCommentRequest
		wantErr string
	}{
		{
			name:  "the id alone",
			flags: []string{"--reply-to", "c3", "-m", "yes", "--author", "claude"},
			want:  server.AddCommentRequest{ReplyTo: "c3", Author: "claude", Body: "yes"},
		},
		{
			name:  "with the thread's place",
			args:  []string{"main.go:12-14"},
			flags: []string{"--reply-to", "c3", "-m", "yes"},
			want:  server.AddCommentRequest{ReplyTo: "c3", Author: DefaultAuthor, Body: "yes", Path: "main.go", StartLine: 12, EndLine: 14},
		},
		{
			name:  "a side that was typed",
			flags: []string{"--reply-to", "c3", "-m", "yes", "--side", "OLD"},
			want:  server.AddCommentRequest{ReplyTo: "c3", Author: DefaultAuthor, Body: "yes", Side: "old"},
		},
		{
			name:  "a suggestion",
			flags: []string{"--reply-to", "c3", "--suggest", "x := 2"},
			want:  server.AddCommentRequest{ReplyTo: "c3", Author: DefaultAuthor, Suggestion: "x := 2"},
		},
		{
			name:    "nothing to say",
			flags:   []string{"--reply-to", "c3"},
			wantErr: "say something",
		},
		{
			name:    "an unreadable place",
			args:    []string{"main.go:0"},
			flags:   []string{"--reply-to", "c3", "-m", "yes"},
			wantErr: "not a line",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetCommentFlags(t)
			t.Cleanup(func() { resetCommentFlags(t) })
			if err := commentCmd.ParseFlags(tt.flags); err != nil {
				t.Fatal(err)
			}
			got, err := replyComment(tt.args, commentCmd.Flags().Changed("side"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("replyComment() = %v, want an error mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("replyComment() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A reply takes no --question, and --json names what each entry answers
// itself; both pairs are refused before anything is sent.
func TestReplyToFlagGroups(t *testing.T) {
	for _, args := range [][]string{
		{"--reply-to", "c1", "--question", "-m", "x"},
		{"--reply-to", "c1", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			resetCommentFlags(t)
			t.Cleanup(func() { resetCommentFlags(t) })
			if err := commentCmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			if err := commentCmd.ValidateFlagGroups(); err == nil {
				t.Errorf("comment %s was accepted", strings.Join(args, " "))
			}
		})
	}
}

func TestReadBulkCommentsWithReplies(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entry   string
		want    server.AddCommentRequest
		wantErr string
	}{
		{
			name:  "a reply needs no path",
			entry: `{"replyTo":"c1","body":"yes","author":"claude"}`,
			want:  server.AddCommentRequest{ReplyTo: "c1", Body: "yes", Author: "claude"},
		},
		{
			name:  "a place is passed along",
			entry: `{"replyTo":"c1","path":"a.go","line":"3-4","body":"yes"}`,
			want:  server.AddCommentRequest{ReplyTo: "c1", Body: "yes", Author: DefaultAuthor, Path: "a.go", StartLine: 3, EndLine: 4},
		},
		{
			name:    "a reply is not a question",
			entry:   `{"replyTo":"c1","body":"really?","question":true}`,
			wantErr: "question",
		},
		{
			name:    "a reply says something",
			entry:   `{"replyTo":"c1"}`,
			wantErr: "says nothing",
		},
		{
			name:    "neither a path nor a reply",
			entry:   `{"body":"x"}`,
			wantErr: "no path",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetCommentFlags(t)
			t.Cleanup(func() { resetCommentFlags(t) })
			got, err := readBulkComments(strings.NewReader("[" + tt.entry + "]"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readBulkComments(%s) = %v, want an error mentioning %q", tt.entry, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("readBulkComments(%s) = %+v, want %+v", tt.entry, got, tt.want)
			}
		})
	}
}

// replyAdder answers every request with a reply to c1, the way the server
// does once it has put the reply on its thread's lines.
type replyAdder struct{}

func (replyAdder) AddComment(_ context.Context, _ string, req server.AddCommentRequest) (*model.Comment, error) {
	return &model.Comment{ID: "c9", ReplyTo: "c1", Path: "a.go", StartLine: 3, EndLine: 4}, nil
}

func TestAddCommentsSaysWhatAReplyAnswers(t *testing.T) {
	withJSONOut(t, false)
	var out, errOut bytes.Buffer
	if err := addComments(context.Background(), replyAdder{}, "default",
		[]server.AddCommentRequest{{ReplyTo: "c2", Body: "x"}}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if got, want := errOut.String(), "sbnn: c9 replies to c1 on a.go:3-4\n"; got != want {
		t.Errorf("addComments printed %q, want %q", got, want)
	}
}
