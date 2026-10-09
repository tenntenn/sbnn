package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/skills"
)

// installAt installs the embedded skill as release ver would, and returns the
// SKILL.md it wrote.
func installAt(t *testing.T, dir, ver string) string {
	t.Helper()
	if err := installSkill(dir, true, ver); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "sbnn", "SKILL.md")
}

func TestRefreshSkill(t *testing.T) {
	cases := []struct {
		name      string
		installed string // release the installed skill was written by
		binary    string
		wantOut   string
		wantKept  bool // the installed file must be untouched
		wantVer   string
	}{
		{"an older skill is refreshed", "1.0.0", "1.1.0", "refreshed", false, "1.1.0"},
		{"the same release is up to date", "1.1.0", "1.1.0", "up to date", true, "1.1.0"},
		{"a newer skill is left alone", "1.2.0", "1.1.0", "upgrade sbnn", true, "1.2.0"},
		{"dev does not overwrite a released skill", "1.2.0", "dev", "upgrade sbnn", true, "1.2.0"},
		{"a skill without a release is refreshed by a release", "dev", "1.1.0", "refreshed", false, "1.1.0"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := installAt(t, dir, tt.installed)
			// Make the text differ from what any binary writes, so that
			// only the release can decide.
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			edited := append(before, []byte("\nan older sentence\n")...)
			if tt.installed == tt.binary {
				edited = before
			}
			if err := os.WriteFile(path, edited, 0o644); err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			if err := refreshSkill(dir, tt.binary, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tt.wantOut)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if kept := bytes.Equal(after, edited); kept != tt.wantKept {
				t.Errorf("installed file kept = %v, want %v", kept, tt.wantKept)
			}
			if got := skills.Version(after); got != tt.wantVer {
				t.Errorf("installed release = %q, want %q", got, tt.wantVer)
			}
		})
	}
}

func TestRefreshSkillWithoutAnInstalledSkill(t *testing.T) {
	var out bytes.Buffer
	err := refreshSkill(t.TempDir(), "1.0.0", &out)
	if err == nil || !strings.Contains(err.Error(), "--install") {
		t.Errorf("err = %v, want one that points at --install", err)
	}
}
