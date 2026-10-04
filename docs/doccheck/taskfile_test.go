package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// AGENTS.md and CONTRIBUTING.md restate the same `task` commands (#358). They
// are held to Taskfile.yml rather than to each other: a command either exists
// there or it does not.

var (
	taskDecl = regexp.MustCompile(`(?m)^  ([a-z][a-z0-9:-]*):\s*$`)
	// taskUse matches a task invocation inside a code span or after a "$ "
	// prompt. Bare prose such as "task and tagpr" is deliberately not matched.
	taskUse = regexp.MustCompile("(?:`|(?m:^\\$ ))task ([a-z][a-z0-9:-]*)")
)

// taskfileTasks returns the task names under the top-level tasks: key.
func taskfileTasks(src string) map[string]bool {
	_, body, ok := strings.Cut(src, "\ntasks:\n")
	if !ok {
		return nil
	}
	names := map[string]bool{}
	for _, m := range taskDecl.FindAllStringSubmatch(body, -1) {
		names[m[1]] = true
	}
	return names
}

// unknownTasks lists the tasks doc invokes that tasks does not define.
func unknownTasks(doc string, tasks map[string]bool) []string {
	var out []string
	for _, m := range taskUse.FindAllStringSubmatch(doc, -1) {
		if !tasks[m[1]] {
			out = append(out, m[1])
		}
	}
	return out
}

func TestUnknownTasks(t *testing.T) {
	tasks := map[string]bool{"test": true, "test-go": true}
	cases := []struct {
		name string
		doc  string
		want int
	}{
		{name: "known in code span", doc: "- Test: `task test`.", want: 0},
		{name: "known after prompt", doc: "$ task test-go   # go only", want: 0},
		{name: "unknown in code span", doc: "- Test: `task tset`.", want: 1},
		{name: "prose is ignored", doc: "`task` and tagpr, a task of its own", want: 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := unknownTasks(tt.doc, tasks); len(got) != tt.want {
				t.Errorf("got %q, want %d unknown", got, tt.want)
			}
		})
	}
}

func TestDocsTaskCommandsExistInTaskfile(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("reading Taskfile.yml: %v", err)
	}
	tasks := taskfileTasks(string(src))
	if !tasks["test"] || !tasks["test-go"] || !tasks["build"] {
		t.Fatalf("read tasks %v out of Taskfile.yml; the parser is looking in the wrong place", tasks)
	}

	cases := []struct {
		file string
	}{
		{file: "AGENTS.md"},
		{file: "CONTRIBUTING.md"},
	}
	for _, tt := range cases {
		t.Run(tt.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(root, tt.file))
			if err != nil {
				t.Fatalf("reading %s: %v", tt.file, err)
			}
			if len(taskUse.FindAllString(string(b), -1)) == 0 {
				t.Fatalf("%s invokes no task; the scanner is looking in the wrong place", tt.file)
			}
			for _, name := range unknownTasks(string(b), tasks) {
				t.Errorf("%s runs `task %s`, which Taskfile.yml does not define", tt.file, name)
			}
		})
	}
}
