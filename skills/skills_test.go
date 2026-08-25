package skills

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tenntenn/sbnn/internal/model"
)

// The skill is a contract: it names flags, JSON keys and hook environment
// variables that the code has to keep providing, and nothing but these tests
// notices when the two drift apart.
//
// A skill is a tree, not a file - SKILL.md may hand parts of itself to
// references/*.md without changing what it promises - so every check below
// that is about content reads the whole embedded tree concatenated, and only
// the per-file checks (empty fences) look at files one at a time.

// skillFile is one Markdown file of the embedded skill tree.
type skillFile struct {
	path string
	text string
}

// skillFiles returns every Markdown file embedded under skills/, sorted by
// path so failures come out in a stable order.
func skillFiles(t *testing.T) []skillFile {
	t.Helper()

	var out []skillFile
	err := fs.WalkDir(FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		b, err := fs.ReadFile(FS(), p)
		if err != nil {
			return err
		}
		out = append(out, skillFile{path: p, text: string(b)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded skill tree: %v", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	if len(out) == 0 {
		t.Fatal("no Markdown files embedded under skills/")
	}
	return out
}

// skillText returns every Markdown file of the skill joined into one string.
func skillText(t *testing.T) string {
	t.Helper()

	files := skillFiles(t)
	parts := make([]string, 0, len(files))
	for _, f := range files {
		parts = append(parts, f.text)
	}
	return strings.Join(parts, "\n\n")
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod, so the test can read sources and build the binary without
// assuming how deep it sits in the tree.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("absolute working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test's working directory")
		}
		dir = parent
	}
}

// emptyFence matches a fence pair with nothing but blank space between its
// two lines. A fence that opens and closes with no content in it is never
// something a reader was meant to see.
var emptyFence = regexp.MustCompile("(?m)^```[a-zA-Z0-9_+-]*[ \t]*\n[ \t]*```[ \t]*$")

func TestSkillHasNoEmptyFences(t *testing.T) {
	for _, f := range skillFiles(t) {
		t.Run(f.path, func(t *testing.T) {
			for _, loc := range emptyFence.FindAllStringIndex(f.text, -1) {
				line := 1 + strings.Count(f.text[:loc[0]], "\n")
				t.Errorf("%s:%d: empty code fence: the example it should hold is missing", f.path, line)
			}
		})
	}
}

// hookEnvAssignment matches the way the server names a variable it hands to a
// review hook: the literal that opens a "NAME=" pair. Matching the assignment
// rather than the bare name keeps a variable that is only talked about in a
// comment out of the list.
var hookEnvAssignment = regexp.MustCompile(`"(SBNN_[A-Z][A-Z0-9_]*)=`)

// hookEnvNames reads the names out of the server package rather than
// repeating them here. Comparing the skill against a copy of the list would
// only prove the copy is intact; comparing it against the source is what
// notices the ninth variable when someone adds one.
//
// Every non-test file of internal/server is scanned, not one function of one
// file. Which function builds the environment is the server's business - it
// has already moved once, from runHookCommand to hookEnv - and a test that
// pins it down goes off with no idea what drifted the moment it is renamed.
func hookEnvNames(t *testing.T, root string) []string {
	t.Helper()

	dir := filepath.Join(root, "internal", "server")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	seen := make(map[string]bool)
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(dir, name), err)
		}
		for _, m := range hookEnvAssignment.FindAllStringSubmatch(string(b), -1) {
			if seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s: no SBNN_* assignments found; the hook environment is built somewhere else now, so this test needs updating", dir)
	}
	sort.Strings(names)
	return names
}

func TestSkillMentionsEveryHookEnvVar(t *testing.T) {
	text := skillText(t)
	for _, name := range hookEnvNames(t, moduleRoot(t)) {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(text, name) {
				t.Errorf("the review hook sets %s but the skill never mentions it, so an agent writing a hook cannot use it", name)
			}
		})
	}
}

