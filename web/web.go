// Package web embeds the built review UI.
//
// The assets are produced by "pnpm build" in this directory, which "task web"
// runs (see Taskfile.yml), and committed so that "go install" works without
// Node.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built assets rooted at dist.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return dist
	}
	return sub
}

// Built reports whether the UI was built into this binary.
func Built() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}
