// Package cmd implements the sbnn command line interface.
package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenntenn/sbnn/internal/client"
	"github.com/tenntenn/sbnn/internal/mo"
	"github.com/tenntenn/sbnn/internal/server"
	"github.com/tenntenn/sbnn/version"
)

// DefaultPort is the port the sbnn server listens on. mo uses 6275, sbnn sits
// next to it.
const DefaultPort = 6280

// maxDiffSize bounds what sbnn reads from stdin.
const maxDiffSize = 32 << 20

var (
	target      string
	port        int
	bind        string
	title       string
	openBrowser bool
	noOpen      bool
	foreground  bool
	showStatus  bool
	doShutdown  bool
	doRestart   bool
	doClear     bool
	clearAll    bool
	assumeYes   bool
	jsonOutput  bool
	moBin       string
	moPort      int
	moBind      string
	allowRemote bool

	onReviewCommand string
	onReviewURL     string
	historyPath     string
	labelFlags      []string
	collapseFlags   []string
)

var rootCmd = &cobra.Command{
	Use:   "sbnn",
	Short: "sbnn reviews a unified diff in your browser",
	Long: `sbnn serves a unified diff read from stdin as a review page in your browser.

sbnn never runs git. Whatever produces a diff - git, jj, diff -u, a patch file,
a coding agent - can pipe it in:

  git diff | sbnn
  git diff HEAD~3 | sbnn --target refactor
  diff -u old.md new.md | sbnn
  cat change.patch | sbnn

Single server, growing session:
  sbnn runs in the background on port 6280. The first invocation starts it and
  returns the shell right away; later invocations add their diff to the
  running server instead of starting a new one, the same way mo does.

  $ git diff | sbnn                 # starts the server, shows the diff
  $ git diff --cached | sbnn        # adds another diff to the same page

Groups:
  --target (-t) puts diffs into a named group with its own URL and its own
  review comments.

  $ git diff | sbnn --target api    # http://localhost:6280/api

New files:
  A new file has no left hand side, so it is always shown as a unified diff.

Markdown preview:
  Markdown files are previewed in a split pane next to the diff. sbnn renders
  the preview itself, and mo (https://github.com/k1LoW/mo) renders a richer
  one for those who install it - the page has a switch for the two. The
  working tree file is previewed when it exists; otherwise sbnn reconstructs
  the new side from the diff itself. For mo: ` + mo.InstallHint + `.

Review comments:
  Comments can be attached to lines in the browser. They are stored by the
  sbnn server, not in the browser, so an agent can read them back:

  $ sbnn comments                   # comments as a prompt for an agent
  $ sbnn comments --format json     # comments as JSON
  $ sbnn comments --clear           # start the next review round

  They go the other way too: an agent can point at the lines it is unsure
  about, and the human sees it next to the diff.

  $ sbnn comment main.go:42 -m "Should this be a 404?" --author claude

Reviewing without a browser:
  A reviewer that is not a person leaves its comments the same way and ends
  the round itself, which is what the Submit button does.

  $ sbnn comment main.go:42 -m "..." --author reviewer
  $ sbnn submit --note "one thing to fix"

Waiting for the review:
  A review lands when the human is ready, which may be after a meeting or a
  night. sbnn can wait for it, or start something itself when it arrives.

  $ sbnn wait                       # blocks until "Submit review" is pressed
  $ git diff | sbnn --on-review 'claude -p "$(sbnn comments)"'

Looking back:
  Every submitted review is written down, one JSON object per line, so a year
  of them can be read as one thing rather than thrown away a round at a time.

  $ sbnn reviews --since 7d          # what was reviewed this week
  $ sbnn reviews --stats             # which files draw comments, and how many

  The log is a file, one JSON object per line, kept outside any working
  tree - a log inside the tree would dirty the very diff it is a log of.
  Reading someone else's is just reading a file:

  $ sbnn reviews --file other.jsonl
  $ cat */reviews.jsonl | sbnn reviews --file - --stats

Exporting:
  sbnn export writes the review as one self-contained HTML page that needs no
  server, which is how a review travels to someone who does not run sbnn.

  $ git diff | sbnn export review.html

Starting and stopping:
  $ sbnn --status                   # what is being reviewed
  $ sbnn --shutdown                 # stop the server
  $ sbnn --restart                  # restart it, keeping the session
  $ sbnn --clear                    # close a review: its diffs, comments, hooks
  $ sbnn --clear --all              # close every review on the server`,
	Args:         cobra.NoArgs,
	RunE:         run,
	SilenceUsage: true,
	// Execute prints the error itself, prefixed with the command name.
	SilenceErrors: true,
	Version:       version.Version,
}

