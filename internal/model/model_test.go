package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
			// The inner fence is part of the replacement text, not
			// the end of the block.
			"suggestion proposing a code block",
			"```suggestion\n```go\nx\n```\n```",
			[]string{"```go\nx\n```"},
		},
		{
			"nested fence indented inside a list item",
			"```suggestion\n- run it:\n\n  ```sh\n  go test ./...\n  ```\n```",
			[]string{"- run it:\n\n  ```sh\n  go test ./...\n  ```"},
		},
		{
			"nested tilde block",
			"```suggestion\n~~~\ny\n~~~\n```",
			[]string{"~~~\ny\n~~~"},
		},
		{
			// A close never needs to be longer than the fence it
			// closes, so the longer run is the suggestion's.
			"a run longer than the nested fence closes the suggestion",
			"````suggestion\n```\n````",
			[]string{"```"},
		},
		{
			"an unclosed nested fence still ends at the suggestion fence",
			"```suggestion\n~~~\n```",
			[]string{"~~~"},
		},
		{
			"two blocks each holding a fence",
			"```suggestion\n```go\na\n```\n```\n\nand\n\n```suggestion\n```go\nb\n```\n```",
			[]string{"```go\na\n```", "```go\nb\n```"},
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
			// Both halves of this body are ones a single fix gets
			// wrong. Skipping the quoted block without tracking the
			// fence nested in the real suggestion cuts the real one
			// in half at its own ```go line; tracking the nested
			// fence without skipping the quoted block returns the
			// quoted "nope" as a proposed change. Only doing both
			// leaves one whole suggestion.
			"a quoted suggestion beside a real one holding a code block",
			"````markdown\n```suggestion\nnope\n```\n````\n\n```suggestion\n```go\nx\n```\n```",
			[]string{"```go\nx\n```"},
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

// Whatever WithSuggestion writes, Suggestions has to read back unchanged:
// the two are the only ends of the wire a suggestion travels over, and the
// block written at one end must neither be cut short nor look quoted at the
// other.
func TestWithSuggestionRoundTrip(t *testing.T) {
	bodies := []struct {
		name string
		body string
		// have is what the body already proposes, which the appended
		// suggestion joins rather than replaces.
		have []string
	}{
		{name: "plain body", body: "note"},
		{name: "empty body", body: ""},
		{name: "body quoting a suggestion", body: "like this:\n\n````markdown\n```suggestion\nquoted\n```\n````"},
		// A body whose own fenced block is never closed would otherwise
		// hold the appended block, and the suggestion `sbnn comment
		// --suggest` was handed would be read back as quoted text.
		{name: "body ending inside a code block", body: "```go\nfoo()"},
		{name: "body ending inside a tilde block", body: "~~~\nfoo()"},
		{name: "body ending inside a long fence", body: "````\n```go\nfoo()"},
		{name: "body ending on a bare opening fence", body: "```"},
		// A body that already proposes something is the common case for
		// a second `sbnn comment --suggest` on the same comment. The
		// closing fence of a code block nested in the existing
		// suggestion must not be read as leaving the body open: the
		// close that would be added to "repair" it opens a block of its
		// own, and the appended suggestion falls inside it.
		{
			name: "body holding a suggestion",
			body: "```suggestion\nfirst\n```",
			have: []string{"first"},
		},
		{
			name: "body proposing a code block",
			body: "```suggestion\n```go\nx\n```\n```",
			have: []string{"```go\nx\n```"},
		},
		{
			name: "body proposing a code block with tildes",
			body: "~~~suggestion\n~~~go\nx\n~~~\n~~~",
			have: []string{"~~~go\nx\n~~~"},
		},
		{
			name: "body proposing a code block after quoting one",
			body: "````markdown\n```suggestion\nquoted\n```\n````\n\n```suggestion\n```go\nx\n```\n```",
			have: []string{"```go\nx\n```"},
		},
	}
	suggestions := []string{
		"func parse() {",
		"a\n\nb",
		"```go\nx\n```",
		"```",
		"~~~\ny\n~~~",
		"text with ``` in it",
		"# Heading\n\n```suggestion\nnested\n```",
		"````\ndeep\n````",
	}
	for _, b := range bodies {
		for _, suggestion := range suggestions {
			t.Run(b.name+"/"+suggestion, func(t *testing.T) {
				want := append(append([]string{}, b.have...), suggestion)
				body := model.WithSuggestion(b.body, suggestion)
				if got := model.Suggestions(body); !reflect.DeepEqual(got, want) {
					t.Errorf("round trip through %q gave %q, want %q", body, got, want)
				}
			})
		}
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

		// Padding is whatever the program the verdict was pasted out of
		// used, not the four spaces ASCII has. U+3000 in particular is what
		// a Japanese keyboard produces, so it turns up in front of anything
		// typed there.
		{"approved\v", model.VerdictApproved, true},
		{"approved\f", model.VerdictApproved, true},
		{"approved\u00a0", model.VerdictApproved, true}, // no-break space
		{"\u3000approved", model.VerdictApproved, true}, // ideographic space
		{"approved\u2007", model.VerdictApproved, true}, // figure space
		{"\u3000changes\u3000requested\u3000", model.VerdictChangesRequested, true},
		{"\u3000", model.VerdictCommented, true}, // padding only is still empty

		// Separators are dropped before matching, so these fold down to the
		// empty string - but they are typos, not an omitted verdict. Reading
		// one as "commented" would confirm the review, write it to the
		// history and fire the hook.
		{"-", "", false},
		{"_", "", false},
		{"...", "", false},
		{"-_-", "", false},
		{". - _", "", false},
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

// The browser reads suggestions out of a comment body too, in
// web/src/suggestion.ts, and has to reach the same answer: a suggestion only
// one side sees is offered in the page and refused by the server, or the
// other way round. testdata/suggestions.json is the corpus both sides run.
func TestSuggestionsCorpus(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "suggestions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Name string   `json:"name"`
			Body string   `json:"body"`
			Want []string `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("the corpus is empty")
	}
	for _, tt := range corpus.Cases {
		t.Run(tt.Name, func(t *testing.T) {
			got := model.Suggestions(tt.Body)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tt.Want) {
				t.Errorf("Suggestions(%q) = %q, want %q", tt.Body, got, tt.Want)
			}
		})
	}
}

// TestBlocks is the one table both sides of the question are held to: the
// status sbnn exits with, and the SBNN_BLOCKING a review hook is handed.
//
// The verdict alone does not settle it. Verdict.Blocking() says a review
// that merely commented never blocks, but sbnn has ended on an open comment
// since before there were verdicts at all, and still does.
func TestBlocks(t *testing.T) {
	open := []*model.Comment{{Body: "a remark"}}
	settled := []*model.Comment{{Body: "a remark", Resolved: true}}
	mixed := []*model.Comment{{Body: "done", Resolved: true}, {Body: "not done"}}

	for _, tt := range []struct {
		name     string
		verdict  model.Verdict
		comments []*model.Comment
		want     bool
	}{
		{"approved, nothing said", model.VerdictApproved, nil, false},
		{"an approval with a remark on it is still an approval", model.VerdictApproved, open, false},
		{"an approval outranks even a pile of open comments", model.VerdictApproved, mixed, false},
		{"changes requested", model.VerdictChangesRequested, nil, true},
		{"changes requested blocks with nothing left open", model.VerdictChangesRequested, settled, true},
		{"commented, with a comment still open", model.VerdictCommented, open, true},
		{"commented, everything resolved", model.VerdictCommented, settled, false},
		{"commented, nothing said at all", model.VerdictCommented, nil, false},
		{"commented, one of two left open", model.VerdictCommented, mixed, true},
		{"no verdict, with a comment still open", "", open, true},
		{"no verdict, everything resolved", "", settled, false},
		{"no verdict, nothing said at all", "", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.Blocks(tt.verdict, tt.comments); got != tt.want {
				t.Errorf("Blocks(%q, %d comment(s)) = %v, want %v",
					tt.verdict, len(tt.comments), got, tt.want)
			}
		})
	}
}

// Verdict.Blocking() answers about the verdict and nothing else. Blocks is
// the wider rule; where they differ, Blocks is the one sbnn acts on.
func TestBlockingIsOnlyHalfOfBlocks(t *testing.T) {
	open := []*model.Comment{{Body: "a remark"}}
	for _, v := range []model.Verdict{model.VerdictApproved, model.VerdictChangesRequested, model.VerdictCommented, ""} {
		if v.Blocking() && !model.Blocks(v, open) {
			t.Errorf("%q blocks on its own but Blocks says it does not", v)
		}
	}
	if model.VerdictCommented.Blocking() {
		t.Error("Verdict.Blocking() has changed meaning; Blocks is the rule to change")
	}
	if !model.Blocks(model.VerdictCommented, open) {
		t.Error("a review that commented and left a comment open must block")
	}
}
