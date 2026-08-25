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

func TestParseVerdict(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want model.Verdict
		ok   bool
	}{
		{"", model.VerdictCommented, true},
		{"  ", model.VerdictCommented, true},
		{"approved", model.VerdictApproved, true},
		{"approve", model.VerdictApproved, true},
		{"LGTM", model.VerdictApproved, true},
		{"accept", model.VerdictApproved, true},
		{"ship it", model.VerdictApproved, true},
		{"commented", model.VerdictCommented, true},
		{"comment", model.VerdictCommented, true},
		{"changes-requested", model.VerdictChangesRequested, true},
		{"changes_requested", model.VerdictChangesRequested, true},
		{"request-changes", model.VerdictChangesRequested, true},
		{"changes", model.VerdictChangesRequested, true},
		// The spelling GitHub's own review API uses.
		{"request_changes", model.VerdictChangesRequested, true},
		{"REQUEST_CHANGES", model.VerdictChangesRequested, true},
		{"requestchanges", model.VerdictChangesRequested, true},
		{"reject", model.VerdictChangesRequested, true},
		{"nope", "", false},
		{"changes requested?", "", false},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := model.ParseVerdict(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("ParseVerdict(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// What String writes has to read back, or a verdict cannot survive being
// printed and typed again.
func TestParseVerdictRoundTripsString(t *testing.T) {
	for _, v := range []model.Verdict{model.VerdictApproved, model.VerdictCommented, model.VerdictChangesRequested} {
		got, ok := model.ParseVerdict(v.String())
		if !ok || got != v {
			t.Errorf("ParseVerdict(%q.String() = %q) = %q, %v", string(v), v.String(), got, ok)
		}
	}
}
