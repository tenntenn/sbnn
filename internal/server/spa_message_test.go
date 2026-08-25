package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestUINotBuiltMessage checks the advice sbnn gives when the review UI is
// missing from the binary. The reader is stuck at that moment, so the
// command has to be one the repository really answers to.
func TestUINotBuiltMessage(t *testing.T) {
	tests := []struct {
		name string
		want string
		in   bool
	}{
		{name: "names the build command", want: "Run `task build`", in: true},
		{name: "says what it builds", want: "pnpm build", in: true},
		{name: "does not name the removed Makefile", want: "make build", in: false},
		{name: "does not name make at all", want: "Makefile", in: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Contains(uiNotBuiltMessage, tt.want)
			if got != tt.in {
				t.Errorf("strings.Contains(uiNotBuiltMessage, %q) = %v, want %v\nmessage:\n%s",
					tt.want, got, tt.in, uiNotBuiltMessage)
			}
		})
	}
}

// TestUINotBuiltMessageNamesARealTask keeps the message honest as the build
// files change: every `task <name>` it tells the reader to run must be a
// task Taskfile.yml actually defines. Renaming the task without touching
// this message fails here.
func TestUINotBuiltMessageNamesARealTask(t *testing.T) {
	defined := taskfileTasks(t)
	named := regexp.MustCompile("`task ([a-zA-Z0-9_:-]+)`").FindAllStringSubmatch(uiNotBuiltMessage, -1)
	if len(named) == 0 {
		t.Fatalf("uiNotBuiltMessage names no task to run:\n%s", uiNotBuiltMessage)
	}
	for _, m := range named {
		if !defined[m[1]] {
			t.Errorf("uiNotBuiltMessage tells the reader to run %q, which Taskfile.yml does not define (defined: %v)",
				m[0], sortedKeys(defined))
		}
	}
}

// taskfileTasks returns the task names defined in the repository's
// Taskfile.yml, read as text so that the test pulls in no YAML dependency.
func taskfileTasks(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "Taskfile.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]bool{}
	name := regexp.MustCompile(`^  ([a-zA-Z0-9_:-]+):\s*$`)
	inTasks := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "tasks:") {
			inTasks = true
			continue
		}
		if !inTasks {
			continue
		}
		// A new top-level key ends the tasks section.
		if line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		if m := name.FindStringSubmatch(line); m != nil {
			out[m[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("no tasks found in %s", path)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
