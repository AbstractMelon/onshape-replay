package main

import (
	"embed"
	"io/fs"
)

//go:embed frontend_dist
var frontendEmbed embed.FS

func frontendFS() fs.FS {
	f, err := fs.Sub(frontendEmbed, "frontend_dist")
	if err != nil {
		panic(err)
	}
	return f
}
