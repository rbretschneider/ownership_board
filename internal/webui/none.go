//go:build !embed_frontend

// Package webui holds the built web app. This build has none; run the Vite dev
// server instead.
package webui

import "io/fs"

// FS returns nil: the web app is not embedded in this build.
func FS() fs.FS {
	return nil
}
