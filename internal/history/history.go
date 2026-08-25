// Package history keeps a record of the reviews that were submitted.
//
// A review is a small, honest sample of what a person cares about in code,
// and it is thrown away the moment the round ends: comments are cleared, the
// group is closed. Writing each submitted review down turns those samples
// into something to look back at - which files draw comments, how much of a
// review is a suggested rewrite, how long a change waits.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// Record is one submitted review.
type Record struct {
	Group      string    `json:"group"`
	ReviewedAt time.Time `json:"reviewedAt"`
	// FirstDiffAt is when the earliest diff of the round arrived, so that
	// the wait between asking for a review and getting one can be read.
	FirstDiffAt time.Time `json:"firstDiffAt,omitzero"`
	Diffs       int       `json:"diffs"`
	Files       int       `json:"files"`
	Additions   int       `json:"additions"`
	Deletions   int       `json:"deletions"`
	Note        string    `json:"note,omitempty"`
	// Verdict is what the reviewer decided about the change as a whole:
	// approved, commented or changes-requested.
	Verdict model.Verdict `json:"verdict,omitempty"`
	// Labels are what the diffs of this round were sent with. A later diff
	// wins over an earlier one for the same key.
	Labels   map[string]string `json:"labels,omitempty"`
	Comments []Comment         `json:"comments"`
}

