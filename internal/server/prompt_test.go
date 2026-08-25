package server_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// The verdict is the first thing an agent reads, because it decides what
// the comments mean: the same three remarks block a change or do not,
// depending on what the reviewer said about the change as a whole.
func TestPromptStatesTheVerdict(t *testing.T) {
	group := func(v model.Verdict) *model.Group {
		return &model.Group{
			Name:          "api",
			ReviewedAt:    time.Now(),
			ReviewVerdict: v,
			Comments: []*model.Comment{
				{Path: "main.go", Side: "new", StartLine: 4, EndLine: 4, Body: "a remark"},
			},
		}
	}
	cases := []struct {
		verdict model.Verdict
		want    []string
		absent  []string
	}{
		{model.VerdictApproved,
			[]string{"approved the change", "does not block", "came with the approval"},
			[]string{"to address", "Address every comment"}},
		{model.VerdictChangesRequested,
			[]string{"asked for changes", "should not go ahead", "to address"},
			[]string{"approved"}},
		{model.VerdictCommented,
			[]string{"without deciding either way", "to address"},
			[]string{"approved", "asked for changes"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.verdict), func(t *testing.T) {
			got := server.Prompt(group(tc.verdict), server.PromptOptions{})
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("prompt does not say %q:\n%s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("prompt should not say %q:\n%s", absent, got)
				}
			}
		})
	}
}

// A round nobody has submitted yet says nothing about a verdict: there is
// no reviewer to attribute one to.
func TestPromptSaysNothingBeforeSubmission(t *testing.T) {
	g := &model.Group{Name: "api", Comments: []*model.Comment{
		{Path: "main.go", Side: "new", StartLine: 4, EndLine: 4, Body: "a remark"},
	}}
	if got := server.Prompt(g, server.PromptOptions{}); strings.Contains(got, "The reviewer ") {
		t.Errorf("prompt claims a verdict before one was given:\n%s", got)
	}
}

// A question and a change request are different asks, and the prose does
// not always separate them. The prompt has to, or the agent rewrites code
// that only needed an explanation.
func TestPromptSeparatesQuestionsFromChanges(t *testing.T) {
	g := &model.Group{
		Name:       "api",
		ReviewedAt: time.Now(),
		Comments: []*model.Comment{
			{Path: "main.go", Side: "new", StartLine: 4, EndLine: 4,
				Body: "Should this be a 404?", Question: true},
			{Path: "main.go", Side: "new", StartLine: 9, EndLine: 9,
				Body: "rename this"},
		},
	}
	got := server.Prompt(g, server.PromptOptions{})
	for _, want := range []string{
		"2 comment(s) to address, 1 of them a question",
		"This one is a question: answer it.",
		"asking for an answer, not for a change",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// Nothing about questions appears when none was asked.
func TestPromptSaysNothingAboutQuestionsWhenThereAreNone(t *testing.T) {
	g := &model.Group{Name: "api", ReviewedAt: time.Now(), Comments: []*model.Comment{
		{Path: "main.go", Side: "new", StartLine: 4, EndLine: 4, Body: "rename this"},
	}}
	if got := server.Prompt(g, server.PromptOptions{}); strings.Contains(got, "question") {
		t.Errorf("prompt talks about questions when none was asked:\n%s", got)
	}
}

// promptFixture is one case of the golden corpus in testdata/prompt. It is
// deliberately plain JSON: the point of the corpus is that a renderer which
// is not written in Go can read the very same files.
type promptFixture struct {
	// Doc says what the case pins, for whoever meets it from the other side.
	Doc     string               `json:"doc"`
	Options server.PromptOptions `json:"options"`
	Group   *model.Group         `json:"group"`
}

var update = flag.Bool("update", false, "rewrite the .golden files in testdata/prompt")

// TestPromptGolden pins the exact text Prompt produces.
//
// The prompt is rendered twice in this repository: here, and again in the
// browser by an exported page, which has to rebuild it because the reader
// can add comments to a page with no server to ask. The two texts are
// claimed to be identical and drifted apart anyway, so the corpus writes the
// contract down: input group in, prompt out, both in formats a renderer in
// another language can read without linking against Go.
func TestPromptGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "prompt", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("the prompt corpus is empty")
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fx promptFixture
			dec := json.NewDecoder(bytes.NewReader(in))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&fx); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			if fx.Doc == "" {
				t.Errorf("%s has no doc saying what it pins", path)
			}
			if fx.Group == nil {
				t.Fatalf("%s has no group", path)
			}

			got := server.Prompt(fx.Group, fx.Options)
			golden := filepath.Join("testdata", "prompt", name+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/server -run TestPromptGolden -update)", err)
			}
			if got != string(want) {
				t.Errorf("prompt for %s does not match the golden file.\n--- got ---\n%s\n--- want ---\n%s",
					name, got, want)
			}
		})
	}
}

// TestPromptGoldenCoversEveryVerdict keeps the corpus from losing the cases
// the two renderers actually disagreed about: each verdict, a note, and a
// round that was never submitted.
func TestPromptGoldenCoversEveryVerdict(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "prompt", "*.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus string
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		corpus += string(b)
	}
	for _, want := range []string{
		"The reviewer approved the change",
		"The reviewer asked for changes",
		"left comments without deciding either way",
		"The reviewer wrote:",
		"came with the approval",
		"to address",
		"The change is approved, so none of this blocks it",
		"Address every comment above",
	} {
		if !strings.Contains(corpus, want) {
			t.Errorf("no golden file contains %q, so nothing pins it", want)
		}
	}
}
