package cmd

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/history"
)

var (
	reviewsSince    string
	reviewsLimit    int
	reviewsFormat   string
	reviewsStats    bool
	reviewsAll      bool
	reviewsTop      int
	reviewsFile     string
	reviewsComments bool
)

var reviewsCmd = &cobra.Command{
	Use:   "reviews",
	Short: "List the reviews that were submitted, and what they say together",
	Long: `List the reviews that were submitted.

Every submitted review is written down: what was reviewed, what was said, how
long the change waited. A round of review is thrown away as soon as it is
over - comments cleared, group closed - and this is what stays, so that a
year of reviews can be read as one thing.

  $ sbnn reviews                     # newest last, one line each
  $ sbnn reviews --since 7d          # this week
  $ sbnn reviews -t api --limit 5
  $ sbnn reviews --stats             # what they say together, and only then

--comments turns the stream around: one record per comment instead of one
per review, which is the shape counting tools want. Parse the jsonl form -
one flat JSON object per line, fields get added but not renamed. The text
form is five tab-separated columns (date, group, path:lines, author, first
line of the body): fine for eyes and quick pipes, lossy by design.

  $ sbnn reviews --comments --format jsonl | jq -r 'select(.suggestions).path'
  $ sbnn reviews --comments | cut -f3 | sort | uniq -c | sort -rn

The log is a JSON object per line, kept at ` + "`$XDG_STATE_HOME/sbnn/reviews.jsonl`" + `
unless --history-file or $SBNN_HISTORY says otherwise, so jq and friends work
on it directly and it can be read from anywhere:

  $ sbnn reviews --file team.jsonl               # someone else's
  $ cat */reviews.jsonl | sbnn reviews --file -  # several at once

Keep the log outside the working tree (the default is outside): a log inside
the tree is appended to on every submit and would dirty the very diff it is
a log of. A record holds no absolute path and nothing about the machine that
wrote it, so a log reads the same wherever it ends up. And it is only a
file: a label you forgot to send can be added afterwards by rewriting it
with jq, and deleting it forgets the lot. Nothing leaves the machine on its
own.`,
	Args:         cobra.NoArgs,
	RunE:         runReviews,
	SilenceUsage: true,
}

func init() {
	f := reviewsCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Only the reviews of this group")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port (used when sbnn is running)")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVar(&historyPath, "history-file", "", historyFileHelp("Where the log is kept"))
	f.StringVar(&reviewsSince, "since", "", "Only reviews after this: 7d, 36h, 2026-01-31")
	f.IntVar(&reviewsLimit, "limit", 0, "Keep only the newest n reviews")
	f.StringVar(&reviewsFormat, "format", "text", "Output format: text, json or jsonl")
	f.BoolVar(&reviewsStats, "stats", false, "Print what the reviews say together")
	f.BoolVar(&reviewsComments, "comments", false, "One record per comment instead of one per review")
	reviewsCmd.MarkFlagsMutuallyExclusive("comments", "stats")
	f.BoolVar(&reviewsAll, "all", false, "Every group, ignoring --target and $SBNN_TARGET")
	f.IntVar(&reviewsTop, "top", 5, "How many entries each tally shows")
	f.BoolVar(&jsonOutput, "json", false, "Shorthand for --format json")
	f.StringVar(&reviewsFile, "file", "",
		`Read this log instead of the usual one ("-" reads stdin)`)
}

