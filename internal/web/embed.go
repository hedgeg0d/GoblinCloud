// Package web serves the embedded single-page interface.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// Handler serves the static web UI from the embedded filesystem.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant
	}
	return http.FileServer(http.FS(sub))
}
