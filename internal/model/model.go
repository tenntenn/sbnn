// Package model defines the data structures shared by the sbnn CLI, the sbnn
// server and the sbnn web UI.
package model

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

// FileStatus represents how a file was changed in a diff.
type FileStatus string

const (
	StatusAdded    FileStatus = "added"
	StatusDeleted  FileStatus = "deleted"
	StatusModified FileStatus = "modified"
	StatusRenamed  FileStatus = "renamed"
	StatusCopied   FileStatus = "copied"
	StatusMode     FileStatus = "mode"
)

// LineKind represents the kind of a line inside a hunk.
type LineKind string

const (
	LineContext LineKind = "context"
	LineAdd     LineKind = "add"
	LineDelete  LineKind = "delete"
)

// ViewMode is the preferred way of rendering a file diff in the web UI.
type ViewMode string

const (
	// ViewUnified renders a single column. New (and deleted) files are always
	// rendered this way because there is no counterpart to put side by side.
	ViewUnified ViewMode = "unified"
	// ViewSplit renders old and new side by side.
	ViewSplit ViewMode = "split"
)

// Line is a single line inside a hunk.
type Line struct {
	Kind LineKind `json:"kind"`
	// OldNumber and NewNumber are 1-based line numbers, or 0 when the line
	// does not exist on that side.
	OldNumber int    `json:"oldNumber"`
	NewNumber int    `json:"newNumber"`
	Content   string `json:"content"`
	// NoNewline reports that "\ No newline at end of file" followed this line.
	NoNewline bool `json:"noNewline,omitempty"`
}

// Hunk is a single @@ section of a file diff.
type Hunk struct {
	Header   string `json:"header"`
	OldStart int    `json:"oldStart"`
	OldLines int    `json:"oldLines"`
	NewStart int    `json:"newStart"`
	NewLines int    `json:"newLines"`
	// Section is the optional function context after the closing @@.
	Section string `json:"section,omitempty"`
	Lines   []Line `json:"lines"`
}

