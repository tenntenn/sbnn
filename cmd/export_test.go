package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// page is what an exported page looks like where it matters: a lot of
// stylesheet, then the line the page reads its data back out of.
func page(t *testing.T) string {
	t.Helper()
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<style>\n" +
		strings.Repeat("body{color:#000}", 20_000) +
		"\n</style>\n</head>\n<body>\n<div id=\"root\"></div>\n" +
		"<script>window.__SBNN_DATA__ = {\"group\":\"default\"};</script>\n</body>\n</html>\n"
}

// The destination is a positional argument, so exporting over a file that
// matters is a slip anyone can make. It has to cost a word.
func TestExportRefusesToOverwriteWhatSbnnDidNotWrite(t *testing.T) {
	dir := t.TempDir()

	t.Run("a file sbnn did not write", func(t *testing.T) {
		path := filepath.Join(dir, "keep.html")
		if err := os.WriteFile(path, []byte("IMPORTANT DATA\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := checkOverwrite(path, false)
		if err == nil {
			t.Fatal("overwriting someone else's file was allowed")
		}
		if !strings.Contains(err.Error(), "--force") {
			t.Errorf("the error does not say how to go ahead: %v", err)
		}
		// Refusing is only worth anything if the file is still there.
		b, readErr := os.ReadFile(path)
		if readErr != nil || string(b) != "IMPORTANT DATA\n" {
			t.Errorf("the file was touched: %q, %v", b, readErr)
		}
	})

	t.Run("--force overwrites it anyway", func(t *testing.T) {
		path := filepath.Join(dir, "forced.html")
		if err := os.WriteFile(path, []byte("IMPORTANT DATA\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkOverwrite(path, true); err != nil {
			t.Errorf("--force was refused: %v", err)
		}
	})

	t.Run("an earlier export", func(t *testing.T) {
		path := filepath.Join(dir, "review.html")
		if err := os.WriteFile(path, []byte(page(t)), 0o644); err != nil {
			t.Fatal(err)
		}
		// Refreshing a page sbnn wrote stays one command.
		if err := checkOverwrite(path, false); err != nil {
			t.Errorf("refreshing an earlier export was refused: %v", err)
		}
	})

	t.Run("an exported fragment", func(t *testing.T) {
		path := filepath.Join(dir, "review.body.html")
		body := "<style>a{}</style>\n<div id=\"root\"></div>\n" +
			"<script>window.__SBNN_DATA__ = {};</script>\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkOverwrite(path, false); err != nil {
			t.Errorf("refreshing an exported fragment was refused: %v", err)
		}
	})

	t.Run("a path with nothing at it", func(t *testing.T) {
		if err := checkOverwrite(filepath.Join(dir, "new.html"), false); err != nil {
			t.Errorf("writing a new file was refused: %v", err)
		}
	})

	t.Run("a file that only mentions sbnn", func(t *testing.T) {
		path := filepath.Join(dir, "notes.md")
		if err := os.WriteFile(path, []byte("run sbnn export to make a page\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkOverwrite(path, false); err == nil {
			t.Error("a file that merely says sbnn was taken for an export")
		}
	})
}

// The marker sits far enough into a page to land on a read boundary, so it
// has to be found across one as well as inside one.
func TestExportedPageIsFoundAcrossReadBoundaries(t *testing.T) {
	dir := t.TempDir()
	for _, pad := range []int{0, 65_535, 65_536, 65_537, 200_000} {
		path := filepath.Join(dir, "p.html")
		body := strings.Repeat("x", pad) + "<script>" + exportMarker + "{};</script>\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if !isExportedPage(path) {
			t.Errorf("the marker was missed with %d byte(s) before it", pad)
		}
	}
}
