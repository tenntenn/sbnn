package cmd

import (
	"strings"
	"testing"
)

// "sbnn hook --help" is where a hook author finds out what the review is
// handed to them as; the server is not something they read. A variable the
// server sets and the help does not name is one nobody uses, which is how
// the verdict got as far as the hook environment without anyone being told
// about it. The list has to be kept in step with hookEnv in
// internal/server/hook.go.
func TestHookHelpNamesEveryVariableTheServerSets(t *testing.T) {
	vars := []string{
		"SBNN_GROUP",
		"SBNN_URL",
		"SBNN_SERVER",
		"SBNN_PORT",
		"SBNN_COMMENTS",
		"SBNN_REVIEW_NOTE",
		"SBNN_VERDICT",
		"SBNN_BLOCKING",
	}

	for _, name := range vars {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(hookCmd.Long, name) {
				t.Errorf("sbnn hook --help does not name %s, so a hook author has no way to learn about it", name)
			}
		})
	}
}