// File is a single file entry of a diff.
type File struct {
	ID        string     `json:"id"`
	OldPath   string     `json:"oldPath"`
	NewPath   string     `json:"newPath"`
	Status    FileStatus `json:"status"`
	IsBinary  bool       `json:"isBinary"`
	OldMode   string     `json:"oldMode,omitempty"`
	NewMode   string     `json:"newMode,omitempty"`
	Index     string     `json:"index,omitempty"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	// ViewMode is the rendering mode the UI should default to.
	ViewMode ViewMode `json:"viewMode"`
	// Folded asks the page to keep this file shut until the reader opens
	// it. Nothing is hidden by it: the file, its counts and its comments
	// stay where they are.
	Folded bool `json:"folded,omitempty"`
	// FoldReason says why, in words the reader can check and disagree
	// with. sbnn never folds a file it cannot give a reason for.
	FoldReason string `json:"foldReason,omitempty"`
	// IsMarkdown reports whether the file can be previewed with mo.
	IsMarkdown bool `json:"isMarkdown"`
	// IsImage reports whether the file can be previewed as an image.
	IsImage bool `json:"isImage"`
	// IsNotebook reports whether the file is a Jupyter notebook, previewed
	// by rendering its cells.
	IsNotebook bool    `json:"isNotebook"`
	Hunks      []*Hunk `json:"hunks"`
}

// Path returns the path used to identify the file. Deleted files are
// identified by their old path, everything else by the new path.
func (f *File) Path() string {
	if f.NewPath != "" && f.NewPath != DevNull {
		return f.NewPath
	}
	return f.OldPath
}

// DevNull is the path git uses for a missing side of a diff.
const DevNull = "/dev/null"

// Diff is one chunk of unified diff text handed to sbnn through stdin.
type Diff struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// BaseDir is the directory the diff paths are relative to. It is the
	// working directory of the sbnn invocation that sent the diff.
	BaseDir   string    `json:"baseDir"`
	CreatedAt time.Time `json:"createdAt"`
	// Labels are whatever the sender wanted to remember about this diff -
	// a revision, a branch, a ticket, a machine. sbnn stores them and gives
	// them no meaning at all, which is what lets a review be joined to
	// whatever else you keep, git or not.
	Labels map[string]string `json:"labels,omitempty"`
	Raw    string            `json:"raw"`
	Files  []*File           `json:"files"`
}

// Stats returns the total number of added and deleted lines of the diff.
func (d *Diff) Stats() (additions, deletions int) {
	for _, f := range d.Files {
		additions += f.Additions
		deletions += f.Deletions
	}
	return additions, deletions
}

// FindFile returns the file with the given ID.
func (d *Diff) FindFile(id string) *File {
	for _, f := range d.Files {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// Comment is a review comment attached to a range of lines of a file.
type Comment struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	DiffID string `json:"diffId"`
	FileID string `json:"fileId"`
	Path   string `json:"path"`
	// Author names who left the comment. It is empty for the reviewer in
	// the browser and set to something like "claude" when an agent writes
	// the comment from the command line.
	Author string `json:"author,omitempty"`
	// Side is "new" or "old" and tells which side of the diff the line
	// numbers belong to.
	Side string `json:"side"`
	// StartLine and EndLine are inclusive 1-based line numbers on Side.
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
	// Body is Markdown. Like on GitHub, a proposed replacement lives inside
	// it as a fenced "suggestion" block, which is what makes it travel
	// through a copied prompt unchanged.
	Body string `json:"body"`
	// Question marks a comment that wants an answer rather than a change.
	// The two are different requests and a reader cannot always tell them
	// apart from the prose, so whoever writes the comment says which it is
	// and nothing has to guess.
	Question bool `json:"question,omitempty"`
	// Snippet is the reviewed code, kept so that the comment stays
	// meaningful once it is exported as a prompt.
	Snippet   string    `json:"snippet"`
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Suggestions returns the replacements proposed inside a comment body.
//
// A suggestion is written the way GitHub writes one: a fenced block whose
// info string is "suggestion". Everything between the fences replaces the
// commented lines.
func Suggestions(body string) []string {
	var out []string
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		fence, ok := suggestionFence(lines[i])
		if !ok {
			continue
		}
		block := make([]string, 0, 4)
		// inner is the fence of the code block nested inside the
		// suggestion while the scan is inside one. Its lines are the
		// replacement text, not Markdown to be read, so a suggestion
		// may propose a file that itself contains a code block.
		inner := ""
		i++
	scan:
		for ; i < len(lines); i++ {
			line := lines[i]
			switch {
			case endsSuggestion(line, fence, inner):
				break scan
			case inner != "":
				if closesFence(line, inner) {
					inner = ""
				}
			default:
				if f, _, ok := openFence(line); ok {
					inner = f
				}
			}
			block = append(block, strings.TrimSuffix(line, "\r"))
		}
		out = append(out, strings.Join(block, "\n"))
	}
	return out
}

// endsSuggestion reports whether a line ends a suggestion block opened with
// fence, given the fence of the code block nested inside it - empty when the
// scan is not inside one.
//
// Inside a nested block only that block's own closing fence is read, which is
// what keeps a proposed code block whole. The exception is a run longer than
// the nested fence: a close never has to be longer than the fence it closes,
// while the suggestion's fence is lengthened precisely to hold shorter ones,
// so the longer run belongs to the suggestion.
func endsSuggestion(line, fence, inner string) bool {
	if !closesFence(line, fence) {
		return false
	}
	if inner == "" {
		return true
	}
	run, _ := fenceRun(line)
	return !closesFence(line, inner) || len(run) > len(inner)
}

// suggestionFence reports whether a line opens a suggestion block, and with
// which fence.
func suggestionFence(line string) (fence string, ok bool) {
	fence, info, ok := openFence(line)
	if !ok || !strings.EqualFold(info, "suggestion") {
		return "", false
	}
	return fence, true
}

// openFence reports whether a line opens a fenced block, with which fence and
// with which info string. A backtick fence may not carry a backtick in its
// info string, so a line of prose holding two code spans opens nothing.
func openFence(line string) (fence, info string, ok bool) {
	fence, info = fenceRun(line)
	if len(fence) < 3 {
		return "", "", false
	}
	if fence[0] == '`' && strings.Contains(info, "`") {
		return "", "", false
	}
	return fence, info, true
}