// commentJSONKeys marshals a comment twice to learn which keys the JSON of
// "sbnn comments --format json" always carries and which ones omitempty can
// drop. A zero comment shows the keys that survive an empty value; a filled
// one shows everything, and the difference is the conditional set.
func commentJSONKeys(t *testing.T) (always, conditional []string) {
	t.Helper()

	keys := func(c *model.Comment) map[string]bool {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal comment: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal comment: %v", err)
		}
		out := make(map[string]bool, len(m))
		for k := range m {
			out[k] = true
		}
		return out
	}

	zero := keys(&model.Comment{})
	filled := keys(&model.Comment{
		ID:        "cmt-1",
		Group:     "default",
		DiffID:    "diff-1",
		FileID:    "file-1",
		Path:      "main.go",
		Author:    "claude",
		Side:      "new",
		StartLine: 41,
		EndLine:   42,
		// Suggestions is derived from the body by MarshalJSON, so the body
		// has to carry a suggestion block for the key to show up at all.
		Body:      "Rename this.\n\n```suggestion\nfoo := 1\n```",
		Question:  true,
		Snippet:   "bar := 1",
		Resolved:  true,
		CreatedAt: time.Unix(0, 0).UTC(),
		UpdatedAt: time.Unix(0, 0).UTC(),
	})

	for k := range zero {
		if !filled[k] {
			t.Fatalf("key %q disappears once the comment has values; this test needs updating", k)
		}
		always = append(always, k)
	}
	for k := range filled {
		if !zero[k] {
			conditional = append(conditional, k)
		}
	}
	sort.Strings(always)
	sort.Strings(conditional)
	if len(always) == 0 || len(conditional) == 0 {
		t.Fatalf("expected both always-present and conditional keys, got %v and %v", always, conditional)
	}
	return always, conditional
}

// hedges are the ways the skill can say that a key is not always there. The
// check for the conditional keys is deliberately the weakest one that still
// catches the drift this test exists for: naming an omitempty key in the
// same breath as the always-present ones. Demanding a particular sentence
// would make the test a spell-checker for prose that is allowed to be
// rewritten, so it only asks that somewhere near the key the skill says the
// key can be absent.
var hedges = []string{
	"only when", "when set", "appear only", "present only", "only as",
	"missing", "absent", "optional", "not always",
}

// hedgeWindow is how far from a mention of the key the hedge may sit. A key
// is usually introduced in a sentence and explained in the bullet under it,
// so the window has to span more than one line.
const hedgeWindow = 600

func hasHedgeNear(text, key string) bool {
	needle := "`" + key + "`"
	lower := strings.ToLower(text)
	for off := 0; ; {
		i := strings.Index(text[off:], needle)
		if i < 0 {
			return false
		}
		i += off
		start := max(i-hedgeWindow, 0)
		end := min(i+len(needle)+hedgeWindow, len(text))
		window := lower[start:end]
		for _, h := range hedges {
			if strings.Contains(window, h) {
				return true
			}
		}
		off = i + len(needle)
	}
}

func TestSkillDescribesCommentJSONKeys(t *testing.T) {
	text := skillText(t)
	always, conditional := commentJSONKeys(t)

	t.Run("always present", func(t *testing.T) {
		for _, k := range always {
			if !strings.Contains(text, "`"+k+"`") {
				t.Errorf("comments --format json always carries %q but the skill never names it, so an agent does not know it can read it", k)
			}
		}
	})

	t.Run("conditional", func(t *testing.T) {
		for _, k := range conditional {
			if !strings.Contains(text, "`"+k+"`") {
				t.Errorf("comments --format json can carry %q but the skill never names it", k)
				continue
			}
			if !hasHedgeNear(text, k) {
				t.Errorf("the skill names %q without saying it can be missing; an agent that reads it by subscript breaks on the comments that do not have it", k)
			}
		}
	})
}

// buildSbnn builds the command line into a temporary directory so the flags
// can be read out of the same tree the test is checking. Nothing is fetched
// and nothing is started: the binary is only asked for its help.
func buildSbnn(t *testing.T, root string) string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "sbnn")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sbnn: %v\n%s", err, out)
	}
	return bin
}

