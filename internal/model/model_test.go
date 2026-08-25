package model_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tenntenn/sbnn/internal/model"
)

func TestSuggestions(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want []string
	}{
		{"none", "just a comment", nil},
		{
			"one block",
			"please rename it\n\n```suggestion\nfunc parse() {\n```\n",
			[]string{"func parse() {"},
		},
		{
			"two blocks",
			"a\n\n```suggestion\nfirst\n```\n\nb\n\n```suggestion\nsecond\nline\n```",
			[]string{"first", "second\nline"},
		},
		{
			"tildes",
			"~~~suggestion\nwith ``` inside\n~~~",
			[]string{"with ``` inside"},
		},
		{
			"longer fence",
			"````suggestion\n```go\ncode\n```\n````",
			[]string{"```go\ncode\n```"},
		},
		{
			"plain code block is not a suggestion",
			"```go\ncode\n```",
			nil,
		},
		{
			// The comment shows the format, it does not use it.
			"quoted suggestion is not a suggestion",
			"Here is how you write one:\n\n`````markdown\n```suggestion\nnot a real one\n```\n`````\n",
			nil,
		},
		{
			"quoted inside a tilde block",
			"~~~markdown\n```suggestion\nnot a real one\n```\n~~~",
			nil,
		},
		{
			"a quoting fence no longer than the one it holds",
			"```markdown\n```suggestion\nx\n```\n```",
			nil,
		},
		{
			"a real suggestion after a quoted one still counts",
			"like this:\n\n````markdown\n```suggestion\nquoted\n```\n````\n\nso:\n\n```suggestion\nreal\n```",
			[]string{"real"},
		},
		{
			"a suggestion before a quoted one still counts",
			"```suggestion\nreal\n```\n\n````markdown\n```suggestion\nquoted\n```\n````",
			[]string{"real"},
		},
		{
			"empty suggestion deletes the lines",
			"drop it\n\n```suggestion\n```",
			[]string{""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.Suggestions(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Suggestions() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithSuggestion(t *testing.T) {
	got := model.WithSuggestion("please rename it", "func parse() {")
	want := "please rename it\n\n```suggestion\nfunc parse() {\n```"
	if got != want {
		t.Errorf("WithSuggestion() = %q, want %q", got, want)
	}
	// The fence grows so that a suggestion may itself contain code blocks.
	got = model.WithSuggestion("", "```go\ncode\n```")
	if suggestions := model.Suggestions(got); len(suggestions) != 1 || suggestions[0] != "```go\ncode\n```" {
		t.Errorf("round trip lost the suggestion: %q -> %q", got, suggestions)
	}
	if model.WithSuggestion("body", "  \n ") != "body" {
		t.Error("an empty suggestion should not add a block")
	}
}

func TestCommentJSONCarriesSuggestions(t *testing.T) {
	c := &model.Comment{Body: "a\n\n```suggestion\nnew\n```"}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Body        string   `json:"body"`
		Suggestions []string `json:"suggestions"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Body != c.Body {
		t.Errorf("body = %q", decoded.Body)
	}
	if !reflect.DeepEqual(decoded.Suggestions, []string{"new"}) {
		t.Errorf("suggestions = %q", decoded.Suggestions)
	}
}

// Whatever WithSuggestion writes, Suggestions has to read back: the two are
// the only ends of the wire a suggestion travels over, and the block written
// at one end must not look quoted at the other.
func TestWithSuggestionRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		suggestion string
	}{
		{"plain", "note", "func parse() {"},
		{"empty body", "", "func parse() {"},
		{"several lines", "note", "a\n\nb"},
		{"body already quoting a block", "like this:\n\n````markdown\n```suggestion\nquoted\n```\n````", "real"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := model.WithSuggestion(tt.body, tt.suggestion)
			if got := model.Suggestions(body); !reflect.DeepEqual(got, []string{tt.suggestion}) {
				t.Errorf("round trip through %q gave %q, want %q", body, got, []string{tt.suggestion})
			}
		})
	}
}
