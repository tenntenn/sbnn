// Package doccheck tests what README.md says against what sbnn actually is.
//
// The README is the only reference material this project has: docs/ holds a
// screenshot and there is no man page or website. So its claims about the
// command line are load-bearing, and until now nothing noticed when a flag was
// renamed out from under them. These tests read the repository's own files and
// a binary built from the tree; no network and no browser are involved.
//
// Three checks the issue asks for are deliberately not here yet, because each
// would fail today for a reason that belongs to somebody else's change:
//
//   - The "Files and ports" table against internal/paths. The table claims
//     $XDG_STATE_HOME everywhere but paths.StateDir uses os.UserConfigDir on
//     macOS and Windows, so asserting it now would just be a red test. #104 is
//     fixing the table; the check belongs in the same commit that makes it true.
//   - A sweep for stale "make"/"Makefile" instructions. internal/server/spa.go
//     still tells the user to run "make build" and web/web.go still points at
//     "the Makefile", both left over from before Taskfile.yml. #36 and #37 are
//     removing those strings; the check is worth adding once they are gone.
//   - The agent skill against the CLI. SKILL.md makes the same kind of claim
//     and deserves the same treatment, but it is #114's subject and is being
//     restructured; these tests look only at README.md.
package doccheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// repoRoot walks up from the test's directory until it finds the go.mod, so
// the tests do not care how deep under the repository they are moved.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolving the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

func readREADME(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	return string(b)
}

// fenceLine matches an opening or closing ``` fence. Only the fences matter
// here; sbnn's own examples are all inside them, and prose that happens to
// mention a flag is not a claim that the flag exists.
var fenceLine = regexp.MustCompile("^\\s*```")

// fencedLines returns the lines inside fenced code blocks, keeping the
// 1-based line number of each so a failure can say where to look.
type docLine struct {
	num  int
	text string
}

func fencedLines(src string) []docLine {
	var out []docLine
	inFence := false
	for i, line := range strings.Split(src, "\n") {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, docLine{num: i + 1, text: line})
		}
	}
	return out
}

// cli is the command tree of a real sbnn binary: the root's flags, and each
// subcommand's flags. Asking the binary rather than the source means the test
// sees exactly what a reader typing the command would see.
type cli struct {
	rootFlags map[string]bool
	subFlags  map[string]map[string]bool
}

func (c cli) hasCommand(name string) bool {
	_, ok := c.subFlags[name]
	return ok
}

func (c cli) flags(command string) map[string]bool {
	if command == "" {
		return c.rootFlags
	}
	return c.subFlags[command]
}

// flagDecl matches a flag as cobra prints it in a "Flags:" block, anchored at
// the start of the line so that a flag named in some other flag's description
// ("With --clear, close every review") is not mistaken for a declaration.
var flagDecl = regexp.MustCompile(`^\s+(?:-([a-zA-Z]), )?--([a-zA-Z][a-zA-Z0-9-]*)`)

// parseFlags collects the flags out of every "Flags:" and "Global Flags:"
// block of one help text.
func parseFlags(help string) map[string]bool {
	flags := map[string]bool{}
	inBlock := false
	for line := range strings.SplitSeq(help, "\n") {
		switch {
		case line == "Flags:" || line == "Global Flags:":
			inBlock = true
			continue
		case strings.TrimSpace(line) == "":
			inBlock = false
			continue
		case !strings.HasPrefix(line, " "):
			inBlock = false
			continue
		}
		if !inBlock {
			continue
		}
		if m := flagDecl.FindStringSubmatch(line); m != nil {
			if m[1] != "" {
				flags["-"+m[1]] = true
			}
			flags["--"+m[2]] = true
		}
	}
	return flags
}

