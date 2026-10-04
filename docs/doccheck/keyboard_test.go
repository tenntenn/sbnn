package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The README tells a reader they can comment on a line from the keyboard, and
// web/src/shortcuts.ts is where the `?` help sheet gets the same fact. #318
// added both and only the sheet was guarded (#355): the README paragraph could
// be deleted with every suite green. These checks hold the paragraph to the
// shortcut entry, so one edit cannot satisfy one half and leave the other.

// keyNames are the words shortcuts.ts uses for keys in prose. A key named in
// the entry's description is part of the route the entry teaches, so the README
// has to name it too.
var keyNames = []string{"Tab", "Shift", "Enter", "Space", "Esc", "Ctrl", "Alt"}

// gutterShortcut is the entry of shortcuts.ts that describes commenting on a
// line from the line gutter.
type gutterShortcut struct {
	keys []string
	what string
}

var (
	shortcutEntry = regexp.MustCompile(`(?s)\{\s*keys:\s*\[([^\]]*)\],\s*what:\s*'((?:[^'\\]|\\.)*)',?\s*\}`)
	quoted        = regexp.MustCompile(`'([^']*)'`)
)

// parseGutterShortcut finds the shortcuts.ts entry whose keys are Enter and
// Space, the keys of the line gutter.
func parseGutterShortcut(src string) (gutterShortcut, bool) {
	for _, m := range shortcutEntry.FindAllStringSubmatch(src, -1) {
		var keys []string
		for _, k := range quoted.FindAllStringSubmatch(m[1], -1) {
			keys = append(keys, k[1])
		}
		if len(keys) == 2 && keys[0] == "Enter" && keys[1] == "Space" {
			return gutterShortcut{keys: keys, what: m[2]}, true
		}
	}
	return gutterShortcut{}, false
}

// keyboardParagraph returns the README bullet that introduces the keyboard
// path: the list item whose bold lead mentions the keyboard.
func keyboardParagraph(readme string) (string, bool) {
	lines := strings.Split(readme, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "- **") || !strings.Contains(strings.ToLower(line), "keyboard") {
			continue
		}
		para := []string{line}
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" || strings.HasPrefix(next, "- ") {
				break
			}
			para = append(para, next)
		}
		return strings.Join(para, " "), true
	}
	return "", false
}

// keyboardProblems says what is wrong with the README's keyboard paragraph
// relative to the shortcut entry; an empty result means they agree.
func keyboardProblems(readme string, sc gutterShortcut) []string {
	para, ok := keyboardParagraph(readme)
	if !ok {
		return []string{"README.md has no bullet about commenting from the keyboard"}
	}
	want := append([]string{}, sc.keys...)
	for _, name := range keyNames {
		if regexp.MustCompile(`\b` + name + `\b`).MatchString(sc.what) {
			want = append(want, name)
		}
	}
	var problems []string
	seen := map[string]bool{}
	for _, w := range want {
		if seen[w] {
			continue
		}
		seen[w] = true
		if !regexp.MustCompile(`\b` + w + `\b`).MatchString(para) {
			problems = append(problems, "the keyboard paragraph never names "+w+", which shortcuts.ts does")
		}
	}
	if !strings.Contains(para, "`?`") {
		problems = append(problems, "the keyboard paragraph does not point at the `?` sheet")
	}
	return problems
}

const testShortcut = `
  {
    keys: ['Enter', 'Space'],
    what: 'Comment on a line: Tab to its line number, then press. Shift extends the range',
  },`

func TestKeyboardParagraphAgainstShortcuts(t *testing.T) {
	sc, ok := parseGutterShortcut(testShortcut)
	if !ok {
		t.Fatal("test fixture does not parse")
	}

	cases := []struct {
		name   string
		readme string
		want   int
	}{
		{
			name: "agrees",
			readme: "- other\n" +
				"- **From the keyboard: Tab to a line number and press Enter or Space.** Every\n" +
				"  line number is a stop, and Shift with the press extends. The `?` sheet says so too.\n",
			want: 0,
		},
		{
			name:   "paragraph deleted",
			readme: "- other\n- another\n",
			want:   1,
		},
		{
			name: "drops Tab and Shift",
			readme: "- **From the keyboard: press Enter or Space.** The `?` sheet says so too.\n" +
				"\n",
			want: 2,
		},
		{
			name: "no pointer to the sheet",
			readme: "- **From the keyboard: Tab to a line number and press Enter or Space.** " +
				"Shift extends it.\n",
			want: 1,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := keyboardProblems(tt.readme, sc)
			if len(got) != tt.want {
				t.Errorf("got %d problems %q, want %d", len(got), got, tt.want)
			}
		})
	}
}

func TestParseGutterShortcut(t *testing.T) {
	cases := []struct {
		name string
		src  string
		ok   bool
	}{
		{name: "present", src: testShortcut, ok: true},
		{name: "other entries only", src: "{ keys: ['j'], what: 'Next file' },", ok: false},
		{name: "empty", src: "", ok: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := parseGutterShortcut(tt.src); ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
		})
	}
}

// TestREADMEKeyboardPathMatchesShortcuts holds the real README to the real
// shortcuts.ts.
func TestREADMEKeyboardPathMatchesShortcuts(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "src", "shortcuts.ts"))
	if err != nil {
		t.Fatalf("reading web/src/shortcuts.ts: %v", err)
	}
	sc, ok := parseGutterShortcut(string(src))
	if !ok {
		t.Fatal("web/src/shortcuts.ts has no entry with keys Enter and Space; this test is looking in the wrong place")
	}
	if !strings.Contains(sc.what, "Tab") {
		t.Fatalf("the shortcuts.ts entry no longer names Tab (%q); #318 is about telling the reader how to reach the line number", sc.what)
	}
	for _, p := range keyboardProblems(readREADME(t), sc) {
		t.Error(p)
	}
}
