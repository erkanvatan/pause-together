// Package web embeds the SvelteKit build. It lives here because go:embed can't reach "../".
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
)

// "all:" keeps build/_app, where all the JS lives: plain go:embed skips names starting with _ or ".".
//
//go:embed all:build
var build embed.FS

// Build returns the web build with "build/" stripped, so index.html sits at the root.
func Build() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic(err) // only on an invalid path, which "build" is not
	}
	return sub
}

// BuildID returns the build's version from _app/version.json, the same value the page gets from
// SvelteKit's $app/environment. "" when there is none: a Go build before any web build.
func BuildID(build fs.FS) string {
	b, err := fs.ReadFile(build, "_app/version.json")
	if err != nil {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return v.Version
}
