package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var content embed.FS

func FS() fs.FS {
	result, err := fs.Sub(content, "dist")
	if err != nil {
		panic(err)
	}
	return result
}
