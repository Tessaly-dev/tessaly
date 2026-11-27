// SPDX-License-Identifier: Apache-2.0

// Package web embeds the built React UI into the tess binary.
package web

import (
	"embed"
	"io/fs"
)

// The "all:" prefix keeps dist/.gitkeep, so the package builds before the UI does.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built UI files, rooted at the dist folder.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub only fails on an invalid path, and "dist" is a constant.
		panic(err)
	}
	return sub
}