// closesFence reports whether a line closes a block opened with fence: the
// same character, at least as long, and nothing else on the line.
func closesFence(line, fence string) bool {
	run, rest := fenceRun(line)
	return rest == "" && len(run) >= len(fence) && run[0] == fence[0]
}

// fenceRun splits a line, once its surrounding space is gone, into the run of
// fence characters it begins with and what follows. run is empty when the
// line does not begin with a run of ` or ~.
func fenceRun(line string) (run, rest string) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return "", ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == trimmed[0] {
		n++
	}
	return trimmed[:n], strings.TrimSpace(trimmed[n:])
}

// WithSuggestion appends a suggestion block to a comment body, which is how
// a client that only has the replacement text - the sbnn command line - writes
// one.
func WithSuggestion(body, suggestion string) string {
	suggestion = strings.TrimRight(suggestion, "\n")
	if strings.TrimSpace(suggestion) == "" {
		return body
	}
	fence := "```"
	for strings.Contains(suggestion, fence) {
		fence += "`"
	}
	block := fence + "suggestion\n" + suggestion + "\n" + fence
	if strings.TrimSpace(body) == "" {
		return block
	}
	return strings.TrimRight(body, "\n") + "\n\n" + block
}

// MarshalJSON adds the suggestions parsed out of the body, so that a client
// does not have to know how they are written down.
func (c *Comment) MarshalJSON() ([]byte, error) {
	type comment Comment
	return json.Marshal(struct {
		*comment
		Suggestions []string `json:"suggestions,omitempty"`
	}{
		comment:     (*comment)(c),
		Suggestions: Suggestions(c.Body),
	})
}

// Group is a named collection of diffs and their review comments, served
// under its own URL path.
type Group struct {
	Name     string     `json:"name"`
	Diffs    []*Diff    `json:"diffs"`
	Comments []*Comment `json:"comments"`
	// ReviewedAt is when the human last said they were done. It is what
	// tells an agent that the comments are worth reading: a review is over
	// when the reviewer says so, not when the first comment appears.
	ReviewedAt time.Time `json:"reviewedAt,omitzero"`
	// ReviewNote is what the reviewer wrote when submitting, if anything.
	ReviewNote string `json:"reviewNote,omitempty"`
	// ReviewVerdict is what the reviewer decided about the change as a
	// whole, separately from what any single comment says.
	ReviewVerdict Verdict `json:"reviewVerdict,omitempty"`
	// Hooks are run when a review is submitted, so that work can carry on
	// even though whoever sent the diff is long gone.
	Hooks []*Hook `json:"hooks,omitempty"`
}

// Verdict is what a reviewer decided about the change as a whole.
//
// Counting comments does not answer it. A review can approve a change and
// still say three things about it, and a review can ask for changes without
// pointing at any single line. So the reviewer says which of the three it
// was, and everything downstream - the exit status, the prompt an agent
// reads, the log - repeats their answer instead of guessing at one.
type Verdict string

const (
	// VerdictApproved means the change can go ahead. Comments left with it
	// are worth reading, not blocking.
	VerdictApproved Verdict = "approved"
	// VerdictCommented means the reviewer had things to say and did not
	// decide either way. It is the default, and the honest answer when a
	// reviewer is not the one who gets to approve.
	VerdictCommented Verdict = "commented"
	// VerdictChangesRequested means the change should not go ahead as it
	// is.
	VerdictChangesRequested Verdict = "changes-requested"
)

