package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

var (
	commentBody        string
	commentSuggest     string
	commentSuggestFrom string
	commentAuthor      string
	commentQuestion    bool
	commentSide        string
	commentDiffID      string
	commentBulk        bool
	commentJSONOut     bool
)

// DefaultAuthor labels comments written from the command line, so that the
// browser and the prompt can tell them apart from the reviewer's own.
const DefaultAuthor = "agent"

var commentCmd = &cobra.Command{
	Use:   "comment [path:line[-line]]",
	Short: "Leave a review comment from the command line",
	Long: `Leave a review comment on a diff that was already sent to sbnn.

This is the other direction of the review loop: an agent can point at the
lines it is unsure about, ask a question, or propose a change, and the human
sees it in the browser next to the diff.

  $ sbnn comment internal/server/server.go:120 -m "Should this be a 404?"
  $ sbnn comment README.md:12-18 -m "Reworded" --suggest "$(cat new.md)"
  $ cat new.txt | sbnn comment main.go:42 -m "Simpler" --suggest -
  $ sbnn comment --author claude cmd/root.go:88 -m "Left over from the old flag"
  $ sbnn comment --question main.go:42 -m "Is a 404 right here, or a 409?"

The line numbers are the ones of the new side of the diff, the same ones the
diff shows; --side old comments on a removed line instead. The file is looked
up in the newest diff of the group that carries that path, unless --diff
names one. sbnn fills in the reviewed code itself.

A suggestion is a fenced block inside the comment, the way GitHub writes one,
so it can also be typed straight into -m.

Many at once, for a whole self review:

  $ sbnn comment --json <<'EOF'
  [
    {"path": "cmd/root.go", "line": "88", "body": "left over"},
    {"path": "README.md", "line": "12-18", "body": "reworded", "suggestion": "..."}
  ]
  EOF

Each entry carries its own text, so --message, --suggest and --suggest-file
are refused next to --json: the array is already on stdin, and --suggest -
would be reading the same stdin the array comes from. Of the rest, --author,
--diff and --question act as defaults for entries that leave "author",
"diffId" or "question" out; the side is taken from the entry alone.

Comments made this way are read back exactly like the ones written in the
browser, with ` + "`sbnn comments`" + `.`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runComment,
	SilenceUsage: true,
}

func init() {
	f := commentCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVarP(&commentBody, "message", "m", "", "Comment body")
	f.StringVar(&commentSuggest, "suggest", "",
		`Suggested replacement, appended to the body as a "suggestion" block ("-" reads stdin)`)
	f.StringVar(&commentSuggestFrom, "suggest-file", "", "Suggested replacement read from a file")
	f.StringVar(&commentAuthor, "author", DefaultAuthor, "Who is commenting")
	f.BoolVar(&commentQuestion, "question", false,
		"Mark it as a question: it wants an answer, not a change")
	f.StringVar(&commentSide, "side", "new", "Side of the diff the lines belong to: new or old")
	f.StringVar(&commentDiffID, "diff", "", "Diff ID (default: the newest diff carrying the path)")
	f.BoolVar(&commentBulk, "json", false, "Read a JSON array of comments from stdin")
	f.BoolVar(&commentJSONOut, "json-output", false, "Print the stored comments as JSON")

	// --json takes every comment from stdin, so nothing else may read stdin or
	// claim to hold the one comment's text. Marked in pairs rather than as one
	// group of four, because --message and --suggest belong together: a
	// suggestion usually comes with a sentence saying why.
	commentCmd.MarkFlagsMutuallyExclusive("json", "message")
	commentCmd.MarkFlagsMutuallyExclusive("json", "suggest")
	commentCmd.MarkFlagsMutuallyExclusive("json", "suggest-file")
}

func runComment(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	group, err := groupName(target)
	if err != nil {
		return err
	}
	c := client.New(addr(), 10*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}

	var requests []server.AddCommentRequest
	if commentBulk {
		if len(args) > 0 {
			return fmt.Errorf("--json reads every comment from stdin, so %q is one too many", args[0])
		}
		requests, err = readBulkComments(os.Stdin)
	} else {
		if len(args) != 1 {
			return fmt.Errorf("say which lines to comment on, as path:line or path:line-line")
		}
		requests, err = singleComment(args[0])
	}
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return fmt.Errorf("no comment to add")
	}

	stored := make([]*model.Comment, 0, len(requests))
	for _, req := range requests {
		added, err := c.AddComment(ctx, group, req)
		if err != nil {
			return err
		}
		stored = append(stored, added)
		if !commentJSONOut {
			fmt.Fprintf(os.Stderr, "sbnn: %s on %s%s\n", added.ID, added.Path, lineRangeOf(added))
		}
	}
	if commentJSONOut {
		return jsonEncoder(os.Stdout).Encode(stored)
	}
	fmt.Println(server.GroupURL(c.BaseURL(), group))
	return nil
}

func lineRangeOf(c *model.Comment) string {
	if c.EndLine > c.StartLine {
		return fmt.Sprintf(":%d-%d", c.StartLine, c.EndLine)
	}
	return fmt.Sprintf(":%d", c.StartLine)
}

