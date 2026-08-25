package server

import (
	"fmt"
	"strings"

	"github.com/tenntenn/sbnn/internal/model"
)

// PromptOptions tunes the prompt rendered from review comments.
//
// The fields are tagged so that the golden corpus in testdata/prompt can
// state them in the same JSON any other renderer of this text reads.
type PromptOptions struct {
	// IncludeResolved keeps comments that were marked as resolved.
	IncludeResolved bool `json:"includeResolved,omitempty"`
	// NoInstruction drops the closing instruction, leaving only the
	// comments themselves.
	NoInstruction bool `json:"noInstruction,omitempty"`
}

// Prompt renders the review comments of a group as Markdown meant to be
// handed to a coding agent.
//
// This text has a second renderer: an exported page rebuilds it in the
// browser, because the reader can add comments to a page that has no server
// to ask. The two have to agree character for character - an agent handed
// the copied prompt must not be told to address comments that came with an
// approval - and prose is easy to let drift. So the exact output is pinned
// by the golden corpus in testdata/prompt: each case is an input group and
// the text it must produce, in plain JSON and plain text, so that a renderer
// written in another language checks itself against the same files.
// Changing the wording here means regenerating the corpus (go test -update)
// and bringing the other renderer along.
func Prompt(g *model.Group, opts PromptOptions) string {
	var b strings.Builder
	comments := make([]*model.Comment, 0, len(g.Comments))
	for _, c := range g.Comments {
		if c.Resolved && !opts.IncludeResolved {
			continue
		}
		comments = append(comments, c)
	}

	fmt.Fprintf(&b, "# Review comments (sbnn group %q)\n\n", g.Name)
	if g.ReviewedAt.IsZero() {
		// Nothing was submitted; these are comments in progress.
	} else {
		fmt.Fprintf(&b, "The reviewer %s.\n\n", verdictSentence(g.ReviewVerdict))
	}
	if note := strings.TrimSpace(g.ReviewNote); note != "" {
		fmt.Fprintf(&b, "The reviewer wrote:\n\n%s\n\n", note)
	}
	if len(comments) == 0 {
		b.WriteString("No open review comments.\n")
		return b.String()
	}
	questions := 0
	for _, c := range comments {
		if c.Question {
			questions++
		}
	}
	if g.ReviewVerdict == model.VerdictApproved {
		fmt.Fprintf(&b, "%d comment(s) came with the approval%s.\n", len(comments), asking(questions))
	} else {
		fmt.Fprintf(&b, "%d comment(s) to address%s.\n", len(comments), asking(questions))
	}

	titles := map[string]string{}
	for _, d := range g.Diffs {
		titles[d.ID] = d.Title
	}

	for i, c := range comments {
		fmt.Fprintf(&b, "\n## %d. %s%s\n", i+1, c.Path, lineRange(c))
		if title := titles[c.DiffID]; title != "" {
			fmt.Fprintf(&b, "\nDiff: %s\n", title)
		}
		if c.Author != "" {
			fmt.Fprintf(&b, "\nFrom: %s\n", c.Author)
		}
		if c.Question {
			b.WriteString("\nThis one is a question: answer it.\n")
		}
		if c.Resolved {
			b.WriteString("\nStatus: resolved\n")
		}
		if snippet := strings.TrimRight(c.Snippet, "\n"); snippet != "" {
			fence := fenceFor(snippet)
			fmt.Fprintf(&b, "\n%s\n%s\n%s\n", fence, snippet, fence)
		}
		// The body is Markdown and may carry suggestion blocks, so it goes
		// out as it is rather than quoted line by line.
		if body := strings.TrimRight(c.Body, "\n"); body != "" {
			fmt.Fprintf(&b, "\n%s\n", body)
		}
		if n := len(model.Suggestions(c.Body)); n > 0 {
			fmt.Fprintf(&b, "\nThe suggestion block above replaces %s.\n", lineRangeText(c))
			if n > 1 {
				fmt.Fprintf(&b, "(%d suggestion blocks: apply them in order.)\n", n)
			}
		}
	}

	if !opts.NoInstruction {
		b.WriteString("\n---\n\n")
		if g.ReviewVerdict == model.VerdictApproved {
			b.WriteString("The change is approved, so none of this blocks it. Act on what is " +
				"worth acting on, say what you are leaving and why, and carry on.\n")
		} else {
			b.WriteString("Address every comment above. A suggestion block replaces the lines it " +
				"names, verbatim. When a comment is not worth acting on, say why instead of " +
				"changing the code.\n")
		}
		if questions > 0 {
			b.WriteString("\nA comment marked as a question is asking for an answer, not for a " +
				"change. Answer it in your reply, in words, and change the code only if your " +
				"own answer says it should change. Leaving a question unanswered is the one " +
				"thing that makes the reviewer ask it again.\n")
		}
	}
	return b.String()
}

// lineRangeText names the lines a suggestion replaces, e.g. "path:12-18".
func lineRangeText(c *model.Comment) string {
	return c.Path + lineRange(c)
}

// lineRange formats the reviewed line range, e.g. ":12-18 (old)".
func lineRange(c *model.Comment) string {
	if c.StartLine <= 0 {
		return ""
	}
	side := ""
	if c.Side == "old" {
		side = " (old)"
	}
	if c.EndLine > c.StartLine {
		return fmt.Sprintf(":%d-%d%s", c.StartLine, c.EndLine, side)
	}
	return fmt.Sprintf(":%d%s", c.StartLine, side)
}

// fenceFor returns a code fence long enough to wrap content that may itself
// contain backticks.
func fenceFor(content string) string {
	longest, current := 0, 0
	for _, r := range content {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

// verdictSentence says what the reviewer decided, in words an agent can act
// on without counting anything.
func verdictSentence(v model.Verdict) string {
	switch v {
	case model.VerdictApproved:
		return "approved the change; anything below is worth reading but does not block it"
	case model.VerdictChangesRequested:
		return "asked for changes; the change should not go ahead as it is"
	default:
		return "left comments without deciding either way"
	}
}

// asking counts the comments that want an answer rather than a change, for
// the line that says how much there is to do.
func asking(questions int) string {
	if questions == 0 {
		return ""
	}
	return fmt.Sprintf(", %d of them a question", questions)
}
