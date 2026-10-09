package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

// bigStore builds the #376 fixture: 20 rounds of 20 files, 300 comments.
func bigStore(tb testing.TB, path string) *Store {
	tb.Helper()
	s := NewStore(path)
	for r := 0; r < 20; r++ {
		var sb strings.Builder
		for f := 0; f < 20; f++ {
			p := fmt.Sprintf("pkg%d/file%d.go", r, f)
			fmt.Fprintf(&sb, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,%d +1,%d @@\n", p, p, p, p, 60, 61)
			for l := 0; l < 60; l++ {
				fmt.Fprintf(&sb, " line %d of %s with some ordinary looking content\n", l, p)
			}
			sb.WriteString("+added line\n")
		}
		raw := sb.String()
		s.AddDiff(DefaultGroup, &model.Diff{Raw: raw, Files: diff.Parse(raw)})
	}
	g, _ := s.Group(DefaultGroup)
	for i := 0; i < 300; i++ {
		df := g.Diffs[i%20]
		if _, err := s.AddComment(&model.Comment{Group: DefaultGroup, DiffID: df.ID, FileID: df.Files[0].ID, Path: df.Files[0].Path(), Side: "new", StartLine: 1, EndLine: 1, Body: "a comment body"}); err != nil {
			tb.Fatal(err)
		}
	}
	return s
}