// Comment is what a record keeps of a review comment: enough to see the
// pattern, including what was said, which is where the pattern lives.
type Comment struct {
	Path        string   `json:"path"`
	Author      string   `json:"author,omitempty"`
	Side        string   `json:"side"`
	StartLine   int      `json:"startLine"`
	EndLine     int      `json:"endLine"`
	Body        string   `json:"body"`
	Suggestions []string `json:"suggestions,omitempty"`
	// Question marks a comment that wanted an answer, not a change.
	Question  bool      `json:"question,omitempty"`
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"createdAt"`
}

// Wait is how long the change waited for its review.
func (r Record) Wait() time.Duration {
	if r.FirstDiffAt.IsZero() || r.ReviewedAt.Before(r.FirstDiffAt) {
		return 0
	}
	return r.ReviewedAt.Sub(r.FirstDiffAt)
}

// FromGroup takes the record of a group as it was submitted.
func FromGroup(g *model.Group) Record {
	rec := Record{
		Group:      g.Name,
		ReviewedAt: g.ReviewedAt,
		Note:       g.ReviewNote,
		Verdict:    g.ReviewVerdict,
		Diffs:      len(g.Diffs),
		Comments:   make([]Comment, 0, len(g.Comments)),
	}
	for _, d := range g.Diffs {
		if rec.FirstDiffAt.IsZero() || d.CreatedAt.Before(rec.FirstDiffAt) {
			rec.FirstDiffAt = d.CreatedAt
		}
		for k, v := range d.Labels {
			if rec.Labels == nil {
				rec.Labels = map[string]string{}
			}
			rec.Labels[k] = v
		}
		rec.Files += len(d.Files)
		adds, dels := d.Stats()
		rec.Additions += adds
		rec.Deletions += dels
	}
	for _, c := range g.Comments {
		rec.Comments = append(rec.Comments, Comment{
			Path:        c.Path,
			Author:      c.Author,
			Side:        c.Side,
			StartLine:   c.StartLine,
			EndLine:     c.EndLine,
			Body:        c.Body,
			Suggestions: model.Suggestions(c.Body),
			Question:    c.Question,
			Resolved:    c.Resolved,
			CreatedAt:   c.CreatedAt,
		})
	}
	return rec
}

// Append writes a record to the log, one JSON object per line so that the
// file stays readable by anything, sbnn included.
//
// The log is one file for the whole machine while a session file is one
// per port, so several servers appending at once is the ordinary case, not
// a corner. O_APPEND alone only settles where a write starts, not that it
// arrives in one piece: a record carrying comment bodies and their
// snippets is far past any size a write is promised to be atomic at, and
// two of them landing inside one another make a line that no longer
// parses - a review that quietly vanishes from sbnn reviews, since Read
// skips what it cannot read. So the write is made under an exclusive
// advisory lock on the file.
func Append(path string, rec Record) error {
	if path == "" {
		return nil
	}
	// Marshalled before the lock is taken: the lock is for the write.
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	unlock, err := lockForAppend(f)
	if err != nil {
		return fmt.Errorf("locking %s: %w", path, err)
	}
	defer unlock()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// Filter narrows what Load returns.
type Filter struct {
	// Group keeps only the reviews of one group.
	Group string
	// Since keeps only the reviews submitted after a moment.
	Since time.Time
	// Limit keeps only the newest n records; 0 keeps all of them.
	Limit int
}

// MaxRecordBytes is how long a line may be and still be read as a record.
// A review of a generated file, where every comment carries its snippet,
// can run to megabytes; past this the line is skipped, the way an
// unparseable one is.
const MaxRecordBytes = 8 << 20

// Skipped is what a read had to leave out. A log is worth reading even
// when part of it cannot be, so these are counted and handed back rather
// than raised: the reader gets the records that are there, plus something
// honest to say about the ones that are not.
type Skipped struct {
	// Broken is lines that were not a record.
	Broken int
	// Long is lines that ran past MaxRecordBytes.
	Long int
}

// Any reports whether anything was left out.
func (s Skipped) Any() bool { return s.Broken > 0 || s.Long > 0 }

// String says what was left out, ready to put in a notice.
func (s Skipped) String() string {
	var parts []string
	if s.Broken > 0 {
		parts = append(parts, fmt.Sprintf("%d unreadable line(s)", s.Broken))
	}
	if s.Long > 0 {
		parts = append(parts, fmt.Sprintf("%d line(s) over %d MiB", s.Long, MaxRecordBytes>>20))
	}
	return strings.Join(parts, ", ")
}

// Load reads the log, oldest first, applying a filter. Lines it cannot use
// are skipped; LoadSkipped says how many there were.
func Load(path string, f Filter) ([]Record, error) {
	records, _, err := LoadSkipped(path, f)
	return records, err
}

// LoadSkipped is Load, also reporting what it had to skip.
func LoadSkipped(path string, f Filter) ([]Record, Skipped, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Skipped{}, nil
		}
		return nil, Skipped{}, err
	}
	defer file.Close()
	return ReadSkipped(file, f)
}

// Read parses a log, skipping the lines it cannot use.
func Read(r io.Reader, f Filter) ([]Record, error) {
	records, _, err := ReadSkipped(r, f)
	return records, err
}

// ReadSkipped is Read, also reporting what it had to skip. Only a failure
// of the underlying reader is an error: a line that is not a record, or is
// too long to be one, costs that line and nothing else.
func ReadSkipped(r io.Reader, f Filter) ([]Record, Skipped, error) {
	var records []Record
	var skipped Skipped
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, long, err := readLine(br)
		switch {
		case long:
			skipped.Long++
		case len(bytes.TrimSpace(line)) == 0:
			// A blank line is not a record and not a complaint.
		default:
			var rec Record
			if jerr := json.Unmarshal(bytes.TrimSpace(line), &rec); jerr != nil {
				skipped.Broken++
			} else if keep(rec, f) {
				records = append(records, rec)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, skipped, err
		}
	}
	if f.Limit > 0 && len(records) > f.Limit {
		records = records[len(records)-f.Limit:]
	}
	return records, skipped, nil
}

// keep says whether a record survives the filter.
func keep(rec Record, f Filter) bool {
	if f.Group != "" && rec.Group != f.Group {
		return false
	}
	if !f.Since.IsZero() && rec.ReviewedAt.Before(f.Since) {
		return false
	}
	return true
}

var newline = []byte("\n")

// readLine reads one newline-terminated line. A line past MaxRecordBytes is
// drained to its end and reported with long set, so that the lines after it
// are still read - which bufio.Scanner cannot do, since it stops the whole
// scan at the first line that outgrows its buffer.
func readLine(br *bufio.Reader) (line []byte, long bool, err error) {
	for {
		chunk, rerr := br.ReadSlice('\n')
		if rerr == bufio.ErrBufferFull {
			// ReadSlice hands back the buffer itself, so keeping any of it
			// means copying it out.
			if !long && len(line)+len(chunk) <= MaxRecordBytes {
				line = append(line, chunk...)
				continue
			}
			long, line = true, nil
			continue
		}
		if !long {
			line = append(line, chunk...)
			// The terminator is not part of the record.
			if len(bytes.TrimSuffix(line, newline)) > MaxRecordBytes {
				long, line = true, nil
			}
		}
		if rerr != nil {
			// io.EOF, with whatever came before it, or a read failure.
			if rerr == io.EOF && !long && len(line) == 0 {
				return nil, false, io.EOF
			}
			return line, long, rerr
		}
		return line, long, nil
	}
}

// CommentRecord is one comment standing on its own: what was said, plus as
// much of the review it came from as it takes to make sense of it. A comment
// is the thing worth counting, and counting is done by whatever the reader
// pipes this into, so each one has to survive on a line by itself.
type CommentRecord struct {
	Group      string            `json:"group"`
	ReviewedAt time.Time         `json:"reviewedAt"`
	Labels     map[string]string `json:"labels,omitempty"`
	Comment
}

// Extension is the kind of file the comment was left on, ".go" or "(none)".
func (c CommentRecord) Extension() string {
	return extension(c.Path)
}

// Who wrote it, with the browser's unnamed comments credited to the
// reviewer.
func (c CommentRecord) Who() string {
	return author(c.Author)
}

// Comments pulls every comment out of the reviews, in the order they were
// reviewed.
func Comments(records []Record) []CommentRecord {
	out := make([]CommentRecord, 0, len(records))
	for _, rec := range records {
		for _, c := range rec.Comments {
			out = append(out, CommentRecord{
				Group:      rec.Group,
				ReviewedAt: rec.ReviewedAt,
				Labels:     rec.Labels,
				Comment:    c,
			})
		}
	}
	return out
}

// Count is one line of a tally.
type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// Stats is what a pile of reviews says when read together.
type Stats struct {
	Reviews     int `json:"reviews"`
	Comments    int `json:"comments"`
	Suggestions int `json:"suggestions"`
	Resolved    int `json:"resolved"`
	Files       int `json:"files"`
	Additions   int `json:"additions"`
	Deletions   int `json:"deletions"`
	// Silent is how many reviews were submitted with nothing to say.
	Silent int `json:"silent"`
	// Approved, Commented and ChangesRequested are how many reviews
	// decided each way. Counting comments does not answer what a review
	// decided - an approval can carry three comments and a request for
	// changes can carry none - which is the whole reason a verdict is
	// written down, so a summary that leaves it out cannot be read for the
	// one thing each review was for.
	Approved         int `json:"approved"`
	Commented        int `json:"commented"`
	ChangesRequested int `json:"changesRequested"`
	// CommentsPerReview is the mean, kept as a float on purpose: 2.8 says
	// more than 3.
	CommentsPerReview float64 `json:"commentsPerReview"`
	// MedianWait is the middle wait between a diff and its review.
	MedianWait time.Duration `json:"medianWaitNanos"`
	// Paths, Extensions and Authors are tallies, most first.
	Paths      []Count   `json:"paths"`
	Extensions []Count   `json:"extensions"`
	Authors    []Count   `json:"authors"`
	First      time.Time `json:"first,omitzero"`
	Last       time.Time `json:"last,omitzero"`
}

// Summarize reads a pile of reviews together.
func Summarize(records []Record) Stats {
	s := Stats{Reviews: len(records)}
	paths := map[string]int{}
	exts := map[string]int{}
	authors := map[string]int{}
	waits := make([]time.Duration, 0, len(records))

	for _, rec := range records {
		s.Files += rec.Files
		s.Additions += rec.Additions
		s.Deletions += rec.Deletions
		if len(rec.Comments) == 0 {
			s.Silent++
		}
		// A record written before the verdict was recorded has an empty
		// one, which reads as "commented" - the default ParseVerdict and
		// the API already apply - so an old log is counted, not dropped.
		switch rec.Verdict {
		case model.VerdictApproved:
			s.Approved++
		case model.VerdictChangesRequested:
			s.ChangesRequested++
		default:
			s.Commented++
		}
		if s.First.IsZero() || rec.ReviewedAt.Before(s.First) {
			s.First = rec.ReviewedAt
		}
		if rec.ReviewedAt.After(s.Last) {
			s.Last = rec.ReviewedAt
		}
		if w := rec.Wait(); w > 0 {
			waits = append(waits, w)
		}
		for _, c := range rec.Comments {
			s.Comments++
			s.Suggestions += len(c.Suggestions)
			if c.Resolved {
				s.Resolved++
			}
			paths[c.Path]++
			exts[extension(c.Path)]++
			authors[author(c.Author)]++
		}
	}
	if s.Reviews > 0 {
		s.CommentsPerReview = float64(s.Comments) / float64(s.Reviews)
	}
	s.MedianWait = median(waits)
	s.Paths = tally(paths)
	s.Extensions = tally(exts)
	s.Authors = tally(authors)
	return s
}

func author(name string) string {
	if name == "" {
		// The comments written in the browser are the reviewer's own.
		return "reviewer"
	}
	return name
}

func extension(p string) string {
	ext := path.Ext(p)
	if ext == "" {
		return "(none)"
	}
	return strings.ToLower(ext)
}

func median(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

// tally turns a map into a list, most first and ties by name so that two
// runs of the same data read the same.
func tally(counts map[string]int) []Count {
	out := make([]Count, 0, len(counts))
	for key, n := range counts {
		out = append(out, Count{Key: key, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// daysRE matches the "7d" form whole, so that "7days" or "7d3h" - which
// plainly mean something other than seven days - are refused rather than
// silently read as the part before the first "d".
var daysRE = regexp.MustCompile(`^([0-9]+)d$`)

// ParseSince reads "7d", "36h", "90m" or an RFC3339 date as a starting
// point. A date without a zone is read in the local zone.
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if m := daysRE.FindStringSubmatch(s); m != nil {
		if days, err := strconv.Atoi(m[1]); err == nil && days > 0 {
			return now.AddDate(0, 0, -days), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		// In the reviewer's own zone: everything the reviews output shows
		// is formatted with .Local(), so a bare date has to start where
		// that date starts on their clock, not nine hours into it. An
		// RFC3339 string carries its own offset and is unaffected.
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %q as a date or a duration like 7d, 36h or 2026-01-31", s)
}
