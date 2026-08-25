package diff_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/diff"
	"github.com/tenntenn/sbnn/internal/model"
)

// hunk is a small helper for building the new side of a file by hand: every
// line is an added line numbered from the hunk's start.
func hunk(newStart int, lines ...string) *model.Hunk {
	h := &model.Hunk{NewStart: newStart, NewLines: len(lines)}
	for i, content := range lines {
		h.Lines = append(h.Lines, model.Line{
			Kind:      model.LineAdd,
			Content:   content,
			NewNumber: newStart + i,
		})
	}
	return h
}

// What a gap in a reconstruction is written as, per kind of file.
//
// The content is handed to a Markdown renderer, to a notebook renderer that
// parses it as JSON, to mo as a file on disk and to an exported page, so
// anything put in it has to be something the file's own format can carry.
func TestReconstructMarksGapsOnlyInMarkdown(t *testing.T) {
	tests := []struct {
		name string
		file *model.File
		want string
	}{
		{
			name: "markdown gets a rule and a line of italics",
			file: &model.File{
				NewPath:    "doc.md",
				IsMarkdown: true,
				Hunks:      []*model.Hunk{hunk(1, "# Title"), hunk(10, "the end")},
			},
			want: "# Title\n\n---\n\n*sbnn: 8 line(s) not included in the diff*\n\nthe end\n",
		},
		{
			name: "a notebook gets nothing: its content is JSON a renderer parses",
			file: &model.File{
				NewPath:    "nb.ipynb",
				IsNotebook: true,
				Hunks:      []*model.Hunk{hunk(1, `{"cells": [`), hunk(10, "]}")},
			},
			want: "{\"cells\": [\n]}\n",
		},
		{
			name: "source is left alone too",
			file: &model.File{
				NewPath: "main.go",
				Hunks:   []*model.Hunk{hunk(1, "package main"), hunk(20, "func main() {}")},
			},
			want: "package main\nfunc main() {}\n",
		},
		{
			name: "a gap before the first line is marked as well",
			file: &model.File{
				NewPath:    "doc.md",
				IsMarkdown: true,
				Hunks:      []*model.Hunk{hunk(4, "body")},
			},
			want: "\n---\n\n*sbnn: 3 line(s) not included in the diff*\n\nbody\n",
		},
		{
			name: "every gap is marked",
			file: &model.File{
				NewPath:    "doc.md",
				IsMarkdown: true,
				Hunks:      []*model.Hunk{hunk(1, "a"), hunk(3, "b"), hunk(6, "c")},
			},
			want: "a\n" +
				"\n---\n\n*sbnn: 1 line(s) not included in the diff*\n\n" +
				"b\n" +
				"\n---\n\n*sbnn: 2 line(s) not included in the diff*\n\n" +
				"c\n",
		},
		{
			name: "no gap, no marker",
			file: &model.File{
				NewPath:    "doc.md",
				IsMarkdown: true,
				Hunks:      []*model.Hunk{hunk(1, "a", "b"), hunk(3, "c")},
			},
			want: "a\nb\nc\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := diff.Reconstruct(tt.file)
			if got != tt.want {
				t.Errorf("Reconstruct() =\n%q\nwant\n%q", got, tt.want)
			}
			if strings.Contains(got, "<!--") {
				t.Errorf("reconstruction carries HTML markup: %q", got)
			}
		})
	}
}