// Execute runs the sbnn command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "sbnn:", err)
		os.Exit(1)
	}
}

func init() {
	f := rootCmd.Flags()
	f.StringVarP(&target, "target", "t", "", "Group name the diff is added to (default \"default\", or $SBNN_TARGET)")
	f.IntVarP(&port, "port", "p", DefaultPort, "Server port")
	f.StringVarP(&bind, "bind", "b", "localhost", "Bind address")
	f.StringVar(&title, "title", "", "Title of the diff (defaults to a generated name)")
	f.StringArrayVar(&labelFlags, "label", nil,
		"key=value kept with the diff, repeatable once per key; spaces around the key and the value are dropped, and a repeated key is an error")
	f.StringArrayVar(&collapseFlags, "collapse", nil,
		"Fold files matching this pattern, repeatable (gitignore-style: go.sum, web/dist/**)")
	f.BoolVar(&openBrowser, "open", false, "Always open the browser")
	f.BoolVar(&noOpen, "no-open", false, "Never open the browser")
	rootCmd.MarkFlagsMutuallyExclusive("open", "no-open")
	f.BoolVar(&foreground, "foreground", false, "Run the server in the foreground")
	f.BoolVar(&showStatus, "status", false, "Show the state of the running server")
	f.BoolVar(&doShutdown, "shutdown", false, "Shut the running server down")
	f.BoolVar(&doRestart, "restart", false, "Restart the running server, keeping the session")
	rootCmd.MarkFlagsMutuallyExclusive("shutdown", "restart")
	f.BoolVar(&doClear, "clear", false, "Close the review: drop the diffs, comments and hooks of the group")
	f.BoolVar(&clearAll, "all", false, "Close every review on the server; only meaningful with --clear, and refused without it")
	f.BoolVar(&assumeYes, "yes", false, "Skip the confirmation of --clear")
	f.BoolVar(&jsonOutput, "json", false, "Print structured JSON on stdout")
	f.StringVar(&moBin, "mo-bin", "mo", "mo executable used for mo's Markdown preview")
	f.IntVar(&moPort, "mo-port", mo.DefaultPort, "Port of the mo server")
	f.StringVar(&moBind, "mo-bind", mo.DefaultBind, "Bind address of the mo server")
	f.BoolVar(&allowRemote, "dangerously-allow-remote-access", false,
		"Allow binding to a non-loopback address (no authentication!)")
	f.StringVar(&onReviewCommand, "on-review", "",
		"Shell command the server runs when the review of this group is submitted")
	f.StringVar(&onReviewURL, "on-review-url", "",
		"URL the server POSTs to when the review of this group is submitted")
	f.StringVar(&historyPath, "history-file", "",
		historyFileHelp("Where submitted reviews are written down"))

	rootCmd.AddCommand(commentCmd, commentsCmd, exportCmd, hookCmd, reviewsCmd, skillCmd, submitCmd, waitCmd)
}

func addr() string {
	return net.JoinHostPort(bind, strconv.Itoa(port))
}

func moRunner() *mo.Runner {
	return mo.New(moBin, moPort, moBind)
}