// ParseVerdict reads a verdict, accepting the spellings people actually
// type. An empty string is "commented".
//
// The separators are thrown away before matching, so that every permutation
// of a two-word spelling means the same thing: "changes-requested" is what
// sbnn stores, "changes_requested" is what a GitHub review payload reports
// and "REQUEST_CHANGES" is what its API takes when submitting one. Whoever
// is bridging the two should not have to guess which of them we accept.
func ParseVerdict(s string) (Verdict, bool) {
	// A verdict left empty is "commented" - `sbnn review` with no --verdict
	// takes this path. It has to be decided on the raw text, before the
	// separators are dropped: "-_-" also folds down to nothing, and reading
	// that as a verdict would confirm a review, write it to the history and
	// fire the hook on what is plainly a typo.
	if strings.TrimSpace(s) == "" {
		return VerdictCommented, true
	}
	switch normalizeVerdict(s) {
	case "approved", "approve", "accept", "accepted", "lgtm", "ship", "shipit":
		return VerdictApproved, true
	case "commented", "comment":
		return VerdictCommented, true
	case "changesrequested", "requestchanges", "changes", "reject", "rejected":
		return VerdictChangesRequested, true
	}
	return "", false
}

// normalizeVerdict folds a written verdict down to letters, so that case and
// the separator someone reached for stop mattering.
func normalizeVerdict(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		// unicode.IsSpace, not a list of the ASCII ones: a verdict pasted out
		// of a terminal or an editor arrives padded with whatever that
		// program uses, and on a Japanese keyboard the leading space is
		// routinely U+3000.
		if unicode.IsSpace(r) {
			continue
		}
		switch r {
		case '-', '_', '.':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Blocking reports whether the verdict, on its own, says the change should
// not go ahead yet.
//
// It answers only half the question. A review that merely commented, or
// that carried no verdict at all, still blocks when it left a comment
// open - see Blocks, which is the rule sbnn actually ends on.
func (v Verdict) Blocking() bool {
	return v == VerdictChangesRequested
}

// Blocks reports whether a submitted review stops the change going ahead:
// the question sbnn answers with the exit status of wait --exit-code and
// submit --exit-code, and the one a review hook is told through
// SBNN_BLOCKING.
//
// The verdict outranks the comments but does not always settle it. An
// approval with three remarks on it is still an approval, and a review
// that asked for changes blocks even if it pointed at no line in
// particular. A review that only commented - or carried no verdict at all,
// as every review did before verdicts existed - blocks exactly when it
// left a comment open, which is the rule sbnn had before there was a
// verdict to consult.
//
// Both callers go through here so that the status sbnn exits with and the
// answer it hands a hook cannot drift apart.
func Blocks(v Verdict, comments []*Comment) bool {
	switch v {
	case VerdictApproved:
		return false
	case VerdictChangesRequested:
		return true
	}
	for _, c := range comments {
		if !c.Resolved {
			return true
		}
	}
	return false
}

// String makes a verdict readable in a sentence.
func (v Verdict) String() string {
	switch v {
	case VerdictApproved:
		return "approved"
	case VerdictChangesRequested:
		return "changes requested"
	default:
		return "commented"
	}
}

// Hook is what sbnn does when a review is submitted.
type Hook struct {
	ID string `json:"id"`
	// Command is run through the shell, with the prompt on its stdin.
	Command string `json:"command,omitempty"`
	// URL is sent a JSON POST describing the review.
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Reviewed reports whether the group was reviewed after its newest diff
// arrived. A diff sent after the last submission starts a new round.
func (g *Group) Reviewed() bool {
	if g.ReviewedAt.IsZero() {
		return false
	}
	for _, d := range g.Diffs {
		if d.CreatedAt.After(g.ReviewedAt) {
			return false
		}
	}
	return true
}

// FindDiff returns the diff with the given ID.
func (g *Group) FindDiff(id string) *Diff {
	for _, d := range g.Diffs {
		if d.ID == id {
			return d
		}
	}
	return nil
}