// A gap that falls inside a fenced code block would be shown as code, not
// read as Markdown, so the block is closed around the marker and opened
// again after it with the line that opened it.
func TestReconstructMarksGapsInsideCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "inside a backtick fence",
			lines: []string{"```go", "a := 1"},
			want: "```go\na := 1\n" +
				"```\n" +
				"\n---\n\n*sbnn: 5 line(s) not included in the diff*\n\n" +
				"```go\n" +
				"tail\n",
		},
		{
			name:  "inside a tilde fence, closed with as many tildes",
			lines: []string{"~~~~", "text"},
			want: "~~~~\ntext\n" +
				"~~~~\n" +
				"\n---\n\n*sbnn: 5 line(s) not included in the diff*\n\n" +
				"~~~~\n" +
				"tail\n",
		},
		{
			name:  "a closed fence leaves the marker alone",
			lines: []string{"```", "a := 1", "```"},
			want: "```\na := 1\n```\n" +
				"\n---\n\n*sbnn: 4 line(s) not included in the diff*\n\n" +
				"tail\n",
		},
		{
			name:  "a fence is not closed by a shorter run of its character",
			lines: []string{"````", "``` still code"},
			want: "````\n``` still code\n" +
				"````\n" +
				"\n---\n\n*sbnn: 5 line(s) not included in the diff*\n\n" +
				"````\n" +
				"tail\n",
		},
		{
			name:  "inline code is not a fence",
			lines: []string{"see `a` and ```b```"},
			want: "see `a` and ```b```\n" +
				"\n---\n\n*sbnn: 6 line(s) not included in the diff*\n\n" +
				"tail\n",
		},
		{
			name:  "an indented line is a code block, not a fence",
			lines: []string{"    ```"},
			want: "    ```\n" +
				"\n---\n\n*sbnn: 6 line(s) not included in the diff*\n\n" +
				"tail\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &model.File{
				NewPath:    "doc.md",
				IsMarkdown: true,
				Hunks:      []*model.Hunk{hunk(1, tt.lines...), hunk(8, "tail")},
			}
			got, complete := diff.Reconstruct(f)
			if complete {
				t.Error("complete = true, want false for a diff with a gap")
			}
			if got != tt.want {
				t.Errorf("Reconstruct() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// The one thing said about a file that is not Markdown is complete, so it
// has to keep being said.
func TestReconstructCompleteness(t *testing.T) {
	tests := []struct {
		name string
		file *model.File
		want bool
	}{
		{
			name: "hunks cover the file from line 1",
			file: &model.File{NewPath: "nb.ipynb", IsNotebook: true,
				Hunks: []*model.Hunk{hunk(1, "{}")}},
			want: true,
		},
		{
			name: "a gap in a notebook is still reported",
			file: &model.File{NewPath: "nb.ipynb", IsNotebook: true,
				Hunks: []*model.Hunk{hunk(1, "{"), hunk(9, "}")}},
			want: false,
		},
		{
			name: "an added file is whole however its hunks are numbered",
			file: &model.File{NewPath: "doc.md", IsMarkdown: true, Status: model.StatusAdded,
				Hunks: []*model.Hunk{hunk(3, "body")}},
			want: true,
		},
		{
			name: "no hunks at all",
			file: &model.File{NewPath: "doc.md", IsMarkdown: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, complete := diff.Reconstruct(tt.file); complete != tt.want {
				t.Errorf("complete = %v, want %v", complete, tt.want)
			}
		})
	}
}

// The notebook renderer parses the reconstruction as JSON. Nothing sbnn made
// up may end up in it, or the browser shows "not valid JSON" instead of the
// notebook - which is what the HTML comment used to cause.
func TestReconstructedNotebookStaysParseable(t *testing.T) {
	body := []string{
		`{`,
		` "cells": [`,
		`  {"cell_type": "code", "source": ["print(1)"]}`,
		` ],`,
		` "nbformat": 4`,
		`}`,
	}
	f := &model.File{
		NewPath:    "analysis.ipynb",
		IsNotebook: true,
		Status:     model.StatusModified,
		// The whole file is in the diff, but as two hunks whose numbering
		// leaves no hole - the reconstruction is the notebook itself.
		Hunks: []*model.Hunk{hunk(1, body[:3]...), hunk(4, body[3:]...)},
	}
	got, complete := diff.Reconstruct(f)
	if !complete {
		t.Fatalf("complete = false, want true; content = %q", got)
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("reconstructed notebook is not valid JSON: %q", got)
	}

	// With a hole in it the notebook cannot be shown, but what is served is
	// still only the file's own lines: sbnn adds nothing to the JSON.
	partial := &model.File{
		NewPath:    "analysis.ipynb",
		IsNotebook: true,
		Status:     model.StatusModified,
		Hunks:      []*model.Hunk{hunk(1, body[:2]...), hunk(30, body[5])},
	}
	got, complete = diff.Reconstruct(partial)
	if complete {
		t.Error("complete = true, want false for a notebook with a gap")
	}
	if want := strings.Join([]string{body[0], body[1], body[5]}, "\n") + "\n"; got != want {
		t.Errorf("Reconstruct() = %q, want %q", got, want)
	}
}

// Snippet is the other half of this file and is unchanged by any of it: it
// keeps the diff markers, because that is what makes a stored comment still
// say something outside the browser.
func TestSnippetKeepsDiffMarkers(t *testing.T) {
	f := &model.File{Hunks: []*model.Hunk{{NewStart: 1, Lines: []model.Line{
		{Kind: model.LineContext, Content: "package main", OldNumber: 1, NewNumber: 1},
		{Kind: model.LineDelete, Content: "var a = 1", OldNumber: 2},
		{Kind: model.LineAdd, Content: "var a = 2", NewNumber: 2},
		{Kind: model.LineContext, Content: "var b = 3", OldNumber: 3, NewNumber: 3},
	}}}}

	if got, want := diff.Snippet(f, "new", 1, 2), " package main\n+var a = 2"; got != want {
		t.Errorf("Snippet(new) = %q, want %q", got, want)
	}
	if got, want := diff.Snippet(f, "old", 1, 2), " package main\n-var a = 1"; got != want {
		t.Errorf("Snippet(old) = %q, want %q", got, want)
	}
	if got := diff.Snippet(f, "new", 9, 12); got != "" {
		t.Errorf("Snippet outside the hunk = %q, want empty", got)
	}
}