// validateClearFlags rejects --all on its own. clearAll is read inside
// runClear and nowhere else, so sbnn --all used to fall through to the
// ordinary "read stdin, add a diff, print the URL" path with the flag
// ignored - a success message for something the user did not ask for, when
// what they meant closes every review on the server.
//
// This is a hand check rather than cobra's "required together" pairing, which
// reads "if one is given both are required" and would make plain sbnn
// --clear, the ordinary way to close one review, an error.
func validateClearFlags(doClear, clearAll bool) error {
	if clearAll && !doClear {
		return errors.New("--all only works with --clear (did you mean --clear --all?)")
	}
	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if err := validateClearFlags(doClear, clearAll); err != nil {
		return err
	}

	group, err := groupName(target)
	if err != nil {
		return err
	}
	// --history-file is only acted on by the invocation that starts the
	// server, but a value the flag refuses is refused wherever it is given.
	// Pointed at a server that was already up, "sbnn --history-file -" said
	// nothing and exited 0, which reads as "the log went somewhere".
	if _, err := historyFile(historyPath); err != nil {
		return err
	}

	switch {
	case foreground:
		return runServer(ctx)
	case showStatus:
		return runStatus(ctx)
	case doShutdown:
		return runShutdown(ctx)
	case doRestart:
		return runRestart(ctx)
	case doClear:
		return runClear(ctx, group)
	}

	labels, err := parseLabels(labelFlags)
	if err != nil {
		return err
	}

	content, err := readStdin()
	if err != nil {
		return err
	}

	c := client.New(addr(), uploadTimeout(len(content)))
	_, started, err := ensureServer(ctx, c)
	if err != nil {
		return err
	}

	if err := registerHooks(ctx, c, group); err != nil {
		return err
	}

	out := serveOutput{URL: server.GroupURL(c.BaseURL(), group), Group: group}
	if content != "" {
		res, err := c.AddDiff(ctx, group, server.AddDiffRequest{
			Title:    title,
			BaseDir:  workingDir(),
			Content:  content,
			Labels:   labels,
			Collapse: splitPatterns(collapseFlags),
		})
		if err != nil {
			return err
		}
		out.URL = res.URL
		out.Diff = summarize(res)
	}
	// mo used to be what previewed Markdown, and its absence was worth a
	// warning. sbnn renders the preview itself now, so nothing is missing:
	// whoever wants mo's richer one is told how to get it in the page,
	// where they ask for it.

	out.print()
	if shouldOpen(started) {
		openURL(out.URL)
	}
	return nil
}

// shouldOpen decides whether to open the browser: like mo, sbnn opens it when
// it just started the server, and otherwise only on request. In a pipeline
// or a job there is nobody to open it for, so it stays shut unless asked.
func shouldOpen(started bool) bool {
	switch {
	case noOpen:
		return false
	case openBrowser:
		return true
	case !isTerminal(os.Stdout) && !isTerminal(os.Stderr):
		return false
	default:
		return started
	}
}

// ensureServer returns the status of the running server, starting one when
// needed. started reports whether this invocation started it.
func ensureServer(ctx context.Context, c *client.Client) (status *server.Status, started bool, err error) {
	if st, err := probe(ctx, c, 500*time.Millisecond); err == nil {
		return st, false, nil
	}
	st, err := spawnServer(ctx, c)
	if err != nil {
		return nil, false, err
	}
	return st, true, nil
}

func probe(ctx context.Context, c *client.Client, timeout time.Duration) (*server.Status, error) {
	probeClient := client.New(c.Addr, timeout)
	return probeClient.Status(ctx)
}

func runStatus(ctx context.Context) error {
	c := client.New(addr(), 2*time.Second)
	st, err := c.Status(ctx)
	if err != nil {
		if jsonOutput {
			return writeJSON(map[string]any{"url": c.BaseURL(), "status": "not running"})
		}
		fmt.Printf("%s  not running\n", c.BaseURL())
		return nil
	}
	if jsonOutput {
		return writeJSON(st)
	}
	fmt.Printf("%s  running (pid %d, sbnn %s)\n", st.URL, st.PID, st.Version)
	if st.MoAvailable {
		fmt.Printf("  mo preview: %s\n", st.MoURL)
	} else {
		fmt.Printf("  mo preview: unavailable (%s)\n", st.MoError)
	}
	for _, g := range st.Groups {
		fmt.Printf("  %-16s %s  %d diff(s), %d file(s), %d comment(s), %d open\n",
			g.Name, g.URL, g.Diffs, g.Files, g.Comments, g.Unresolved)
	}
	return nil
}

func runShutdown(ctx context.Context) error {
	c := client.New(addr(), 2*time.Second)
	if _, err := c.Status(ctx); err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}
	if err := c.Shutdown(ctx); err != nil {
		return err
	}
	if err := waitForDown(ctx, c, 5*time.Second); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sbnn: server on %s stopped\n", c.Addr)
	return nil
}