var (
	availableCommand = regexp.MustCompile(`^\s{2,}([a-z][a-z0-9-]*)\s{2,}\S`)
	definedFlag      = regexp.MustCompile(`^\s+(?:-([a-zA-Z]),\s+)?(--[a-z][a-zA-Z0-9-]*)`)
)

func runHelp(t *testing.T, bin string, args ...string) string {
	t.Helper()

	out, err := exec.Command(bin, append(args, "--help")...).CombinedOutput()
	if err != nil {
		t.Fatalf("sbnn %s --help: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// section returns the lines under the named help heading, stopping at the
// blank line that ends it.
func section(help, heading string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(help, "\n") {
		switch {
		case strings.TrimSpace(line) == heading:
			in = true
		case in && strings.TrimSpace(line) == "":
			in = false
		case in:
			out = append(out, line)
		}
	}
	return out
}

// commandFlags maps every sbnn command to the flags it accepts, with the
// root command under the empty string. Flags are read from the built
// binary's help rather than from the cobra tree, because rootCmd is
// unexported and importing cmd from here would be a cycle.
func commandFlags(t *testing.T, bin string) map[string]map[string]bool {
	t.Helper()

	flagsOf := func(args ...string) map[string]bool {
		help := runHelp(t, bin, args...)
		set := make(map[string]bool)
		for _, heading := range []string{"Flags:", "Global Flags:"} {
			for _, line := range section(help, heading) {
				m := definedFlag.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				if m[1] != "" {
					set["-"+m[1]] = true
				}
				set[m[2]] = true
			}
		}
		return set
	}

	rootHelp := runHelp(t, bin)
	out := map[string]map[string]bool{"": flagsOf()}
	for _, line := range section(rootHelp, "Available Commands:") {
		m := availableCommand.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[m[1]] = flagsOf(m[1])
	}
	if len(out) < 2 {
		t.Fatal("no subcommands found in sbnn --help; this test needs updating")
	}
	return out
}

// stripCommandSubstitution removes every $( ... ) span, tracking nesting so
// that the parentheses inside a pathspec like ':(attr:generated)' do not end
// the span early. Whatever runs in there is another program with its own
// flags, and leaving it in would have the test check git's flags against
// sbnn's.
func stripCommandSubstitution(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '(' {
			depth := 1
			j := i + 2
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

// stripQuoted removes single- and double-quoted spans, so that a flag value
// spelled out as prose is not mistaken for a flag of its own.
func stripQuoted(s string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

var shellSeparator = regexp.MustCompile(`\|\||&&|[|;]`)

// sbnnInvocations returns every "sbnn ..." command written inside a fenced
// block of the skill, as its own list of tokens.
func sbnnInvocations(text string) [][]string {
	var out [][]string
	fenced := false
	for _, raw := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			continue
		}
		line := stripQuoted(stripCommandSubstitution(raw))
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		for _, part := range shellSeparator.Split(line, -1) {
			fields := strings.Fields(part)
			if len(fields) == 0 || fields[0] != "sbnn" {
				continue
			}
			out = append(out, fields)
		}
	}
	return out
}

var flagToken = regexp.MustCompile(`^--?[a-zA-Z]`)

func TestSkillUsesFlagsThatExist(t *testing.T) {
	root := moduleRoot(t)
	flags := commandFlags(t, buildSbnn(t, root))

	invocations := sbnnInvocations(skillText(t))
	if len(invocations) == 0 {
		t.Fatal("no sbnn invocations found in the skill; this test needs updating")
	}

	for _, fields := range invocations {
		command := ""
		if len(fields) > 1 {
			if _, ok := flags[fields[1]]; ok {
				command = fields[1]
			}
		}
		known := flags[command]
		for _, tok := range fields[1:] {
			if !flagToken.MatchString(tok) {
				continue
			}
			name, _, _ := strings.Cut(tok, "=")
			if known[name] {
				continue
			}
			t.Errorf("the skill writes %q but %s has no %s flag",
				strings.Join(fields, " "), displayCommand(command), name)
		}
	}
}

func displayCommand(command string) string {
	if command == "" {
		return "sbnn"
	}
	return fmt.Sprintf("sbnn %s", command)
}
