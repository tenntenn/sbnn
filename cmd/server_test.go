package cmd

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The background server is a second process, so whatever this invocation
// resolved has to travel to it as a flag. Dropping the flag when the value
// could not be resolved sent the server off to its own default and left the
// reviews piling up in a file nobody asked for - the silent log the refusal of
// "-" exists to prevent - so a value historyFile refuses has to stop the spawn
// instead.
func TestHistoryFileArgs(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    string // the value expected after --history-file
		wantAbs bool
		wantErr bool
	}{
		{name: "off word", flag: "off", want: "off"},
		{name: "false", flag: "false", want: "off"},
		{name: "zero", flag: "0", want: "off"},
		{name: "disabled", flag: "disabled", want: "off"},
		{name: "off from env", env: "none", want: "off"},
		{name: "path", flag: "reviews.jsonl", wantAbs: true},
		{name: "path from env", env: "reviews.jsonl", wantAbs: true},
		{name: "dash", flag: "-", wantErr: true},
		{name: "dash from env", env: "-", wantErr: true},
		{name: "padded dash", flag: " - ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv(HistoryEnv, tt.env)

			args, err := historyFileArgs(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("historyFileArgs(%q) returned %v, want the server not to be started at all",
						tt.flag, args)
				}
				if !strings.Contains(err.Error(), "standard stream") {
					t.Errorf("historyFileArgs(%q) failed with %q, want the standard stream complaint",
						tt.flag, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("historyFileArgs(%q): %v", tt.flag, err)
			}
			if len(args) != 2 || args[0] != "--history-file" {
				t.Fatalf("historyFileArgs(%q) = %v, want a --history-file pair", tt.flag, args)
			}
			// An empty value would tell the server to use its own default, and
			// the server has no way to tell that apart from "not given".
			if args[1] == "" {
				t.Fatalf("historyFileArgs(%q) passed an empty --history-file", tt.flag)
			}
			switch {
			case tt.wantAbs:
				if !filepath.IsAbs(args[1]) {
					t.Errorf("historyFileArgs(%q) = %q, want an absolute path", tt.flag, args[1])
				}
			default:
				if args[1] != tt.want {
					t.Errorf("historyFileArgs(%q) = %q, want %q", tt.flag, args[1], tt.want)
				}
				if !slices.Contains(HistoryOffWords, args[1]) {
					t.Errorf("historyFileArgs(%q) = %q, which the server does not read as off",
						tt.flag, args[1])
				}
			}
		})
	}
}