func runReviews(cmd *cobra.Command, _ []string) error {
	filter := history.Filter{Limit: reviewsLimit}
	if !reviewsAll {
		filter.Group = target
		if filter.Group == "" {
			filter.Group = os.Getenv(TargetEnv)
		}
	}
	if reviewsSince != "" {
		since, err := history.ParseSince(reviewsSince, time.Now())
		if err != nil {
			return err
		}
		filter.Since = since
	}

	records, err := loadReviews(cmd, filter)
	if err != nil {
		return err
	}

	format := reviewsFormat
	if jsonOutput {
		format = "json"
	}
	if reviewsComments {
		return printCommentStream(os.Stdout, history.Comments(records), format)
	}
	switch format {
	case "json":
		return jsonEncoder(os.Stdout).Encode(map[string]any{
			"reviews": records,
			"stats":   history.Summarize(records),
		})
	case "jsonl":
		enc := lineEncoder(os.Stdout)
		for _, rec := range records {
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	case "text":
		return printReviews(records)
	default:
		return fmt.Errorf("unknown format %q: use text, json or jsonl", format)
	}
}

// loadReviews reads the reviews: from stdin, from the log the flags point
// at, or - when nothing was pointed at and the usual log has nothing to say
// - from the running server, which may be keeping them somewhere this
// invocation was not told about.
//
// A log named with --file is read and nothing else is: answering a question
// about someone else's log with this machine's reviews would be a lie, and
// a quiet one.
func loadReviews(cmd *cobra.Command, filter history.Filter) ([]history.Record, error) {
	if reviewsFile == "-" {
		return history.Read(cmd.InOrStdin(), filter)
	}
	if reviewsFile != "" {
		if _, err := os.Stat(reviewsFile); err != nil {
			return nil, fmt.Errorf("cannot read the log %s: %w", reviewsFile, err)
		}
		return history.Load(reviewsFile, filter)
	}
	path, err := historyFile(historyPath)
	if err != nil {
		return nil, err
	}
	if path != "" {
		records, err := history.Load(path, filter)
		if err != nil {
			return nil, err
		}
		if len(records) > 0 {
			return records, nil
		}
	}
	c := client.New(addr(), 5*time.Second)
	if _, err := c.Status(cmd.Context()); err != nil {
		return nil, nil
	}
	return c.Reviews(cmd.Context(), filter)
}

// printCommentStream writes one record per comment: flat lines that sort,
// cut, awk and jq can take from here.
//
// The jsonl form is the stable interface: fields may be added to it, the
// existing ones stay. The text form is five tab-separated columns - date,
// group, path:lines, author, first line of the body - locked by a test
// because pipes depend on positions, and lossy by design: the whole body
// only travels in jsonl.
func printCommentStream(w io.Writer, comments []history.CommentRecord, format string) error {
	switch format {
	case "json":
		return jsonEncoder(w).Encode(comments)
	case "jsonl":
		enc := lineEncoder(w)
		for _, c := range comments {
			if err := enc.Encode(c); err != nil {
				return err
			}
		}
		return nil
	case "text":
		for _, c := range comments {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				c.ReviewedAt.Local().Format("2006-01-02"),
				c.Group,
				lineRef(c),
				c.Who(),
				firstLine(c.Body))
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q: use text, json or jsonl", format)
	}
}

func lineRef(c history.CommentRecord) string {
	if c.EndLine > c.StartLine {
		return fmt.Sprintf("%s:%d-%d", c.Path, c.StartLine, c.EndLine)
	}
	return fmt.Sprintf("%s:%d", c.Path, c.StartLine)
}

// firstLine keeps a body to one field of one line.
func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return strings.ReplaceAll(s, "\t", " ")
}

func printReviews(records []history.Record) error {
	if len(records) == 0 {
		fmt.Println("no review has been submitted yet")
		return nil
	}
	if reviewsStats {
		// The aggregate is printed when asked for and only then; the list
		// is a list.
		printStats(history.Summarize(records))
		return nil
	}
	for _, rec := range records {
		fmt.Printf("%s  %-14s %2d comment(s)%s  %d file(s), +%d -%d%s%s\n",
			rec.ReviewedAt.Local().Format("2006-01-02 15:04"),
			rec.Group,
			len(rec.Comments),
			suggestionCount(rec),
			rec.Files, rec.Additions, rec.Deletions,
			waited(rec),
			labelPairs(rec))
		if note := strings.TrimSpace(rec.Note); note != "" {
			fmt.Printf("%s\n", indent(note, "      "))
		}
	}
	return nil
}

// labelPairs puts the labels a review was sent with on its line, sorted so
// that two runs read the same.
func labelPairs(rec history.Record) string {
	if len(rec.Labels) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(rec.Labels))
	for _, k := range slices.Sorted(maps.Keys(rec.Labels)) {
		pairs = append(pairs, k+"="+rec.Labels[k])
	}
	return "  " + strings.Join(pairs, " ")
}

func suggestionCount(rec history.Record) string {
	n := 0
	for _, c := range rec.Comments {
		n += len(c.Suggestions)
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", %d suggestion(s)", n)
}

func waited(rec history.Record) string {
	if w := rec.Wait(); w > 0 {
		return "  waited " + shortDuration(w)
	}
	return ""
}

func printStats(s history.Stats) {
	fmt.Printf("%d review(s), %d comment(s) (%.1f per review), %d suggestion(s)\n",
		s.Reviews, s.Comments, s.CommentsPerReview, s.Suggestions)
	fmt.Printf("%d review(s) had nothing to say, %d comment(s) were resolved\n", s.Silent, s.Resolved)
	fmt.Printf("%d file(s) reviewed, +%d -%d\n", s.Files, s.Additions, s.Deletions)
	if s.MedianWait > 0 {
		fmt.Printf("median wait from diff to review: %s\n", shortDuration(s.MedianWait))
	}
	printTally("most commented", s.Paths)
	printTally("by kind of file", s.Extensions)
	printTally("by author", s.Authors)
}

func printTally(title string, counts []history.Count) {
	if len(counts) == 0 {
		return
	}
	if reviewsTop > 0 && len(counts) > reviewsTop {
		counts = counts[:reviewsTop]
	}
	fmt.Printf("%s:\n", title)
	for _, c := range counts {
		fmt.Printf("  %-40s %d\n", c.Key, c.Count)
	}
}

// shortDuration writes a duration the way someone would say it.
func shortDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