func runRestart(ctx context.Context) error {
	c := client.New(addr(), 2*time.Second)
	if _, err := c.Status(ctx); err == nil {
		if err := c.Shutdown(ctx); err != nil {
			return err
		}
		if err := waitForDown(ctx, c, 5*time.Second); err != nil {
			return err
		}
	}
	// The session lives in the state file, so the new server comes back with
	// the same diffs and comments.
	st, err := spawnServer(ctx, c)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(st)
	}
	fmt.Println(st.URL)
	return nil
}

func runClear(ctx context.Context, group string) error {
	c := client.New(addr(), 2*time.Second)
	// The status already carries what is about to be lost, which is what the
	// confirmation is built from - no new endpoint needed.
	st, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("no sbnn server found on %s", c.Addr)
	}
	if clearAll {
		ok, err := askBeforeClear(clearAllQuestion(st.Groups))
		if err != nil {
			return err
		}
		if !ok {
			return errCancelled
		}
		removed, err := c.DeleteAllGroups(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "sbnn: closed %d review(s)\n", removed)
		return nil
	}
	ok, err := askBeforeClear(clearGroupQuestion(st.Groups, group))
	if err != nil {
		return err
	}
	if !ok {
		return errCancelled
	}
	if err := c.DeleteGroup(ctx, group); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sbnn: closed the review of %q\n", group)
	return nil
}

// errCancelled ends a --clear that the user declined. Nothing was dropped,
// which is not a failure, but it is not the job the caller asked for either:
// a script that pipes an answer in, or runs into a prompt it cannot answer,
// has to be able to tell "closed" from "left alone", and a status of 0 makes
// the no-op invisible. Execute prints it as "sbnn: cancelled" and exits 1.
var errCancelled = errors.New("cancelled")

// stdinIsTerminal reports whether there is somebody there to answer. It is a
// variable so a test can drive the prompt: stdin under "go test" is never a
// terminal, and the answer to a destructive question is the thing worth
// testing.
var stdinIsTerminal = func() bool { return isTerminal(os.Stdin) }

// askBeforeClear puts a question to the user before something is dropped. An
// empty question means there is nothing to lose and so nothing to ask.
// --yes skips it on purpose, and so does a stdin that is not a terminal: a
// pipeline or a job has nobody there to answer, and blocking one on a prompt
// would break the scripts that clear a review today.
func askBeforeClear(question string) (bool, error) {
	if question == "" || assumeYes || !stdinIsTerminal() {
		return true, nil
	}
	return confirm(os.Stdin, os.Stderr, question)
}

// clearAllQuestion names every review --clear --all is about to close, with
// what each one holds, so the count of open comments is in front of the user
// before they answer. It returns "" when the server holds nothing, because
// there is then nothing to lose by going ahead.
func clearAllQuestion(groups []server.GroupSummary) string {
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("sbnn: this will close every review on the server:\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "  %-16s %d diff(s), %d comment(s), %d open\n",
			g.Name, g.Diffs, g.Comments, g.Unresolved)
	}
	fmt.Fprintf(&b, "Close all %d review(s)? [y/N]: ", len(groups))
	return b.String()
}

// clearGroupQuestion asks about one review the way the browser does, and only
// where the browser does: when comments are still open. An unknown group is
// already empty, so it returns "" and the clear goes ahead as before.
func clearGroupQuestion(groups []server.GroupSummary, group string) string {
	for _, g := range groups {
		if g.Name != group {
			continue
		}
		if g.Unresolved == 0 {
			return ""
		}
		return fmt.Sprintf("Close the review of %q? %d comment(s) are still open and will go with it. [y/N]: ",
			group, g.Unresolved)
	}
	return ""
}

// confirm writes the question to out and reads the answer from in. Only "y"
// and "yes" mean yes; a blank line, an EOF, or anything else means no, so the
// default of a destructive prompt is to keep what is there. An EOF is an
// answer, not a failure.
func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	if _, err := fmt.Fprint(out, question); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

func waitForDown(ctx context.Context, c *client.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := probe(ctx, c, 300*time.Millisecond); err != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server on %s did not stop", c.Addr)
}

// readStdin reads the diff piped into sbnn. A terminal on stdin means the user
// only wants to open or manage the server, so it reads nothing.
func readStdin() (string, error) { return readDiff(os.Stdin) }

