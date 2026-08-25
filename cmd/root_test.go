package cmd

import (
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/server"
)

func TestConfirm(t *testing.T) {
	const question = "Close all 2 review(s)? [y/N]: "

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes short", "y\n", true},
		{"yes short upper", "Y\n", true},
		{"yes long", "yes\n", true},
		{"yes long upper", "YES\n", true},
		{"yes padded", " y \n", true},
		{"yes without newline", "y", true},
		{"no", "n\n", false},
		{"blank line", "\n", false},
		{"eof", "", false},
		{"anything else", "yolo\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			got, err := confirm(strings.NewReader(tt.input), &out, question)
			if err != nil {
				t.Fatalf("confirm(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("confirm(%q) = %v; want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfirmAsks(t *testing.T) {
	const question = "Close all 2 review(s)? [y/N]: "

	var out strings.Builder
	if _, err := confirm(strings.NewReader("n\n"), &out, question); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if out.String() != question {
		t.Errorf("confirm wrote %q; want the question %q", out.String(), question)
	}
}

func TestClearAllQuestion(t *testing.T) {
	tests := []struct {
		name     string
		groups   []server.GroupSummary
		want     string
		contains []string
	}{
		{
			name:   "nothing to lose",
			groups: nil,
			want:   "",
		},
		{
			name: "one review, nothing open",
			groups: []server.GroupSummary{
				{Name: "default", Diffs: 1, Comments: 0, Unresolved: 0},
			},
			contains: []string{"default", "1 diff(s)", "0 comment(s)", "0 open", "Close all 1 review(s)? [y/N]: "},
		},
		{
			name: "several reviews, comments still open",
			groups: []server.GroupSummary{
				{Name: "default", Diffs: 3, Comments: 2, Unresolved: 1},
				{Name: "api", Diffs: 1, Comments: 0, Unresolved: 0},
			},
			contains: []string{
				"this will close every review on the server",
				"default", "3 diff(s)", "2 comment(s)", "1 open",
				"api", "1 diff(s)",
				"Close all 2 review(s)? [y/N]: ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clearAllQuestion(tt.groups)
			if tt.contains == nil {
				if got != tt.want {
					t.Fatalf("clearAllQuestion = %q; want %q", got, tt.want)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("clearAllQuestion = %q; want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestClearGroupQuestion(t *testing.T) {
	groups := []server.GroupSummary{
		{Name: "default", Diffs: 3, Comments: 2, Unresolved: 0},
		{Name: "api", Diffs: 1, Comments: 4, Unresolved: 1},
	}

	tests := []struct {
		name     string
		group    string
		contains []string
	}{
		{name: "nothing open, no question", group: "default"},
		{name: "unknown group is already empty", group: "gone"},
		{
			name:     "open comments are named",
			group:    "api",
			contains: []string{`Close the review of "api"?`, "1 comment(s) are still open", "[y/N]: "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clearGroupQuestion(groups, tt.group)
			if tt.contains == nil {
				if got != "" {
					t.Fatalf("clearGroupQuestion(%q) = %q; want no question", tt.group, got)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("clearGroupQuestion(%q) = %q; want it to contain %q", tt.group, got, want)
				}
			}
		})
	}
}
