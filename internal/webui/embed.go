//go:build embed_frontend

// Package webui holds the built web app. Release builds copy web/dist here and
// compile with -tags embed_frontend; dev builds leave it out and use Vite.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built web app.
func FS() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub only fails for an invalid path, and "dist" is a constant.
		panic(err)
	}
	return files
}