// readDiff reads the diff from f. A character device is the legitimate signal
// that nothing was piped in; a stat that fails is a different thing and is
// reported, because answering "no diff" for it would let sbnn print a review
// URL and exit 0 without ever sending the diff it was handed.
func readDiff(f *os.File) (string, error) {
	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("cannot inspect stdin: %w", err)
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDiffSize+1))
	if err != nil {
		return "", fmt.Errorf("cannot read the diff from stdin: %w", err)
	}
	if len(data) > maxDiffSize {
		return "", errors.New("the diff on stdin is too large (max 32MB)")
	}
	return string(data), nil
}

// uploadTimeout is how long to give the calls this command makes, which is
// decided by the diff it is carrying.
//
// The server has to read the body, unescape the JSON around the diff, parse
// the diff and then answer with the parsed result, and near the 32MB limit
// that whole round trip takes tens of seconds - measured at about 20s for a
// diff of exactly 32MB. A flat five seconds is right for the small calls and
// short enough that a large diff was reported as a timeout, which reads as a
// server that is not answering rather than as an upload still in progress. So
// the allowance grows with what is being sent, and stays at the old five
// seconds when there is no diff to send at all.
func uploadTimeout(size int) time.Duration {
	const (
		base  = 5 * time.Second
		perMB = 2 * time.Second
		oneMB = 1 << 20
	)
	if size <= 0 {
		return base
	}
	mb := (size + oneMB - 1) / oneMB
	return base + time.Duration(mb)*perMB
}

// parseLabels reads repeated key=value flags. The values are whatever the
// sender wanted to remember - a revision, a branch, a ticket - and sbnn keeps
// them without reading anything into them.
//
// A key may be given once. Labels are how a review is tied back to a PR or a
// revision, so silently keeping one of two values is worse than refusing the
// pair. An empty value stays legal; only an empty key is not.
func parseLabels(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	labels := make(map[string]string, len(flags))
	for _, flag := range flags {
		// Cut first, then trim: cutting on the first = is what lets a value
		// hold one of its own ("a=b=c" is a -> b=c).
		key, value, ok := strings.Cut(flag, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return nil, fmt.Errorf("--label wants key=value, got %q", flag)
		}
		if _, dup := labels[key]; dup {
			return nil, fmt.Errorf("--label %q was given more than once", key)
		}
		labels[key] = value
	}
	return labels, nil
}

// splitPatterns lets one --collapse carry a list, so that a shell can hand
// over whatever produced it without a loop.
func splitPatterns(flags []string) []string {
	out := make([]string, 0, len(flags))
	for _, flag := range flags {
		for _, p := range strings.FieldsFunc(flag, func(r rune) bool { return r == ',' || r == '\n' }) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// serveOutput is what a successful invocation prints.
type serveOutput struct {
	URL   string       `json:"url"`
	Group string       `json:"group"`
	Diff  *diffSummary `json:"diff,omitempty"`
}

type diffSummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Files         int    `json:"files"`
	Additions     int    `json:"additions"`
	Deletions     int    `json:"deletions"`
	MarkdownFiles int    `json:"markdownFiles"`
}

func summarize(res *server.AddDiffResponse) *diffSummary {
	if res == nil || res.Diff == nil {
		return nil
	}
	adds, dels := res.Diff.Stats()
	s := &diffSummary{
		ID:        res.Diff.ID,
		Title:     res.Diff.Title,
		Files:     len(res.Diff.Files),
		Additions: adds,
		Deletions: dels,
	}
	for _, f := range res.Diff.Files {
		if f.IsMarkdown {
			s.MarkdownFiles++
		}
	}
	return s
}

func (o serveOutput) print() {
	if jsonOutput {
		writeJSON(o)
		return
	}
	fmt.Println(o.URL)
	if o.Diff != nil {
		fmt.Printf("  %s: %d file(s), +%d -%d\n", o.Diff.Title, o.Diff.Files, o.Diff.Additions, o.Diff.Deletions)
	}
}

func writeJSON(v any) error {
	enc := jsonEncoder(os.Stdout)
	return enc.Encode(v)
}

// openURL opens the review page, ignoring failures: printing the URL is
// enough for headless environments.
func openURL(u string) {
	if strings.TrimSpace(u) == "" {
		return
	}
	if err := browserOpen(u); err != nil {
		fmt.Fprintf(os.Stderr, "sbnn: cannot open a browser (%v); open %s yourself\n", err, u)
	}
}