// singleComment builds the request for "path:line" or "path:line-line".
func singleComment(spec string) ([]server.AddCommentRequest, error) {
	path, start, end, err := parseLineSpec(spec)
	if err != nil {
		return nil, err
	}
	suggestion, err := suggestionText()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(commentBody) == "" && suggestion == "" {
		return nil, fmt.Errorf("say something with -m, or propose something with --suggest")
	}
	side, err := normalizeSide(commentSide)
	if err != nil {
		return nil, err
	}
	return []server.AddCommentRequest{{
		DiffID:     commentDiffID,
		Path:       path,
		Author:     commentAuthor,
		Side:       side,
		StartLine:  start,
		EndLine:    end,
		Body:       commentBody,
		Question:   commentQuestion,
		Suggestion: suggestion,
	}}, nil
}

// suggestionText resolves --suggest and --suggest-file.
func suggestionText() (string, error) {
	switch {
	case commentSuggest != "" && commentSuggestFrom != "":
		return "", fmt.Errorf("use either --suggest or --suggest-file, not both")
	case commentSuggestFrom != "":
		b, err := os.ReadFile(commentSuggestFrom)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\n"), nil
	case commentSuggest == "-":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			return "", fmt.Errorf("cannot read the suggestion from stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\n"), nil
	default:
		return commentSuggest, nil
	}
}

// bulkComment is one entry of the JSON array read by --json.
type bulkComment struct {
	Path       string    `json:"path"`
	Line       flexLines `json:"line"`
	StartLine  int       `json:"startLine"`
	EndLine    int       `json:"endLine"`
	Side       string    `json:"side"`
	Body       string    `json:"body"`
	Suggestion string    `json:"suggestion"`
	// Question is a pointer so an entry that says "question": false keeps its
	// false even when --question sets the default for the entries that leave
	// the field out, the same way an empty "author" falls back to --author.
	Question *bool  `json:"question"`
	Author   string `json:"author"`
	DiffID   string `json:"diffId"`
}

// flexLines accepts "12", "12-18" and 12 alike.
type flexLines struct {
	Start, End int
}

func (l *flexLines) UnmarshalJSON(b []byte) error {
	text := strings.TrimSpace(string(b))
	if text == "null" || text == `""` {
		return nil
	}
	if text[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		start, end, err := parseLines(s)
		if err != nil {
			return err
		}
		l.Start, l.End = start, end
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("line must be a number or a string like \"12-18\": %w", err)
	}
	l.Start, l.End = n, n
	return nil
}

func readBulkComments(r io.Reader) ([]server.AddCommentRequest, error) {
	var entries []bulkComment
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("cannot read the comments from stdin: %w", err)
	}
	requests := make([]server.AddCommentRequest, 0, len(entries))
	for i, e := range entries {
		if e.Path == "" {
			return nil, fmt.Errorf("comment %d has no path", i+1)
		}
		start, end := e.Line.Start, e.Line.End
		if start == 0 {
			start, end = e.StartLine, e.EndLine
		}
		if start <= 0 {
			return nil, fmt.Errorf("comment %d (%s) has no line", i+1, e.Path)
		}
		if end < start {
			end = start
		}
		if strings.TrimSpace(e.Body) == "" && strings.TrimSpace(e.Suggestion) == "" {
			return nil, fmt.Errorf("comment %d (%s) says nothing", i+1, e.Path)
		}
		side, err := normalizeSide(e.Side)
		if err != nil {
			return nil, fmt.Errorf("comment %d (%s): %w", i+1, e.Path, err)
		}
		author := e.Author
		if author == "" {
			author = commentAuthor
		}
		diffID := e.DiffID
		if diffID == "" {
			diffID = commentDiffID
		}
		question := commentQuestion
		if e.Question != nil {
			question = *e.Question
		}
		requests = append(requests, server.AddCommentRequest{
			DiffID:     diffID,
			Path:       e.Path,
			Author:     author,
			Side:       side,
			StartLine:  start,
			EndLine:    end,
			Body:       e.Body,
			Question:   question,
			Suggestion: e.Suggestion,
		})
	}
	return requests, nil
}

func normalizeSide(side string) (string, error) {
	switch side {
	case "", "new":
		return "new", nil
	case "old":
		return "old", nil
	default:
		return "", fmt.Errorf("unknown side %q: use new or old", side)
	}
}

var lineSpecPattern = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)

// parseLineSpec splits "path:12" or "path:12-18". Paths may contain colons on
// some systems, so the last colon wins.
func parseLineSpec(spec string) (path string, start, end int, err error) {
	i := strings.LastIndex(spec, ":")
	if i <= 0 || i == len(spec)-1 {
		return "", 0, 0, fmt.Errorf("cannot read %q: say path:line or path:line-line", spec)
	}
	start, end, err = parseLines(spec[i+1:])
	if err != nil {
		return "", 0, 0, fmt.Errorf("cannot read %q: %w", spec, err)
	}
	return spec[:i], start, end, nil
}

func parseLines(s string) (start, end int, err error) {
	m := lineSpecPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0, fmt.Errorf("%q is not a line or a line range", s)
	}
	start, _ = strconv.Atoi(m[1])
	end = start
	if m[2] != "" {
		end, _ = strconv.Atoi(m[2])
	}
	if start <= 0 || end < start {
		return 0, 0, fmt.Errorf("%q is not a line range", s)
	}
	return start, end, nil
}
