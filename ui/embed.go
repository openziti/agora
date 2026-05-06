//go:build !no_agora_ui

package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedDist embed.FS

func distFS() fs.FS {
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
