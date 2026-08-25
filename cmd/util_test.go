package cmd

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The words that mean "keep no log" are what someone types when they want
// the log off, and anything not on the list quietly becomes a file of that
// name. "false" and "0" used to fall through to filepath.Abs and leave a
// junk file in the working directory that "sbnn reviews" never showed.
func TestHistoryFileOffWords(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "off", flag: "off"},
		{name: "uppercase", flag: "OFF"},
		{name: "padded", flag: " off "},
		{name: "none", flag: "none"},
		{name: "no", flag: "no"},
		{name: "false", flag: "false"},
		{name: "mixed case false", flag: "False"},
		{name: "zero", flag: "0"},
		{name: "disabled", flag: "disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(HistoryEnv, "")
			got, err := historyFile(tt.flag)
			if err != nil {
				t.Fatalf("historyFile(%q): %v", tt.flag, err)
			}
			if got != "" {
				t.Errorf("historyFile(%q) = %q, want %q (no log)", tt.flag, got, "")
			}
		})
	}
}

// $SBNN_HISTORY takes the same words as the flag; a value set once in a
// shell profile is exactly where a silent mistake would live longest.
func TestHistoryFileOffWordsFromEnv(t *testing.T) {
	for _, word := range HistoryOffWords {
		t.Run(word, func(t *testing.T) {
			t.Setenv(HistoryEnv, word)
			got, err := historyFile("")
			if err != nil {
				t.Fatalf("historyFile(\"\") with %s=%q: %v", HistoryEnv, word, err)
			}
			if got != "" {
				t.Errorf("%s=%q gave %q, want %q (no log)", HistoryEnv, word, got, "")
			}
		})
	}
}

// "-" names a standard stream in "reviews --file" and "comment --suggest",
// so taking it as a file name here writes to the one file name that is a
// nuisance to delete. It has to be an error, from the flag and from the
// environment alike.
func TestHistoryFileRejectsDash(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		t.Setenv(HistoryEnv, "")
		got, err := historyFile("-")
		if err == nil {
			t.Fatalf("historyFile(\"-\") = %q, want an error", got)
		}
	})
	t.Run("env", func(t *testing.T) {
		t.Setenv(HistoryEnv, "-")
		got, err := historyFile("")
		if err == nil {
			t.Fatalf("historyFile(\"\") with %s=- returned %q, want an error", HistoryEnv, got)
		}
	})
}

// Everything that is not a word and not a dash is still a path, and still
// made absolute, because that is what pointing the log into a repository
// relies on.
func TestHistoryFilePathsStayPaths(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "relative", flag: "reviews.jsonl"},
		{name: "dot slash", flag: "./reviews.jsonl"},
		{name: "named off in a directory", flag: "logs/off"},
		{name: "dash in a name", flag: "-.jsonl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(HistoryEnv, "")
			got, err := historyFile(tt.flag)
			if err != nil {
				t.Fatalf("historyFile(%q): %v", tt.flag, err)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("historyFile(%q) = %q, want an absolute path", tt.flag, got)
			}
			want, err := filepath.Abs(tt.flag)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("historyFile(%q) = %q, want %q", tt.flag, got, want)
			}
		})
	}
}

// No flag and no environment still means the state directory, not "no log".
func TestHistoryFileDefaultsToStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(HistoryEnv, "")
	got, err := historyFile("")
	if err != nil {
		t.Fatalf("historyFile(\"\"): %v", err)
	}
	if got == "" {
		t.Fatal("historyFile(\"\") kept no log, want the default path")
	}
	if filepath.Base(got) != "reviews.jsonl" {
		t.Errorf("historyFile(\"\") = %q, want the default reviews.jsonl", got)
	}
}

// Every spelling the flag accepts has to be in the flag's help. The list of
// words grew from three to six while the help still named only "off", so
// half the accepted spellings were undiscoverable and the near-misses the
// flag now catches looked like plain paths. Driving this from
// HistoryOffWords rather than from a copy of the words is the point: adding
// a word to the list without touching the help fails here.
func TestHistoryFileHelpNamesEveryOffWord(t *testing.T) {
	usages := map[string]string{
		"sbnn":         rootCmd.Flags().Lookup("history-file").Usage,
		"sbnn reviews": reviewsCmd.Flags().Lookup("history-file").Usage,
	}
	for command, usage := range usages {
		t.Run(command, func(t *testing.T) {
			if usage == "" {
				t.Fatalf("%s --history-file has no help", command)
			}
			for _, word := range HistoryOffWords {
				if !strings.Contains(usage, strconv.Quote(word)) {
					t.Errorf("%s --history-file help does not name %s, which historyFile accepts:\n\t%s",
						command, strconv.Quote(word), usage)
				}
			}
			if !strings.Contains(usage, strconv.Quote(historyStdinWord)) {
				t.Errorf("%s --history-file help does not say %s is refused:\n\t%s",
					command, strconv.Quote(historyStdinWord), usage)
			}
			if !strings.Contains(usage, "$"+HistoryEnv) {
				t.Errorf("%s --history-file help does not name $%s:\n\t%s", command, HistoryEnv, usage)
			}
		})
	}
}

// The help has to name the words because they work, so the words it names
// are checked against historyFile itself: a word documented as "nowhere"
// that historyFile turns into a path would be a worse lie than silence.
func TestHistoryFileHelpWordsAreAccepted(t *testing.T) {
	usage := rootCmd.Flags().Lookup("history-file").Usage
	for _, word := range HistoryOffWords {
		if !strings.Contains(usage, strconv.Quote(word)) {
			continue
		}
		t.Setenv(HistoryEnv, "")
		got, err := historyFile(word)
		if err != nil {
			t.Errorf("historyFile(%q), documented as keeping no log: %v", word, err)
			continue
		}
		if got != "" {
			t.Errorf("historyFile(%q) = %q, but the help says it keeps no log", word, got)
		}
	}
}
