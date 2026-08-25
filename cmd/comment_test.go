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
func TestCommentHelpNamesBulkDefaults(t *testing.T) {
	help := commentCmd.Long
	for _, want := range []string{"--author", "--diff", "--question"} {
		if !strings.Contains(help, want) {
			t.Errorf("the help does not say what %s does for --json entries", want)
		}
	}
}
