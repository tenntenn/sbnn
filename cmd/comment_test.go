package cmd

import (
	"strings"
	"testing"
)

// resetCommentFlags puts the flags that carry state between runs back to their
// defaults; commentCmd is a package-level singleton, so tests share it.
func resetCommentFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"json", "message", "suggest", "suggest-file", "author", "diff", "question"} {
		f := commentCmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("comment has no --%s flag any more", name)
		}
		if err := f.Value.Set(f.DefValue); err != nil {
			t.Fatalf("cannot reset --%s: %v", name, err)
		}
		f.Changed = false
	}
}

// TestCommentStdinReaders pins down that nothing else may read stdin while
// --json is reading the array of comments from it.
func TestCommentStdinReaders(t *testing.T) {
	tests := map[string]struct {
		args    []string
		wantErr bool
	}{
		"json alone":            {args: []string{"--json"}},
		"suggest from stdin":    {args: []string{"--suggest", "-"}},
		"message and suggest":   {args: []string{"-m", "reworded", "--suggest", "new text"}},
		"message alone":         {args: []string{"-m", "hello"}},
		"json and suggest":      {args: []string{"--json", "--suggest", "-"}, wantErr: true},
		"json and suggest text": {args: []string{"--json", "--suggest", "x"}, wantErr: true},
		"json and suggest-file": {args: []string{"--json", "--suggest-file", "new.md"}, wantErr: true},
		"json and message":      {args: []string{"--json", "-m", "hello"}, wantErr: true},

		// These stay allowed: they are the defaults bulk entries fall back to.
		"json and author":   {args: []string{"--json", "--author", "claude"}},
		"json and diff":     {args: []string{"--json", "--diff", "d1"}},
		"json and question": {args: []string{"--json", "--question"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resetCommentFlags(t)
			if err := commentCmd.ParseFlags(test.args); err != nil {
				t.Fatalf("parsing %v failed: %v", test.args, err)
			}
			err := commentCmd.ValidateFlagGroups()
			if test.wantErr {
				if err == nil {
					t.Fatalf("comment %s was accepted, want it refused as two readers of stdin",
						strings.Join(test.args, " "))
				}
				if !strings.Contains(err.Error(), "none of the others can be") {
					t.Fatalf("comment %s failed with %q, want the mutually exclusive complaint",
						strings.Join(test.args, " "), err)
				}
				return
			}
			if err != nil {
				t.Fatalf("comment %s was refused: %v", strings.Join(test.args, " "), err)
			}
		})
	}
	resetCommentFlags(t)
}

// TestCommentHelpNamesBulkDefaults keeps the help honest about which
// single-comment flags reach a --json entry, which is otherwise guesswork.
//
// The strings looked for are ones only the bulk paragraph can supply: --author,
// --diff and --question appear in the flag list on their own, so matching those
// alone would still pass with the paragraph deleted.
func TestCommentHelpNamesBulkDefaults(t *testing.T) {
	help := commentCmd.Long
	for _, want := range []string{
		// The JSON field names, which nothing else in the help mentions.
		`"author"`,
		`"diffId"`,
		`"question"`,
		// That the three act as defaults, and that the side does not.
		"act as defaults",
		"the side is taken from the entry alone",
		// And that the text-carrying flags are refused rather than ignored.
		"--message, --suggest and --suggest-file",
		"refused next to --json",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("the help of --json does not mention %q", want)
		}
	}
}

// TestBulkCommentQuestionDefault pins down that --question fills in the entries
// that leave "question" out and nothing else: an entry saying false stays a
// plain comment. It used to be OR-ed in, so --question turned every entry into
// a question whatever the entry said.
func TestBulkCommentQuestionDefault(t *testing.T) {
	tests := map[string]struct {
		entry string
		flag  bool
		want  bool
	}{
		"omitted, no flag": {entry: `{"path":"main.go","line":4,"body":"x"}`, want: false},
		"omitted, flag":    {entry: `{"path":"main.go","line":4,"body":"x"}`, flag: true, want: true},
		"false, no flag":   {entry: `{"path":"main.go","line":4,"question":false,"body":"x"}`, want: false},
		"false beats flag": {entry: `{"path":"main.go","line":4,"question":false,"body":"x"}`, flag: true, want: false},
		"true, no flag":    {entry: `{"path":"main.go","line":4,"question":true,"body":"x"}`, want: true},
		"true, flag":       {entry: `{"path":"main.go","line":4,"question":true,"body":"x"}`, flag: true, want: true},
		"null falls back":  {entry: `{"path":"main.go","line":4,"question":null,"body":"x"}`, flag: true, want: true},
		"null, no flag":    {entry: `{"path":"main.go","line":4,"question":null,"body":"x"}`, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resetCommentFlags(t)
			t.Cleanup(func() { resetCommentFlags(t) })
			commentQuestion = test.flag

			requests, err := readBulkComments(strings.NewReader("[" + test.entry + "]"))
			if err != nil {
				t.Fatalf("reading %s failed: %v", test.entry, err)
			}
			if len(requests) != 1 {
				t.Fatalf("read %d comments from %s, want 1", len(requests), test.entry)
			}
			if requests[0].Question != test.want {
				t.Errorf("%s with --question=%t stored question=%t, want %t",
					test.entry, test.flag, requests[0].Question, test.want)
			}
		})
	}
}