// parseCommands collects the subcommand names out of the root help.
func parseCommands(help string) []string {
	var names []string
	inBlock := false
	for line := range strings.SplitSeq(help, "\n") {
		if line == "Available Commands:" {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if strings.TrimSpace(line) == "" || !strings.HasPrefix(line, " ") {
			break
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// buildCLI builds sbnn from the tree under test and reads its command tree out
// of --help. The build is a second or so and needs nothing but a toolchain.
func buildCLI(t *testing.T) cli {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH, so there is no binary to compare against")
	}
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "sbnn")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building sbnn: %v\n%s", err, out)
	}

	help := func(args ...string) string {
		out, err := exec.Command(bin, append(args, "--help")...).CombinedOutput()
		if err != nil {
			t.Fatalf("sbnn %s --help: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	rootHelp := help()
	c := cli{rootFlags: parseFlags(rootHelp), subFlags: map[string]map[string]bool{}}
	for _, name := range parseCommands(rootHelp) {
		c.subFlags[name] = parseFlags(help(name))
	}
	if len(c.rootFlags) == 0 || len(c.subFlags) == 0 {
		t.Fatalf("read no flags or no commands out of --help; the help format must have changed:\n%s", rootHelp)
	}
	return c
}

// stripSubstitutions removes $( … ) spans, tracking nesting so that a ) inside
// the substitution does not end it early.
//
// This matters more than it looks. The README runs
//
//	git diff | sbnn --label rev=$(git rev-parse --short HEAD)
//
// where --short belongs to git, not to sbnn. Anything that reads the line
// without removing the substitution first reports --short as an unknown sbnn
// flag, which is a false alarm about the one construct the README uses to show
// that a label can be computed.
func stripSubstitutions(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "$(") {
			depth, j := 1, i+2
			for ; j < len(s) && depth > 0; j++ {
				switch s[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// segments splits one shell line into the commands it runs, so that a flag is
// attributed to the program it was typed after. "sbnn wait -q && git commit -m
// ..." is two commands, and -m is git's.
//
// Quotes are honoured well enough to keep a | or a # inside them from ending
// anything, and the quote characters themselves are dropped. This is not a
// shell; it only has to be right about the lines a README shows.
func segments(line string) [][]string {
	var (
		segs  [][]string
		cur   []string
		tok   strings.Builder
		open  bool
		quote byte
	)
	flush := func() {
		if open {
			cur = append(cur, tok.String())
			tok.Reset()
			open = false
		}
	}
	end := func() {
		flush()
		if len(cur) > 0 {
			segs = append(segs, cur)
			cur = nil
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				tok.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			open = true
		case ' ', '\t':
			flush()
		case '|', '&', ';':
			if (c == '|' || c == '&') && i+1 < len(line) && line[i+1] == c {
				i++
			}
			end()
		case '#':
			if !open {
				// A comment: the rest of the line is prose.
				end()
				return segs
			}
			tok.WriteByte(c)
		default:
			tok.WriteByte(c)
			open = true
		}
	}
	end()
	return segs
}

// looksLikeFlag keeps the check to tokens that are unambiguously flags. A bare
// "-" is a value (sbnn reviews --file -), and something like "<dir>" is a
// placeholder, so neither is looked up.
var looksLikeFlag = regexp.MustCompile(`^(--[a-zA-Z][a-zA-Z0-9-]*|-[a-zA-Z])$`)

// TestREADMECommandsExist checks that every sbnn invocation the README shows
// names a command that exists and passes flags that command accepts. A flag
// renamed in cmd/ now breaks a test instead of quietly making the README lie.
func TestREADMECommandsExist(t *testing.T) {
	c := buildCLI(t)

	type problem struct {
		line int
		what string
	}
	var problems []problem
	calls := 0

	for _, dl := range fencedLines(readREADME(t)) {
		text := strings.TrimSpace(dl.text)
		text = strings.TrimPrefix(text, "$ ")
		// A trailing backslash continues the command onto the next line; the
		// continuation carries no sbnn of its own, so dropping it is enough.
		text = strings.TrimSuffix(text, "\\")

		for _, seg := range segments(stripSubstitutions(text)) {
			if seg[0] != "sbnn" {
				continue
			}
			calls++

			command, args := "", seg[1:]
			if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
				if c.hasCommand(args[0]) {
					command, args = args[0], args[1:]
				} else if looksLikeCommandName(args[0]) {
					problems = append(problems, problem{dl.num, "unknown command " + args[0]})
					continue
				}
			}

			known := c.flags(command)
			for _, a := range args {
				if a == "--" {
					break
				}
				name := a
				if eq := strings.Index(name, "="); eq > 0 {
					name = name[:eq]
				}
				if !looksLikeFlag.MatchString(name) {
					continue
				}
				if !known[name] {
					where := "sbnn"
					if command != "" {
						where = "sbnn " + command
					}
					problems = append(problems, problem{dl.num, where + " has no flag " + name})
				}
			}
		}
	}

	if calls < 20 {
		t.Fatalf("only %d sbnn invocations found in README.md; the scanner is not reading the fenced blocks", calls)
	}
	t.Logf("checked %d sbnn invocations", calls)

	for _, p := range problems {
		t.Errorf("README.md:%d: %s", p.line, p.what)
	}
}

// looksLikeCommandName tells a mistyped subcommand from a positional argument
// such as a path or a placeholder, so only the first is reported.
func looksLikeCommandName(s string) bool {
	return regexp.MustCompile(`^[a-z][a-z-]*$`).MatchString(s) &&
		!strings.Contains(s, "/") && !strings.Contains(s, ".")
}

// TestREADMEDocumentsEnvVars checks that every SBNN_* variable cmd/util.go
// defines is mentioned somewhere in the README. An environment variable that
// changes where reviews are written down is not discoverable from --help
// alone, so the README is where a reader finds out it exists at all.
func TestREADMEDocumentsEnvVars(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "cmd", "util.go"))
	if err != nil {
		t.Fatalf("reading cmd/util.go: %v", err)
	}

	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(SBNN_[A-Z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	if len(seen) == 0 {
		t.Fatal(`no "SBNN_*" string literals in cmd/util.go; this test is looking in the wrong place`)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	readme := readREADME(t)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(readme, name) {
				t.Errorf("cmd/util.go defines %s and README.md never mentions it", name)
			}
		})
	}
}

// TestREADMEHasNoEmptyCodeBlocks catches a fence that opens and closes with
// nothing between it. It reads as a missing example rather than as a mistake,
// so nobody reports it; the same accident is open against the skill files in
// #109, and it is cheap to hold the README to it too.
func TestREADMEHasNoEmptyCodeBlocks(t *testing.T) {
	lines := strings.Split(readREADME(t), "\n")
	inFence := false
	openedAt, body := 0, 0
	for i, line := range lines {
		if !fenceLine.MatchString(line) {
			if inFence && strings.TrimSpace(line) != "" {
				body++
			}
			continue
		}
		if !inFence {
			inFence, openedAt, body = true, i+1, 0
			continue
		}
		if body == 0 {
			t.Errorf("README.md:%d: the code block opened here is empty", openedAt)
		}
		inFence = false
	}
	if inFence {
		t.Errorf("README.md:%d: this code block is never closed", openedAt)
	}
}

// referenceRow finds one row of the "Command and flag reference" table and
// returns its last cell, the one listing the flags. The table is keyed by the
// command as it is typed, so `sbnn comments` is looked up by "comments".
//
// The row is matched on its first cell rather than searched for anywhere in
// the file, because the point of the check below is that the *table* names the
// flag. A flag mentioned once in a story three hundred lines earlier is not
// what a reader scanning the reference finds.
func referenceRow(t *testing.T, readme, command string) string {
	t.Helper()
	want := "`sbnn`"
	if command != "" {
		want = "`sbnn " + command + "`"
	}
	for line := range strings.SplitSeq(readme, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 3 {
			continue
		}
		if strings.TrimSpace(cells[0]) != want {
			continue
		}
		return cells[len(cells)-1]
	}
	t.Fatalf("README.md has no reference-table row for %s; this test is looking in the wrong place", want)
	return ""
}

// clearScopeFlags are the flags that change what `--clear` destroys. They are
// singled out because getting them wrong is not recoverable: a reader who
// follows a `--clear` line that never mentions the narrower form throws away
// comments nobody has answered yet, and there is nothing left to read
// afterwards to find out that happened.
//
// #331 put the same guard on skills/sbnn/SKILL.md
// (TestSkillDocumentsTheScopeOfClear). The README needs its own, because the
// checks above only run in one direction: they take the flags the README
// prints and ask the binary whether they exist. A flag the binary has and the
// README never prints is invisible to them, which is exactly how
// --resolved-only stayed out of both `sbnn comments` blocks for #335.
var clearScopeFlags = []struct {
	command string
	flag    string
	why     string
}{
	{
		command: "comments",
		flag:    "--resolved-only",
		why:     "the selective clear: without it --clear drops the still open comments too",
	},
}

// TestREADMEDocumentsTheScopeOfClear checks both halves: that the binary still
// has the flag, and that the README names it in the prose and in the reference
// table. Either half failing is a real answer — the first says the list here is
// out of date, the second says the README is.
func TestREADMEDocumentsTheScopeOfClear(t *testing.T) {
	c := buildCLI(t)
	readme := readREADME(t)

	for _, tt := range clearScopeFlags {
		t.Run(tt.command+tt.flag, func(t *testing.T) {
			if !c.flags(tt.command)[tt.flag] {
				t.Fatalf("sbnn %s has no %s flag; this test needs updating", tt.command, tt.flag)
			}
			if !strings.Contains(readme, tt.flag) {
				t.Errorf("README.md never names %s of sbnn %s: %s",
					tt.flag, tt.command, tt.why)
			}
			if row := referenceRow(t, readme, tt.command); !strings.Contains(row, tt.flag) {
				t.Errorf("the reference table row for sbnn %s does not list %s: %s\nrow flags: %s",
					tt.command, tt.flag, tt.why, strings.TrimSpace(row))
			}
		})
	}
}
