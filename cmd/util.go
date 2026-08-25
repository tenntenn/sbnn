package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/browser"

	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/paths"
	"github.com/tenntenn/sbnn/internal/server"
)

// TargetEnv names the group to work on when --target is not given. It is
// there for whoever is driving sbnn - a shell, a script, an agent working in
// two checkouts at once - to say which review is theirs, once, instead of
// repeating the flag. sbnn itself has no idea what the name stands for.
const TargetEnv = "SBNN_TARGET"

// HistoryEnv points the log of submitted reviews somewhere else. Any of the
// words in HistoryOffWords keeps no log at all.
const HistoryEnv = "SBNN_HISTORY"

// HistoryOffWords are the spellings of "keep no log" accepted by
// --history-file and $SBNN_HISTORY. They are matched case-insensitively and
// after trimming spaces. The list is deliberately generous: someone turning
// the log off reaches for whichever of these words their other tools use,
// and a word that is not recognised silently becomes a file of that name
// with the reviews piling up in it.
var HistoryOffWords = []string{"off", "none", "no", "false", "0", "disabled"}

// historyStdinWord is the one spelling --history-file refuses outright: it
// names a standard stream everywhere else in sbnn, and a log cannot be
// appended to one.
const historyStdinWord = "-"

// historyFileHelp builds the help of a --history-file flag out of
// HistoryOffWords itself, so that a spelling the flag accepts cannot stay
// undocumented: the list grew from three words to six and the help still
// named only "off", which is the very thing that makes a near-miss such as
// --history-file false unguessable. lead is the part that differs between
// the commands, e.g. "Where the log is kept".
func historyFileHelp(lead string) string {
	quoted := make([]string, 0, len(HistoryOffWords))
	for _, word := range HistoryOffWords {
		quoted = append(quoted, strconv.Quote(word))
	}
	words := quoted[0]
	if len(quoted) > 1 {
		words = strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
	}
	return fmt.Sprintf("%s (%s for nowhere, %q is refused, or $%s)",
		lead, words, historyStdinWord, HistoryEnv)
}

// historyFile resolves where the reviews are written down: --history-file,
// then $SBNN_HISTORY, then the state directory. It is a plain path on purpose:
// a project that wants its reviews under version control points it into the
// repository and commits the file.
//
// "-" is rejected rather than taken as a file name. Elsewhere in sbnn - in
// "reviews --file" and in "comment --suggest" - it names a standard stream,
// and there is nothing to append a log to on stdout, so a bare dash here is
// far more likely to be a mistake than a request for a file called "-".
func historyFile(flag string) (string, error) {
	if flag == "" {
		flag = os.Getenv(HistoryEnv)
	}
	word := strings.ToLower(strings.TrimSpace(flag))
	if word == "" {
		return paths.HistoryFile()
	}
	if slices.Contains(HistoryOffWords, word) {
		return "", nil
	}
	if word == historyStdinWord {
		return "", fmt.Errorf("history file %q: a log cannot be written to a standard stream; pass a path, or %q to keep no log",
			flag, HistoryOffWords[0])
	}
	return filepath.Abs(flag)
}

// groupName resolves the group a command works on: --target, then
// $SBNN_TARGET, then the default group.
func groupName(flag string) (string, error) {
	if flag == "" {
		flag = os.Getenv(TargetEnv)
	}
	return server.ValidateGroupName(flag)
}

// outputFormats are the shapes a review can be printed in, in the order the
// flag help lists them.
var outputFormats = []string{"prompt", "markdown", "json"}

// resolveFormat folds --format and the --json shorthand into one name, and
// says so straight away when the name is not one sbnn knows. It is meant to
// be called before a command does any work: "sbnn wait" blocks for as long
// as the review takes, and a format it cannot print is worth knowing about
// before the wait rather than after it.
//
// --format is checked even when --json overrides it, because a --format
// nobody reads is a typo either way.
func resolveFormat(format string, asJSON bool) (string, error) {
	if !slices.Contains(outputFormats, format) {
		return "", fmt.Errorf("unknown format %q: use prompt, markdown or json", format)
	}
	if asJSON {
		return "json", nil
	}
	return format, nil
}

func jsonEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}

// lineEncoder writes one JSON object per line, which is what jsonl means.
func lineEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}

func browserOpen(url string) error {
	return browser.OpenURL(url)
}

// isTerminal reports whether a stream is attached to a terminal. sbnn is meant
// to sit in a pipeline as much as in a shell, and a pipeline has nobody to
// open a browser for or to answer a question.
//
// A character device is not enough on its own: the null device is one, so
// "sbnn --clear --all < /dev/null" - the shape a cron job, a systemd unit or
// a CI step reaches for to say "there is nobody here" - was read as a
// terminal, and the caller got a prompt answered by the EOF it could not see.
// Asking the kernel with an ioctl (golang.org/x/term) would settle it exactly,
// but the null device is the one character device that turns up as a stream in
// practice, and ruling it out costs no dependency.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return !isDevNull(fi)
}

// isDevNull reports whether fi describes the null device. It compares the
// file itself rather than its name, so /dev/stdin and the other paths that
// lead to it are caught too.
func isDevNull(fi os.FileInfo) bool {
	null, err := os.Stat(os.DevNull)
	if err != nil {
		return false
	}
	return os.SameFile(fi, null)
}

// ExitOpenComments is the status of a command that found comments to
// address, in the tradition of grep and diff: nothing to say is 0, something
// to say is 1, and a real failure is above that.
const ExitOpenComments = 1

// exitWithComments ends a --exit-code command.
func exitWithComments(comments []*model.Comment) error {
	for _, c := range comments {
		if !c.Resolved {
			os.Exit(ExitOpenComments)
		}
	}
	return nil
}

// exitWithVerdict ends a --exit-code command that knows what the reviewer
// decided, which outranks counting comments: an approval with three remarks
// on it is still an approval, and a review that asked for changes blocks
// even if it pointed at no line in particular.
func exitWithVerdict(v model.Verdict, comments []*model.Comment) error {
	switch v {
	case model.VerdictApproved:
		return nil
	case model.VerdictChangesRequested:
		os.Exit(ExitOpenComments)
	}
	return exitWithComments(comments)
}
